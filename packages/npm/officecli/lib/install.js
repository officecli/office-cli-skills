"use strict";

const crypto = require("node:crypto");
const fs = require("node:fs");
const fsp = require("node:fs/promises");
const os = require("node:os");
const path = require("node:path");
const { execFileSync, spawn } = require("node:child_process");
const https = require("node:https");

const pkg = require("../package.json");

const PACKAGE_ROOT = path.resolve(__dirname, "..");
const RUNTIME_DIR = path.join(PACKAGE_ROOT, "runtime");
const BINARY_NAME = "officecli";
const BINARY_PATH = path.join(RUNTIME_DIR, BINARY_NAME);
const METADATA_PATH = path.join(RUNTIME_DIR, "metadata.json");
const DEFAULT_DIST_REPO = process.env.OFFICECLI_NPM_DIST_REPO || "officecli/officecli-dist";
const DEFAULT_LATEST_TAG = process.env.OFFICECLI_NPM_LATEST_TAG || "latest";
const DEFAULT_VERSION = process.env.OFFICECLI_NPM_VERSION || pkg.version;
const PROGRESS_INTERVAL_MS = 1000;

function detectPackageManager() {
  const override = String(process.env.OFFICECLI_NPM_PACKAGE_MANAGER || "").trim().toLowerCase();
  if (override) {
    return override;
  }

  const userAgent = String(process.env.npm_config_user_agent || "").trim().toLowerCase();
  if (userAgent) {
    const token = userAgent.split(/\s+/)[0] || "";
    const name = token.split("/")[0] || "";
    if (["npm", "pnpm", "yarn", "bun"].includes(name)) {
      return name;
    }
  }

  const execPath = String(process.env.npm_execpath || "").trim().toLowerCase();
  if (execPath.includes("pnpm")) {
    return "pnpm";
  }
  if (execPath.includes("yarn")) {
    return "yarn";
  }
  if (execPath.includes("bun")) {
    return "bun";
  }
  const packageRoot = PACKAGE_ROOT;
  if (packageRoot.toLowerCase().includes(`${path.sep}pnpm${path.sep}`)) {
    return "pnpm";
  }
  if (packageRoot.toLowerCase().includes(`${path.sep}yarn${path.sep}`)) {
    return "yarn";
  }
  if (packageRoot.toLowerCase().includes(`${path.sep}bun${path.sep}`)) {
    return "bun";
  }

  const inferred = detectPackageManagerFromGlobalRoots(packageRoot);
  if (inferred) {
    return inferred;
  }

  return "";
}

function normalizedPath(value) {
  const trimmed = String(value || "").trim();
  if (!trimmed) {
    return "";
  }
  try {
    return fs.realpathSync(trimmed);
  } catch {
    return path.resolve(trimmed);
  }
}

function safeCommandOutput(command, args) {
  try {
    return execFileSync(command, args, {
      encoding: "utf8",
      stdio: ["ignore", "pipe", "ignore"]
    }).trim();
  } catch {
    return "";
  }
}

function detectPackageManagerFromGlobalRoots(packageRoot) {
  const normalizedRoot = normalizedPath(packageRoot);
  const checks = [
    {
      name: "pnpm",
      candidate: () => {
        const root = safeCommandOutput("pnpm", ["root", "-g"]);
        return root ? path.join(root, pkg.name) : "";
      }
    },
    {
      name: "yarn",
      candidate: () => {
        const root = safeCommandOutput("yarn", ["global", "dir"]);
        return root ? path.join(root, "node_modules", pkg.name) : "";
      }
    },
    {
      name: "npm",
      candidate: () => {
        const root = safeCommandOutput("npm", ["root", "-g"]);
        return root ? path.join(root, pkg.name) : "";
      }
    },
    {
      name: "bun",
      candidate: () => {
        const bunInstall = String(process.env.BUN_INSTALL || path.join(os.homedir(), ".bun")).trim();
        return bunInstall ? path.join(bunInstall, "install", "global", "node_modules", pkg.name) : "";
      }
    }
  ];

  for (const check of checks) {
    const candidate = normalizedPath(check.candidate());
    if (candidate && candidate === normalizedRoot) {
      return check.name;
    }
  }
  return "";
}

