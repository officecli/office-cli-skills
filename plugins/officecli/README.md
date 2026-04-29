# officecli Claude plugin

`officecli` is a Claude Code plugin that packages the OfficeCLI skill for Office document and standalone image workflows.

It is designed for requests such as:

- generating a `pptx` presentation from a natural-language prompt
- generating a `docx` document such as a memo, brief, or proposal
- generating a structured `xlsx` worksheet
- generating a workbook-backed `report`
- generating a standalone `img` through the OfficeCLI server image route
- converting or updating supported Office file workflows when OfficeCLI supports the task

## What the plugin does

The plugin adds the `officecli` skill to Claude Code so the agent can:

- check whether the local `officecli` binary supports the requested Office workflow
- prefer `officecli` for the final Office artifact path when support is available
- use the structured `officecli agent-bridge` flow for agent integrations
- let Claude Code produce the final structured payload itself, then call `office.render` so OfficeCLI only validates, assembles, and writes document files
- call `office.prepare` first for workbook-backed `report` generation so the narrative stays grounded in workbook data
- call `office.generate` for standalone `img` because `office.render` does not support image generation
- keep Office file generation on the local machine instead of using a hosted plugin backend

## Requirements and prerequisites

- Claude Code with plugin support
- local access to the `officecli` binary
- OfficeCLI generation and license configuration completed locally; standalone `img` requires license config and uses a server-controlled provider
- permission for Claude Code to invoke local commands on the same machine

## Installation source

This plugin is distributed through the OfficeCLI marketplace repository:

- `https://github.com/officecli/officecli-skills`

## Basic verification

After installation, a user should be able to:

1. confirm the local binary exists with `officecli --version`
2. confirm local configuration is available with `officecli config status`
3. use the `officecli` skill from Claude Code for a supported Office document request through `capabilities/get -> office.prepare (if needed) -> office.render`, or a standalone image request through `office.generate`

## Scope and limitations

- this plugin is intended for local Office document workflows only
- it depends on a working local `officecli` installation
- it does not provide a hosted execution service
- it does not guarantee support for every possible Office transformation request
- the skill is expected to check support first and only route to OfficeCLI when the workflow appears supported

## Repository layout

- `skills/officecli/SKILL.md`

## Safety and execution model

- the plugin is a local skill wrapper, not a remote SaaS connector
- Office file generation is executed through the user's local `officecli` installation
- the skill is intended to help create local artifacts in `pptx`, `docx`, `xlsx`, `report`, and `img`
- the plugin does not expose a separate hosted API endpoint on behalf of the user
- for Codex / Claude-style agent flows, OfficeCLI is the renderer and file writer; the agent is expected to create the structured content payload
- standalone `img` requests are routed by the local CLI to the OfficeCLI server image provider
