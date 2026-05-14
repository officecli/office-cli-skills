# OfficeCLI Skill for Codex and Local Agents

`officecli` is the public skill bundle for local agent runtimes that want to route supported Office
document tasks into a local `officecli` runtime.

Use this skill when the request is about:

- generating `pptx`, `docx`, or `xlsx` outputs
- routing workbook-backed report workflows through OfficeCLI
- deciding whether a local Office task should use OfficeCLI instead of a generic script
- checking or changing the OfficeCLI default runtime with `officecli config runtime` or `officecli config set-runtime external|hosted`

Install details and runtime-specific entrypoints:

- Overview: `https://officecli.io/officecli`
- Install: `https://officecli.io/officecli/install`
- Codex: `https://officecli.io/officecli/codex`
- Claude Code: `https://officecli.io/officecli/claude-code`
- OpenClaw: `https://officecli.io/officecli/openclaw`

The public skill bundle is a routing layer, not a hosted execution backend. Final Office file generation
still depends on a working local `officecli` installation.
Hosted runtime still uses the user's OfficeCLI platform API key and hosted credits; aigateway keys are managed by the platform and are not exposed to users.