function getBinaryPath() {
  return BINARY_PATH;
}

function resolvePlatform() {
  const platformMap = {
    darwin: "darwin",
    linux: "linux"
  };
  const archMap = {
    x64: "amd64",
    arm64: "arm64"
  };

  const platform = platformMap[process.platform];
  const arch = archMap[process.arch];

  if (!platform) {
    throw new Error(`unsupported operating system: ${process.platform}`);
  }
  if (!arch) {
    throw new Error(`unsupported architecture: ${process.arch}`);
  }

  return { platform, arch };
}

function resolveRequestedVersion() {
  const raw = String(DEFAULT_VERSION || "").trim();
  if (!raw || raw === "latest") {
    return { requestedVersion: "latest", releaseTag: DEFAULT_LATEST_TAG };
  }

  const normalized = raw.startsWith("v") ? raw.slice(1) : raw;
  return { requestedVersion: normalized, releaseTag: `v${normalized}` };
}

function archiveName(version, platform, arch) {
  if (version === "latest") {
    return `officecli_latest_${platform}_${arch}.tar.gz`;
  }
  return `officecli_${version}_${platform}_${arch}.tar.gz`;
}

function releaseBaseUrl(distRepo, releaseTag) {
  return `https://github.com/${distRepo}/releases/download/${releaseTag}`;
}

function githubApiUrl(distRepo, latestTag) {
  if (latestTag === "latest") {
    return `https://api.github.com/repos/${distRepo}/releases/latest`;
  }
  return `https://api.github.com/repos/${distRepo}/releases/tags/${latestTag}`;
}

function download(url) {
  return new Promise((resolve, reject) => {
    const request = https.get(
      url,
      {
        headers: {
          "user-agent": "officecli-npm-wrapper"
        }
      },
      (response) => {
        const status = response.statusCode || 0;

        if ([301, 302, 303, 307, 308].includes(status) && response.headers.location) {
          response.resume();
          resolve(download(response.headers.location));
          return;
        }

        if (status < 200 || status >= 300) {
          const chunks = [];
          response.on("data", (chunk) => chunks.push(chunk));
          response.on("end", () => {
            reject(
              new Error(
                `download failed: ${url} returned status ${status} ${Buffer.concat(chunks)
                  .toString("utf8")
                  .trim()}`
              )
            );
          });
          return;
        }

        const chunks = [];
        response.on("data", (chunk) => chunks.push(chunk));
        response.on("end", () => resolve(Buffer.concat(chunks)));
      }
    );

    request.on("error", reject);
  });
}

function formatBytes(bytes) {
  const value = Number(bytes) || 0;
  const units = ["B", "KB", "MB", "GB", "TB"];
  let current = value;
  let unitIndex = 0;
  while (current >= 1024 && unitIndex < units.length - 1) {
    current /= 1024;
    unitIndex += 1;
  }
  if (unitIndex === 0) {
    return `${Math.round(current)} ${units[unitIndex]}`;
  }
  return `${current.toFixed(1)} ${units[unitIndex]}`;
}

function formatPercent(downloaded, total) {
  if (!total || total <= 0) {
    return "";
  }
  const percent = Math.min(100, (downloaded / total) * 100);
  return `${percent.toFixed(0)}%`;
}

function formatRate(bytesPerSecond) {
  return `${formatBytes(bytesPerSecond)}/s`;
}

function createProgressLogger(label, totalBytes) {
  const startedAt = Date.now();
  let downloadedBytes = 0;
  let lastLoggedAt = 0;

  function write(force = false) {
    const now = Date.now();
    if (!force && now - lastLoggedAt < PROGRESS_INTERVAL_MS) {
      return;
    }
    lastLoggedAt = now;
    const elapsedSeconds = Math.max(1, (now - startedAt) / 1000);
    const rate = downloadedBytes / elapsedSeconds;
    if (totalBytes > 0) {
      console.log(
        `downloading ${label}: ${formatBytes(downloadedBytes)} / ${formatBytes(totalBytes)} (${formatPercent(downloadedBytes, totalBytes)}) ${formatRate(rate)}`
      );
      return;
    }
    console.log(`downloading ${label}: ${formatBytes(downloadedBytes)} ${formatRate(rate)}`);
  }

  return {
    update(chunkLength) {
      downloadedBytes += chunkLength;
      write(false);
    },
    finish() {
      write(true);
    }
  };
}

