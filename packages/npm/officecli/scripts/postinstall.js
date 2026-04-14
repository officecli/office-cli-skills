#!/usr/bin/env node

const { ensureInstalled } = require("../lib/install");

async function main() {
  if (process.env.OFFICECLI_NPM_SKIP_DOWNLOAD === "1") {
    console.log("skipping officecli download because OFFICECLI_NPM_SKIP_DOWNLOAD=1");
    return;
  }

  await ensureInstalled({ verbose: true });
}

main().catch((error) => {
  console.error(error instanceof Error ? error.message : String(error));
  process.exit(1);
});
