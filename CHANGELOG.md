# Changelog

## 0.2.76 - 2026-05-22

### Added

- TUI interactive mode: `/mode [hosted|external]` command to display the current runtime mode or switch between hosted and external without restarting the session.

### Changed

- Hosted AI image generation now charges a flat 10 credits per image (~$0.10), replacing the previous tiered formula. Default hosted pricing rule (`gpt-image-2 / image_default`) and the runtime billing path are updated in lockstep; minimum charge stays at 10 credits.
- Billing page surfaces the new per-image usage rate so customers can estimate cost before generation.
- Marketing site: officedex hero CTA copy updated to "Coming soon" for consistency with other coming-soon surfaces.

### Fixed

- Admin dashboard `joinList` helper now handles `null`/`undefined` arrays without throwing, preventing dashboard render crashes when an upstream field is missing.

## 0.2.75 - 2026-05-22

### Changed

- npm Repository link now points at the URL declared in `packages/npm/officecli/package.json` (currently `github.com/officecli/officecli`). The sync script no longer overwrites `repository`/`bugs` with the internal `officecli-npm` mirror.

## 0.2.74 - 2026-05-22

### Added

- Windows (amd64/arm64) binary release targets. The npm wrapper now installs the matching `officecli.exe` on `win32` x64/arm64 hosts.

### Changed

- npm package metadata: add `repository` and `bugs` fields and expand social links in the README.
- README "Supported Platforms" section now lists Windows x64 and arm64.

## 0.2.73 - 2026-05-21

### Fixed

- Fix hosted-mode PPTX standard-quality images sending wrong model name (`hosted/text` instead of `hosted/image`) to the platform, causing image generation to fail and credits not being tracked.

### Changed

- Default generation mode changed from `fast` to `best` for all document types except `img`. Best mode asks clarifying questions before generating, producing higher-quality output. Use `--mode fast` to skip questions.
- Admin dashboard: add fingerprint quality CSV export and enhanced store operations.

## 0.2.72 - 2026-05-21

### Fixed

- Fix preflight misidentifying session-logged-in users as anonymous, causing `officecli new img` and other generation commands to fail with a misleading "account login or API key required" error.
- Handle whoami network failures gracefully by distinguishing network errors from genuinely invalid sessions, preventing transient connectivity issues from blocking authenticated users.
- Make skill bundle refresh non-fatal when the officecli binary is already installed, so transient GitHub connectivity issues no longer block generation.
- Allow session-authenticated users to pass the license check for image generation without a paid API key (hosted credits are sufficient).
- Add `OFFICECLI_SKIP_SKILL_PREFLIGHT=1` hint to network-related preflight error messages.

## 0.2.71 - 2026-05-21

### Fixed

- Partial fix for preflight auth detection (superseded by 0.2.72).

## 0.2.70 - 2026-05-21

### Changed

- Anonymous users running image generation are now guided to `officecli login` instead of being prompted for a raw API key.
- Environment check and fix scripts detect authentication mode via `officecli whoami` and surface login recommendations when anonymous.
- Skill documentation adds an Authentication section covering login, whoami, doctor, and set-key commands.
- Preflight error messages include actionable login guidance for the `account_login` missing item.

## 0.2.69 - 2026-05-20

### Fixed

- Check for updates before launching the interactive TUI from empty input or a natural-language prompt while keeping explicit command entrypoints unchanged.

## 0.2.68 - 2026-05-20

### Added

- Added operational event tracking and an admin operations funnel view for acquisition, activation, usage, and revenue health.

### Fixed

- Retry transient internal platform transport failures up to three times so standalone image generation can recover from temporary EOF or connection reset errors.

## 0.2.67 - 2026-05-15

### Fixed

- Treat online preview publishing failures as warnings so document generation still succeeds and reports the local file path.
- Reuse active CLI login sessions for platform preview publishing when generation and publish target the same OfficeCLI platform endpoint.

## 0.2.66 - 2026-05-15

### Changed

- Added `/login` to the interactive TUI so users can complete browser-based account login without leaving the session.
- Removed `/clear` from the interactive TUI command set and report it as an unknown command.
- Wrapped TUI help, status, and footer text to prevent long help lines from being truncated.

## 0.2.65 - 2026-05-15

### Changed

- Enabled online preview publishing by default for new installs with no existing config file.
- Updated OpenClaw OfficeCLI skill templates so newly generated skill config also defaults to publishing previews.

## 0.2.64 - 2026-05-15

### Fixed

- Fixed hosted document generation requests so account-credit billing always receives a request id before reserving credits.
- Fixed the TUI prompt router so Chinese requests such as `画一个图，关于长江` use standalone image generation instead of PPTX generation.

## 0.2.63 - 2026-05-15

### Changed

- Changed `officecli login` success output to show the account email when the platform returns it.

## 0.2.62 - 2026-05-15

### Added

