# Changelog

## Unreleased

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
