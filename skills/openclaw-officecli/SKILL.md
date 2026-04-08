---
name: openclaw-officecli
description: Use when an OpenClaw user clearly wants a local Office file artifact such as a PPTX, DOCX, or XLSX, and route the request through officecli agent-bridge instead of parsing human CLI output.
---

# OpenClaw OfficeCLI Skill

## Purpose

This skill lets an OpenClaw agent generate local `pptx`, `docx`, and `xlsx` files through `officecli agent-bridge`, then send the generated file back to the current channel as an attachment.

## Trigger Rules

Trigger when the user clearly wants a file artifact, for example:

- `生成一个五页的 PPT，介绍企业协作平台`
- `帮我写一份给客户的 docx`
- `做一个预算 excel 表`
- `turn this into slides`
- `write a docx for customers`

Do not trigger for:

- pure explanation
- brainstorming
- outline-only requests
- analysis without a file deliverable

## Runtime Contract

The skill must use `officecli agent-bridge` as the local execution protocol.

Do not treat `officecli new ...` stdout as a protocol.

Always prefer the structured bridge:

- transport: `stdio`
- framing: `Content-Length`
- protocol: `JSON-RPC 2.0`
- tool: `office.generate`

Required bridge methods:

- `initialize`
- `session/open`
- `task/invoke`
- `task/respond`
- `task/status`
- `task/cancel`

Primary event types:

- `task.started`
- `task.progress`
- `task.question`
- `task.output`
- `task.completed`
- `task.failed`
- `task.cancelled`

## Agent Behavior

1. Run `fix-officecli-env.sh` before starting any bridge session so the skill bundle and binary are refreshed on every task.
2. Run `check-officecli-env.sh` after the refresh step.
3. Ensure `officecli` is installed, configured, and reachable.
4. Ensure `officecli agent-bridge` can be started locally.
5. Convert the user's natural-language request into:
   - `document_type`
   - `topic`
   - `prompt`
   - optional `mode`
      - optional `lang`
   - optional `style`
   - optional `audience`
6. Use `interactive=true` by default so the chat can handle follow-up questions.
7. Use `mode=fast` by default unless the user explicitly asks for a higher-quality, more iterative workflow.
8. On `task.question`, present the question naturally in the channel and forward the answer via `task/respond`.
9. On `task.output`, read `result.file_path` and send the file as an attachment in the current channel.
10. On `task.failed`, convert the error into a user-friendly message.
11. On user cancel, send `task/cancel`.

## Environment Repair Rules

- refresh the OpenClaw skill bundle and `officecli` binary on every task by running `fix-officecli-env.sh`
- when the user explicitly asks to uninstall `officecli`, run `uninstall-officecli.sh`
- use `check-officecli-env.sh` as the single readiness probe for binary, config, and bridge
- use `fix-officecli-env.sh` as the single repair entrypoint
- when config is missing, ask only for the missing generation/license values and let the fix script write local config
- online preview config is optional unless the current request needs publish URLs
- do not try to start `agent-bridge` until the check script returns ready
- if refresh or check fails, stop and report the `officecli` environment error; do not fall back to any other PPT/DOC/XLS generation tool without explicit user approval

## Attachment Delivery

When generation succeeds:

- read `task.output.payload.result.file_path`
- upload that file to the current channel
- include a short note with:
  - document type
  - document name
  - any warnings returned by bridge

Do not only send a local file path unless attachment upload is impossible on the current channel.

## Conversation Policy

- If document type is missing, ask which file type the user wants.
- If topic or goal is missing, ask a concise clarifying question.
- If the bridge emits `task.question`, relay it instead of inventing your own replacement question.
- Keep progress updates short and stage-based.

## Local Requirements

Expected local setup:

- `officecli` available in `PATH`, or repairable by `fix-officecli-env.sh`
- generation and license config already completed, or repairable by the fix script
- OpenClaw agent has permission to:
  - spawn local commands
  - read generated files
  - upload attachments to the active channel
