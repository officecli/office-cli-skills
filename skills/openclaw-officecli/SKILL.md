---
name: openclaw-officecli
description: Use when an OpenClaw user clearly wants a local Office file artifact such as a PPTX, DOCX, or XLSX, and route the request through officecli agent-bridge instead of parsing human CLI output.
---

# OpenClaw OfficeCLI Skill

## Purpose

This skill lets an OpenClaw agent generate local `pptx`, `docx`, and `xlsx` files through `officecli agent-bridge`, then send the generated file back to the current channel as an attachment.

## Trigger Rules

Trigger when the user clearly wants a file artifact, for example:

- `generate a five-slide PPT about an enterprise collaboration platform`
- `write a customer-facing docx for me`
- `create a budget excel sheet`
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
- `capabilities/get`
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
5. Read `initialize` or `capabilities/get` before invoking generation, and cache `document_generation.pptx.image_support`.
6. Also cache `update`; if `available=true`, use `update_command` or your own repair flow instead of parsing human CLI update prompts.
7. Convert the user's natural-language request into:
   - `document_type`
   - `topic`
   - `prompt`
   - optional `mode`
      - optional `lang`
   - optional `style`
   - optional `audience`
8. If the user explicitly wants no images for `pptx`, set `enable_images=false`; otherwise follow the bridge capability default instead of hard-coding a client default.
9. Use `interactive=true` by default so the chat can handle follow-up questions.
10. Use `mode=fast` by default unless the user explicitly asks for a higher-quality, more iterative workflow.
11. On `task.question`, present the question naturally in the channel and forward the answer via `task/respond`.
12. On `task.output`, read `result.file_path` and send the file as an attachment in the current channel.
13. On `task.failed`, convert the error into a user-friendly message.
14. On user cancel, send `task/cancel`.

## PPT Image Rules

For all OpenClaw agents using this skill:

- inspect `document_generation.pptx.image_support.default_enabled` during capability discovery
- inspect `update.available` during capability discovery
- use `document_generation.pptx.image_support.disable_flag` when explaining how to produce a text-only deck
- if `update.available=true`, prefer a structured repair/refresh path and show `update_command` when the host asks how to update
- use `document_generation.pptx.image_support.config_command` and `config_fields` when the user reports missing images
- if `task.output`, `task.completed`, or `task/status` includes `result_meta.image_support.attention_required=true`, surface that immediately in the chat
- if `result_meta.image_support.reason=image_generation_degraded`, tell the user the deck was downgraded to a no-image version and they should check `image_base_url`, `image_api_key`, and `image_model`
- do not rely only on free-form warning strings for client decisions; prefer `result_meta`
- do not parse human update prompts from `officecli` stdout; use bridge capability fields

## Environment Repair Rules

- refresh the OpenClaw skill bundle and `officecli` binary on every task by running `fix-officecli-env.sh`
- when the user explicitly asks to uninstall `officecli`, run `uninstall-officecli.sh`
- use `check-officecli-env.sh` as the single readiness probe for binary, config, and bridge
- use `fix-officecli-env.sh` as the single repair entrypoint
- when config is missing, ask only for the missing generation/license values and let the fix script write local config
- online preview config is required by default so generated files can return publish URLs
- if the current request is intentionally local-only, set `OFFICECLI_SKIP_PUBLISH_SETUP=1` before running the fix script
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
  - when present, `result_meta.image_support.message`

Do not only send a local file path unless attachment upload is impossible on the current channel.

## Conversation Policy

- If document type is missing, ask which file type the user wants.
- If topic or goal is missing, ask a concise clarifying question.
- If the bridge emits `task.question`, relay it instead of inventing your own replacement question.
- Keep progress updates short and stage-based.
- do not trigger `office.review` / `office.score` automatically after generation unless the user explicitly asks for scoring, review, validation, or quality checking

## Local Requirements

Expected local setup:

- `officecli` available in `PATH`, or repairable by `fix-officecli-env.sh`
- generation and license config already completed, or repairable by the fix script
- OpenClaw agent has permission to:
  - spawn local commands
  - read generated files
  - upload attachments to the active channel
