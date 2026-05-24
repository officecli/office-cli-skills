# Changelog

## 0.2.82 - 2026-05-24

### Changed

- Web paywall funnel: site `Pricing` cards now deep-link to `platform.officecli.io/app/billing?pack=<code>&autostart=1`; on landing the app reads the query and auto-launches Stripe Checkout once (idempotent via `hasAutoStartedRef`). Marketing-site CTA → Stripe is now effectively 2 clicks for signed-in users (was 6 hops with pack-selection loss).
- Pricing labels rewritten: removed the misleading "100 credits = $1 USD" auxiliary line. Each pack now shows real `$/credit` (3 sig figs) and an `≈ N images @ 10 credits each` business-value conversion, computed from the same pack data on both `site/Pricing` and `app/BillingPage`.
- Site `Pricing` renders up to 3 hosted-credits packs (was hardcoded to 2). The "Best value" badge is data-driven (lowest `$/credit`); "POPULAR" stays on the middle card.
- App `Overview` now shows a low-balance warning banner when hosted credits fall below 20, plus a "Buy more credits →" link on the Hosted Credits metric card.
- App `Billing` page adds a "Have a redeem code?" link to `/redeem`.
- Redeem page localized to English (form labels, errors, history table, success toast).
- App sidebar: Quota nav entry demoted to "Legacy quota" with the `Archive` icon, moved below Billing — it remains routable but no longer competes for primary attention.
- Site `/login` route removed (Navbar Login already jumps to `platform.officecli.io/app`); the orphan stub page file is left on disk for future reuse.

### Added

- `siteApi`: new helpers `pricePerCreditLabel`, `imagesPerPack`, `bestValueCode` for consistent pricing math.
- Tests: 3 new autostart cases in `BillingPage.test.tsx` (autostart fires once, idempotent on re-render, ignored when pack unknown); new `Pricing.test.tsx` covering 3-card render, deep-link href shape, and Best value badge selection.

## 0.2.81 - 2026-05-23

### Added

- New `hosted-100` SKU on the hosted pricing tier: 100 credits for $1.00 USD. Lands as the entry-level pack alongside the existing `hosted-300` ($29) and `hosted-1200` ($99) packs and surfaces on `/pricing` as a third card (entry / popular / bulk) when 3+ hosted packs are returned.

### Changed

- Site `Pricing` component now renders a responsive 1/2/3-column grid driven by the number of hosted packs returned by `/api/pricing`. The 2-pack fallback path is preserved for compatibility.

## 0.2.80 - 2026-05-23

### Added

- Redemption code system: admins can mint, edit, enable/disable codes from the admin web (`/redemption-codes`) and audit every claim from `/redemption-records`. Each code carries `credit_amount`, optional `max_redemptions`, optional `expires_at`, and a `per_user_limit` (default 1). A partial UNIQUE index on `(redemption_code_id, user_id) WHERE per_user_limit_at_claim = 1` enforces single-claim codes even if the application-layer `FOR UPDATE` serialization is bypassed.
- Users can redeem codes from three entry points sharing one platform service:
  - CLI: `officecli redeem <code>` (with `--code`, `--json`, `--source`).
  - TUI: new `/redeem <code>` slash command — runs the same code path as the CLI and refreshes credit status inline.
  - Web app: new `/redeem` page (sidebar entry "Redeem") with claim form and personal redemption history.
- New backend endpoints: `POST /api/app/redemption-codes/redeem` (cookie session), `GET /api/app/redemption-codes/my`, `POST /api/cli/redemption-codes/redeem` (Bearer auth), and admin CRUD under `/api/admin/redemption-codes` plus `/api/admin/redemption-codes/redemptions`.
- Hosted credit ledger gains a `redemption_code` source; each successful redeem writes a single ledger entry with an idempotency key tied to the redemption row so retries are safe.
- Stable machine-readable error codes returned to clients (`code_not_found`, `code_disabled`, `code_expired`, `code_exhausted`, `code_already_claimed`, `code_required`) mapped to 404/403/410/410/409/400 respectively.

