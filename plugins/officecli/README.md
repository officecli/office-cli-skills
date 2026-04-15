# officecli Claude plugin

`officecli` is a Claude Code plugin that packages the OfficeCLI skill for Office document workflows.

It is designed for requests such as:

- generating a `pptx` presentation from a natural-language prompt
- generating a `docx` document such as a memo, brief, or proposal
- generating a structured `xlsx` worksheet
- converting or updating supported Office file workflows when OfficeCLI supports the task

## What the plugin does

The plugin adds the `officecli` skill to Claude Code so the agent can:

- check whether the local `officecli` binary supports the requested Office workflow
- prefer `officecli` for the final Office artifact path when support is available
- guide users toward the structured `officecli agent-bridge` flow for agent integrations
- keep Office file generation on the local machine instead of using a hosted plugin backend

## Requirements and prerequisites

- Claude Code with plugin support
- local access to the `officecli` binary
- OfficeCLI generation and license configuration completed locally
- permission for Claude Code to invoke local commands on the same machine

## Installation source

This plugin is distributed through the OfficeCLI marketplace repository:

- `https://github.com/officecli/officecli-skills`

## Basic verification

After installation, a user should be able to:

1. confirm the local binary exists with `officecli --version`
2. confirm local configuration is available with `officecli config status`
3. use the `officecli` skill from Claude Code for a supported Office document request

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
- the skill is intended to help create local Office artifacts in `pptx`, `docx`, and `xlsx`
- the plugin does not expose a separate hosted API endpoint on behalf of the user