function downloadToFile(url, outputPath, label) {
  return new Promise((resolve, reject) => {
    const request = https.get(
      url,
      {
        headers: {
          "user-agent": "officecli-npm-wrapper"
        }
      },
      (response) => {
        const status = response.statusCode || 0;

        if ([301, 302, 303, 307, 308].includes(status) && response.headers.location) {
          response.resume();
          resolve(downloadToFile(response.headers.location, outputPath, label));
          return;
        }

        if (status < 200 || status >= 300) {
          const chunks = [];
          response.on("data", (chunk) => chunks.push(chunk));
          response.on("end", () => {
            reject(
              new Error(
                `download failed: ${url} returned status ${status} ${Buffer.concat(chunks)
                  .toString("utf8")
                  .trim()}`
              )
            );
          });
          return;
        }

        const totalBytes = Number(response.headers["content-length"] || 0);
        const progress = createProgressLogger(label, totalBytes);
        const hash = crypto.createHash("sha256");
        const file = fs.createWriteStream(outputPath);

        response.on("data", (chunk) => {
          hash.update(chunk);
          progress.update(chunk.length);
        });

        response.on("error", (error) => {
          file.destroy();
          reject(error);
        });

        file.on("error", (error) => {
          response.destroy(error);
          reject(error);
        });

        file.on("finish", () => {
          progress.finish();
          file.close(() => {
            resolve({
              sha256: hash.digest("hex"),
              totalBytes
            });
          });
        });

        response.pipe(file);
      }
    );

    request.on("error", reject);
  });
}

async function resolveLatestReleaseTag(distRepo, latestTag) {
  const payload = await download(githubApiUrl(distRepo, latestTag));
  const data = JSON.parse(payload.toString("utf8"));
  const tagName = String(data.tag_name || "").trim();
  if (!tagName) {
    throw new Error(`failed to resolve release tag from ${distRepo}`);
  }
  return tagName;
}

function readChecksum(checksumText, expectedName) {
  const lines = checksumText.split(/\r?\n/);
  for (const line of lines) {
    const trimmed = line.trim();
    if (!trimmed) {
      continue;
    }
    const parts = trimmed.split(/\s+/);
    if (parts.length >= 2 && parts[1] === expectedName) {
      return parts[0];
    }
  }
  throw new Error(`missing checksum entry for ${expectedName}`);
}

function sha256(buffer) {
  return crypto.createHash("sha256").update(buffer).digest("hex");
}

function runTar(args, cwd) {
  return new Promise((resolve, reject) => {
    const child = spawn("tar", args, { cwd, stdio: "pipe" });
    let stderr = "";

    child.stderr.on("data", (chunk) => {
      stderr += chunk.toString("utf8");
    });

    child.on("error", (error) => {
      reject(new Error(`failed to run tar: ${error.message}`));
    });

    child.on("exit", (code) => {
      if (code === 0) {
        resolve();
        return;
      }
      reject(new Error(`tar exited with code ${code}: ${stderr.trim()}`));
    });
  });
}

async function extractArchive(archivePath, tempDir) {
  await runTar(["-xzf", archivePath, "-C", tempDir], PACKAGE_ROOT);
  const candidate = path.join(tempDir, BINARY_NAME);
  await fsp.access(candidate, fs.constants.R_OK);
  return candidate;
}

async function loadMetadata() {
  try {
    const raw = await fsp.readFile(METADATA_PATH, "utf8");
    return JSON.parse(raw);
  } catch {
    return null;
  }
}