### Migration

- Postgres `025_grant_existing_users_100_credit_bonus.sql` retroactively grants 100 hosted credits to every existing user (idempotent via `signup-bonus-bump-100:<userID>`), completing the 30→100 signup-bonus bump from 0.2.78.
- Postgres `026_redemption_codes.sql` creates `redemption_codes` and `redemption_code_redemptions`.
- Postgres `027_redemption_code_singleton_unique.sql` adds `per_user_limit_at_claim` snapshot column and the partial UNIQUE index that enforces single-claim codes.

## 0.2.79 - 2026-05-23

### Changed

- Anonymous trial replaced by per-device hosted credits. The legacy lifetime `free_quotas` and daily `daily_free_quotas` tables are dropped; each new device fingerprint now lands a `fingerprint_credit_accounts` row seeded with 100 starter credits the first time it calls license `check`. Anonymous and registered users share the same hosted billing path (10 credits per image, token-priced text, etc.) — the only difference is that registered users can top up.
- `officecli login` exchange now carries `fingerprint_hash`. On successful login the server merges the available (non-reserved) anonymous balance into the user's hosted credit account via a single idempotent transfer (`anonymous_transfer_in`/`anonymous_transfer_out` ledger entries, keyed by `anonymous-transfer:{fp}:{user}`); reserved credits remain on the fingerprint account so in-flight settlements can finish.
- `license check` now returns `quota_snapshot.credit_account` (`owner_kind` / `balance` / `reserved` / `available`) instead of `free_trial` / `free_trial_daily`. `CheckResponse.FreeLimit/FreeUsed/FreeRemaining` and `ConsumeResponse.FreeUsed/FreeRemaining` are gone.
- CLI status, whoami, and TUI footers print the credit-account view: `Anonymous credit balance (this device): X available / Y reserved / Z total` instead of "Free trial quota (this machine, lifetime)". Logout copy is now "back to anonymous credit mode."
- Admin web removes the "Free Trial Devices" page and the `/free-quotas` route; QuotaSources surfaces only reward / paid / hosted credentials. Admin backend removes `ListFreeQuotas` / `UpdateFreeQuota`. Dashboard "Total Machines" stat now counts distinct rows in `fingerprint_credit_accounts`.

### Migration

- Postgres migrations `023_fingerprint_credit_accounts.sql` (create) and `024_drop_free_quotas.sql` (drop legacy `daily_free_quotas` / `free_quotas`). Historical anonymous trial counts are not backfilled — they are deprecated.

## 0.2.78 - 2026-05-23

### Changed

- New-user signup hosted credit bonus increased from 30 to 100 credits. Existing users are unaffected (the grant is idempotent per `signup-hosted-credits:<userID>`); the overview UI and web fallback both reflect the new amount.
- TUI footer no longer shows the anonymous "Trial: N generations" counter while running in External Mode once the device's free quota is exhausted or the user is signed in — Trial state only makes sense for fresh hosted-trial sessions.
- Image edit (`new img --reference-image`) now accepts OpenAI responses that return a `url` field in addition to `b64_json`, and surfaces the response body in error messages when decoding fails.

## 0.2.77 - 2026-05-23

### Fixed

- `officecli upgrade` no longer auto-runs `npm install -g officecli` without consent. In an interactive shell it asks before applying; in a non-interactive context it prints the suggested command and exits. Pass `--apply` (or `-y`) to keep the previous one-shot behavior.
- Generation commands no longer fail with "missing account login" when the binary has a valid CLI session but the locally installed `~/.codex/skills/officecli/env-common.sh` is an older copy that doesn't recognize the binary's `Mode: logged in` output. The preflight now double-checks the binary's own config and overrides the stale shell verdict when an active session or API key is present.
- `officecli new ...` rejects the combination of `--prompt` and `--prompt-file` with a clear error instead of silently ignoring the file.
- The local dev-build license-proof skip warning is now emitted at most once per process instead of repeating on every access check.

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
