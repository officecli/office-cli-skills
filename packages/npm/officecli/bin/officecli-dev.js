#!/usr/bin/env node

const { spawn } = require("node:child_process");
const { detectPackageManager, ensureInstalled, getBinaryPath, loadMetadata, saveMetadata } = require("../lib/install");

const DEV_PLATFORM_BASE_URL = "https://officecli.shimodev.com";
const DEV_PLATFORM_HOST = "officecli.shimodev.com";

function appendNoProxy(value, host) {
  const parts = String(value || "")
    .split(",")
    .map((part) => part.trim())
    .filter(Boolean);
  if (!parts.includes(host)) {
    parts.push(host);
  }
  return parts.join(",");
}

async function main() {
  try {
    const binaryPath = await ensureInstalled({ verbose: false });
    const metadata = await loadMetadata();
    const packageManager =
      typeof metadata?.packageManager === "string" && metadata.packageManager.trim()
        ? metadata.packageManager.trim()
        : detectPackageManager();
    if (metadata && packageManager && metadata.packageManager !== packageManager) {
      await saveMetadata({
        ...metadata,
        packageManager
      });
    }
    const noProxy = appendNoProxy(process.env.NO_PROXY || process.env.no_proxy || "", DEV_PLATFORM_HOST);
    const child = spawn(binaryPath, process.argv.slice(2), {
      stdio: "inherit",
      env: {
        ...process.env,
        OFFICE_CLI_PROFILE: "dev",
        OFFICECLI_DEV_PLATFORM_BASE_URL: process.env.OFFICECLI_DEV_PLATFORM_BASE_URL || DEV_PLATFORM_BASE_URL,
        NO_PROXY: noProxy,
        no_proxy: noProxy,
        OFFICECLI_INSTALL_METHOD: "npm",
        ...(packageManager ? { OFFICECLI_PACKAGE_MANAGER: packageManager } : {})
      }
    });

    child.on("exit", (code, signal) => {
      if (signal) {
        process.kill(process.pid, signal);
        return;
      }
      process.exit(code ?? 1);
    });

    child.on("error", (error) => {
      console.error(`failed to start officecli-dev binary: ${error.message}`);
      process.exit(1);
    });
  } catch (error) {
    const binaryPath = getBinaryPath();
    console.error(`failed to prepare officecli-dev binary at ${binaryPath}`);
    console.error(error instanceof Error ? error.message : String(error));
    process.exit(1);
  }
}

main();
