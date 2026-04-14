"use strict";

const crypto = require("node:crypto");
const fs = require("node:fs");
const fsp = require("node:fs/promises");
const os = require("node:os");
const path = require("node:path");
const { spawn } = require("node:child_process");
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
  let effectiveVersion = requestedVersion;
  let effectiveTag = releaseTag;
  let archive = archiveName(effectiveVersion, platform, arch);
  let baseUrl = releaseBaseUrl(distRepo, effectiveTag);

  if (verbose) {
    console.log(`installing officecli ${requestedVersion} for ${platform}/${arch} from ${distRepo}`);
  }

  let archiveBuffer;
  try {
    archiveBuffer = await download(`${baseUrl}/${archive}`);
  } catch (error) {
    if (requestedVersion !== "latest") {
      throw error;
    }
    effectiveTag = await resolveLatestReleaseTag(distRepo, DEFAULT_LATEST_TAG);
    effectiveVersion = effectiveTag.replace(/^v/, "");
    archive = archiveName(effectiveVersion, platform, arch);
    baseUrl = releaseBaseUrl(distRepo, effectiveTag);
    archiveBuffer = await download(`${baseUrl}/${archive}`);
  }

  const checksumsBuffer = await download(`${baseUrl}/checksums.txt`);
  const expectedChecksum = readChecksum(checksumsBuffer.toString("utf8"), archive);
  const actualChecksum = sha256(archiveBuffer);
  if (expectedChecksum !== actualChecksum) {
    throw new Error(`checksum mismatch for ${archive}`);
  }

  const tempDir = await fsp.mkdtemp(path.join(os.tmpdir(), "officecli-npm-"));
  try {
    const archivePath = path.join(tempDir, archive);
    await fsp.writeFile(archivePath, archiveBuffer);
    const extractedBinary = await extractArchive(archivePath, tempDir);

    await fsp.mkdir(RUNTIME_DIR, { recursive: true });
    await fsp.copyFile(extractedBinary, BINARY_PATH);
    await fsp.chmod(BINARY_PATH, 0o755);
    await saveMetadata({
      distRepo,
      requestedVersion,
      installedVersion: effectiveVersion,
      releaseTag: effectiveTag,
      platform,
      arch
    });
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
  ensureInstalled,
  getBinaryPath,
  installBinary
};