async function saveMetadata(metadata) {
  await fsp.mkdir(RUNTIME_DIR, { recursive: true });
  await fsp.writeFile(METADATA_PATH, `${JSON.stringify(metadata, null, 2)}\n`, "utf8");
}

async function installBinary({ verbose = true } = {}) {
  const { platform, arch } = resolvePlatform();
  const distRepo = DEFAULT_DIST_REPO;
  const { requestedVersion, releaseTag } = resolveRequestedVersion();
  const packageManager = detectPackageManager();
  let effectiveVersion = requestedVersion;
  let effectiveTag = releaseTag;
  let archive = archiveName(effectiveVersion, platform, arch);
  let baseUrl = releaseBaseUrl(distRepo, effectiveTag);

  if (verbose) {
    console.log(`installing officecli ${requestedVersion} for ${platform}/${arch} from ${distRepo}`);
  }

  let archiveSha256;
  let archivePath;
  let tempDir;
  try {
    tempDir = await fsp.mkdtemp(path.join(os.tmpdir(), "officecli-npm-"));
    archivePath = path.join(tempDir, archive);
    const archiveDownload = await downloadToFile(`${baseUrl}/${archive}`, archivePath, archive);
    archiveSha256 = archiveDownload.sha256;
  } catch (error) {
    if (requestedVersion !== "latest") {
      throw error;
    }
    effectiveTag = await resolveLatestReleaseTag(distRepo, DEFAULT_LATEST_TAG);
    effectiveVersion = effectiveTag.replace(/^v/, "");
    archive = archiveName(effectiveVersion, platform, arch);
    baseUrl = releaseBaseUrl(distRepo, effectiveTag);
    if (!tempDir) {
      tempDir = await fsp.mkdtemp(path.join(os.tmpdir(), "officecli-npm-"));
    }
    archivePath = path.join(tempDir, archive);
    const archiveDownload = await downloadToFile(`${baseUrl}/${archive}`, archivePath, archive);
    archiveSha256 = archiveDownload.sha256;
  }

  const checksumsBuffer = await download(`${baseUrl}/checksums.txt`);
  const expectedChecksum = readChecksum(checksumsBuffer.toString("utf8"), archive);
  if (verbose) {
    console.log(`verifying checksum for ${archive}`);
  }
  if (expectedChecksum !== archiveSha256) {
    throw new Error(`checksum mismatch for ${archive}`);
  }

  try {
    if (verbose) {
      console.log(`extracting ${archive}`);
    }
    const extractedBinary = await extractArchive(archivePath, tempDir);

    await fsp.mkdir(RUNTIME_DIR, { recursive: true });
    await fsp.copyFile(extractedBinary, BINARY_PATH);
    await fsp.chmod(BINARY_PATH, 0o755);
    await saveMetadata({
      distRepo,
      requestedVersion,
      installedVersion: effectiveVersion,
      releaseTag: effectiveTag,
      packageManager,
      platform,
      arch
    });
    if (verbose) {
      console.log(`installed officecli ${effectiveVersion} to ${BINARY_PATH}`);
    }
  } finally {
    await fsp.rm(tempDir, { recursive: true, force: true });
  }

  return BINARY_PATH;
}

async function ensureInstalled({ verbose = true } = {}) {
  if (process.env.OFFICECLI_NPM_SKIP_DOWNLOAD === "1") {
    throw new Error("officecli download is disabled by OFFICECLI_NPM_SKIP_DOWNLOAD=1");
  }

  const metadata = await loadMetadata();
  const { platform, arch } = resolvePlatform();
  const { requestedVersion } = resolveRequestedVersion();

  if (fs.existsSync(BINARY_PATH) && metadata) {
    const platformMatches = metadata.platform === platform && metadata.arch === arch;
    const versionMatches =
      metadata.requestedVersion === requestedVersion ||
      (requestedVersion === "latest" && typeof metadata.installedVersion === "string");
    if (platformMatches && versionMatches) {
      return BINARY_PATH;
    }
  }

  return installBinary({ verbose });
}

module.exports = {
  detectPackageManager,
  ensureInstalled,
  getBinaryPath,
  installBinary,
  loadMetadata,
  saveMetadata
};
