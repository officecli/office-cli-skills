# OfficeCLI Skill for Codex and Local Agents

`officecli` is the public skill bundle for local agent runtimes that want to route supported Office
document tasks into a local `officecli` runtime.

Use this skill when the request is about:

- generating `pptx`, `docx`, or `xlsx` outputs
- routing workbook-backed report workflows through OfficeCLI
- deciding whether a local Office task should use OfficeCLI instead of a generic script

Install details and runtime-specific entrypoints:

- Overview: `https://officecli.io/officecli-skills`
- Install: `https://officecli.io/officecli-skills/install`
- Codex: `https://officecli.io/officecli-skills/codex`
- Claude Code: `https://officecli.io/officecli-skills/claude-code`
- OpenClaw: `https://officecli.io/officecli-skills/openclaw`

The public skill bundle is a routing layer, not a hosted execution backend. Final Office file generation
still depends on a working local `officecli` installation.