- Added browser-based `officecli login`, `officecli logout`, and `officecli whoami` for account hosted credits.
- Added account-level hosted credit accounts, ledgers, CLI sessions, and migrations for MySQL and Postgres.

### Changed

- Changed Hosted Mode billing so CLI sessions and API keys consume the same account hosted credits.
- Changed Billing, Overview, API Keys, Invite, Discord, Docs, and Download copy to the account hosted credits model.
- Changed invite activation and Discord verification rewards to grant 100 account hosted credits each.

## 0.2.61 - 2026-05-14

### Changed

- Removed the previously added non-interactive namespace and kept `officecli new ...` as the only generation command path.
- External OpenAI-compatible generation now retries `/v1/chat/completions` when a root base URL returns an HTML app shell, matching New API-style gateways configured without `/v1`.

## 0.2.60 - 2026-05-14

### Changed

- Added the Bubble Tea based Codex-style `officecli` TUI for continuous natural-language document generation, including `--no-alt-screen` for scrollback-friendly sessions.
- Added `officecli exec ...` as the recommended non-interactive command namespace while keeping `officecli new ...` compatible.
- Added a local MIT-licensed `go-localereader` replacement for darwin/linux TUI builds so dependency scans do not rely on an upstream module without a standalone LICENSE file.

## 0.2.59 - 2026-05-14

### Changed

- Lowered the default PPT quality evaluation pass threshold to 60 for installed CLI E2E runs.

## 0.2.58 - 2026-05-14

### Changed

- Simplified `officecli --help` and `officecli new --help` around hosted-first copy-paste examples for first-time users.
- Added post-install next steps to the npm wrapper and shell installer.
- Reworked README, npm README, download page, and docs quickstart so hosted trial generation is the default first-run path.

## 0.2.57 - 2026-05-14

### Changed

- Made the official website URL visible in the npm package README link text.

## 0.2.56 - 2026-05-14

### Changed

- Added the official website link to the top of the npm package README.

## 0.2.55 - 2026-05-14

### Changed

- The npm-installed CLI now defaults to hosted anonymous trial access, so first-run generation works without a local model endpoint or hosted API key.
- Anonymous hosted trial quota now uses the lifetime machine fingerprint quota and reports `quota_snapshot.free_trial` with `scope=lifetime`.
- Hosted text, JSON, and structured requests accept valid anonymous commit tokens, while final quota is consumed only after a successful artifact is written.

## 0.2.54 - 2026-05-12

### Changed

- External Mode is now free and unlimited for document and standalone image generation, while Hosted Mode continues to use hosted credits.
- Billing and pricing now sell hosted credit packs only, with historical external orders preserved for reconciliation.
- The marketing site, app, admin, docs, and quickstart copy now present External and Hosted as the two primary runtime modes.

### Added

- New users receive 30 hosted credits, and each activated referral grants the inviter 20 hosted credits with idempotent grant tracking.

## 0.2.53 - 2026-05-12

### Changed

- Hosted pricing profiles are now limited to `text` and `image`: document text generation uses `hosted/text`, while standalone images and PPT image assets use `hosted/image`.

## 0.2.52 - 2026-05-11

### Fixed

- `officecli config set-license` now syncs the platform publish credential when publishing uses the default OfficeCLI platform endpoint, preventing stale preview-publish keys after rotating a platform API key.

## 0.2.35 - 2026-05-07

### Added

- Added `officecli new img --reference-image <path-or-url>` for a single local or remote reference image.
- Added `agent-bridge` `office.generate` support for `reference_image` and capability metadata under standalone image generation.
- Added platform hosted image support for OpenAI image edits when a parsed `reference_image` payload is present.
- Added default online publishing for standalone `new img` outputs, including protected platform image preview links.

## 0.1.0 - 2026-03-31

First usable CLI release, focused on turning the repository from a reusable library into a tool that end users can run directly.

### Added

- Added the `officecli new <pptx|docx|xlsx> <topic> [brief]` command entrypoint
- Added `--prompt`, `--prompt-file`, `--mode`, `--lang`, `--style`, `--audience`, `--out`, `--publish`, `--no-publish`, and `--json`
- Added default human-readable output and structured `--json` output
- Added `--help`, `--version`, and build-time version injection
- Added `internal/providers/llm` with OpenAI-compatible and internal HTTP providers
- Added `internal/providers/publish` to publish generated files and return URLs/passwords
- Added sample configuration and prompt files under `examples/`
- Added a `Makefile` covering `build/test/install/run-help/demo/release`
- Added `scripts/demo.sh` for a full local CLI demo flow

### Changed

- Rewrote the README as a human-user-facing usage guide
- Added a runtime wiring layer on top of the engine libraries so `pptx/docx/xlsx` can all run through one unified CLI

### Notes

- Current release targets output `darwin` and `linux` `amd64/arm64` binaries into `dist/`
- The default version string is `dev`; inject a real version with `make build VERSION=...` or `make release VERSION=...`
