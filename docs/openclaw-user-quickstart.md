# OpenClaw User Quickstart

This quickstart explains how to use OfficeCLI from an OpenClaw integration without relying on local implementation details that only exist in skills.

## Prerequisites

- a working `officecli` binary
- generation configuration completed through the CLI for External Mode document generation and PPT/IMG image assets
- license configuration completed through the CLI only when using Hosted Mode, hosted credits, or online publishing credentials
- optional publish configuration if online preview links are required

## Basic Flow

1. Confirm the binary is available:

```bash
officecli version
```

2. Confirm configuration:

```bash
officecli config status
```

3. Generate a presentation:

```bash
officecli new pptx "Product Introduction" --prompt-file ./prompt.txt --lang en-US --style "Brand Showcase" --audience "General business and classroom presentations" --out output --json --mode fast
```

4. For a text-only deck, disable generated images:

```bash
officecli new pptx "Product Introduction" --prompt-file ./prompt.txt --no-images --out output
```

5. PPTX generation automatically scans `.pptx` files under the current working directory and uses them as a compact reference style profile. To control this behavior:

```bash
officecli new pptx "Product Introduction" --prompt-file ./prompt.txt --reference-root ./brand-assets --reference-pptx ./templates/company.pptx --out output
officecli new pptx "Product Introduction" --prompt-file ./prompt.txt --no-reference-scan --out output
officecli new pptx "Product Introduction" --prompt-file ./prompt.txt --pptx-backend officegen --no-images --out output
```

When `--json` is used, PPTX results may include `reference_style`, `pptx_review`, and `pptx_backend` metadata. `reference_style` contains aggregate counts and safe style intent, not raw PPTX XML or copied source slides. `pptx_review` reports the structural review score produced after generation. The only supported PPTX backend is `officegen`.

6. Generate a standalone image:

```bash
officecli new img "Product Launch Visual" --prompt "Create a polished launch image for an enterprise collaboration platform." --ratio landscape --reference-image ./reference.png --out output --json
```

## Important Rules

- User-visible output must remain English-only.
- Binary behavior is the source of truth; do not rely on skill-only wording or wrappers to change product semantics.
- External Mode standalone `img` generation uses the local `config set-generation` image provider. Hosted Mode standalone `img` uses the OfficeCLI-managed runtime and hosted credits.
- Standalone `img` supports one `--reference-image <path-or-url>` for local or remote reference images.
- PPTX reference style learning is separate from `--reference-image`: use `--reference-root`, `--reference-pptx`, and `--no-reference-scan` only with `document_type=pptx`.
- PPTX `--pptx-backend` only accepts `officegen`.
- Standalone `img` publishes online by default when publishing is configured; pass `--no-publish` for local-only output.
- If the platform returns preview links, passwords, or quota warnings, that output should already be normalized by the binary.
