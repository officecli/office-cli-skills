"use strict";

const test = require("node:test");
const assert = require("node:assert/strict");
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
  assert.match(output, /officecli new pptx "Q3 Business Review" --prompt "Create a six-slide executive deck for a SaaS quarterly business review\. Cover growth, retention, risks, and next-quarter actions\."/);
  assert.match(output, /officecli new docx "Product Launch Brief" --prompt "Write a concise launch brief with audience, positioning, timeline, risks, and next steps\."/);
  assert.match(output, /officecli auth status/);
  assert.match(output, /officecli auth set-key <api-key>/);
});
