"use strict";

const test = require("node:test");
const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");
const pkg = require("../package.json");

const modulePath = "../lib/install.js";

function loadInstallModule() {
  delete require.cache[require.resolve(modulePath)];
  return require(modulePath);
}

test("resolveRequestedVersion uses package.json version", () => {
  delete process.env.OFFICECLI_NPM_VERSION;
  delete process.env.OFFICECLI_NPM_LATEST_TAG;

  const install = loadInstallModule();
  const resolved = install.resolveRequestedVersion();
  const expectedVersion = String(pkg.version).replace(/^v/, "");

  assert.equal(resolved.requestedVersion, expectedVersion);
  assert.equal(resolved.releaseTag, `v${expectedVersion}`);
});

test("resolveRequestedVersion rejects legacy version override", () => {
  process.env.OFFICECLI_NPM_VERSION = "latest";
  delete process.env.OFFICECLI_NPM_LATEST_TAG;

  const install = loadInstallModule();
  assert.throws(
    () => install.resolveRequestedVersion(),
    /OFFICECLI_NPM_VERSION is no longer supported/
  );

  delete process.env.OFFICECLI_NPM_VERSION;
});

test("resolveRequestedVersion rejects legacy latest tag override", () => {
  delete process.env.OFFICECLI_NPM_VERSION;
  process.env.OFFICECLI_NPM_LATEST_TAG = "latest";

  const install = loadInstallModule();
  assert.throws(
    () => install.resolveRequestedVersion(),
    /OFFICECLI_NPM_LATEST_TAG is no longer supported/
  );

  delete process.env.OFFICECLI_NPM_LATEST_TAG;
});

test("archiveName always resolves to a stable release asset name", () => {
  const install = loadInstallModule();
  const expectedVersion = String(pkg.version).replace(/^v/, "");
  assert.equal(
    install.archiveName(expectedVersion, "linux", "arm64"),
    `officecli_${expectedVersion}_linux_arm64.tar.gz`
  );
});

test("install next steps show copy-paste hosted examples", () => {
  const install = loadInstallModule();
  const output = install.nextStepsText();

  assert.match(output, /Try OfficeCLI now:/);
  assert.match(output, /officecli --version/);
  assert.match(output, /^  officecli$/m);
  assert.match(output, /officecli "Create a Q3 business review deck"/);
  assert.match(output, /officecli new pptx "Q3 Business Review" --prompt "Create a six-slide executive deck for a SaaS quarterly business review\. Cover growth, retention, risks, and next-quarter actions\."/);
  assert.match(output, /officecli auth status/);
  assert.match(output, /officecli auth set-key <api-key>/);
});

test("package exposes officecli-dev wrapper with dev profile defaults", () => {
  assert.equal(pkg.bin["officecli-dev"], "bin/officecli-dev.js");

  const wrapperPath = path.resolve(__dirname, "..", "bin", "officecli-dev.js");
  const wrapper = fs.readFileSync(wrapperPath, "utf8");
  assert.match(wrapper, /OFFICE_CLI_PROFILE/);
  assert.match(wrapper, /OFFICECLI_DEV_PLATFORM_BASE_URL/);
  assert.match(wrapper, /NO_PROXY/);
  assert.match(wrapper, /https:\/\/officecli\.shimodev\.com/);
});
