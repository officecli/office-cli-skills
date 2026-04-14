#!/usr/bin/env node

const pkg = require("../package.json");

function normalizeTag(raw) {
  const value = String(raw || "").trim();
  if (!value) {
    return "";
  }
  return value.startsWith("v") ? value.slice(1) : value;
}

function main() {
  const releaseTag =
    process.env.RELEASE_TAG ||
    process.env.GITHUB_REF_NAME ||
    process.env.npm_package_version ||
    "";

  const normalizedTag = normalizeTag(releaseTag);
  if (!normalizedTag) {
    console.error("missing release tag; set RELEASE_TAG or GITHUB_REF_NAME");
    process.exit(1);
  }

  if (normalizedTag !== pkg.version) {
    console.error(
      `npm wrapper version mismatch: package.json=${pkg.version}, release_tag=${normalizedTag}`
    );
    process.exit(1);
  }

  console.log(`release version check passed: ${pkg.version}`);
}

main();
