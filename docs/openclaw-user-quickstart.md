# OpenClaw User Quickstart

This quickstart explains how to use OfficeCLI from an OpenClaw integration without relying on local implementation details that only exist in skills.

## Prerequisites

- a working `officecli` binary
- generation configuration completed through the CLI
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

## Important Rules

- User-visible output must remain English-only.
- Binary behavior is the source of truth; do not rely on skill-only wording or wrappers to change product semantics.
- If the platform returns preview links, passwords, or quota warnings, that output should already be normalized by the binary.
