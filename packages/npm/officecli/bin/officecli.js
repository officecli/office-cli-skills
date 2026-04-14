#!/usr/bin/env node

const { spawn } = require("node:child_process");
const { ensureInstalled, getBinaryPath } = require("../lib/install");

async function main() {
  try {
    const binaryPath = await ensureInstalled({ verbose: false });
    const child = spawn(binaryPath, process.argv.slice(2), { stdio: "inherit" });

    child.on("exit", (code, signal) => {
      if (signal) {
        process.kill(process.pid, signal);
        return;
      }
      process.exit(code ?? 1);
    });

    child.on("error", (error) => {
      console.error(`failed to start officecli binary: ${error.message}`);
      process.exit(1);
    });
  } catch (error) {
    const binaryPath = getBinaryPath();
    console.error(`failed to prepare officecli binary at ${binaryPath}`);
    console.error(error instanceof Error ? error.message : String(error));
    process.exit(1);
  }
}

main();
