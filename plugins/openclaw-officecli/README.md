# openclaw-officecli Claude plugin

This plugin packages the `openclaw-officecli` skill for Claude Code.

Skill path:

- `skills/openclaw-officecli/SKILL.md`

Agent integration note:

- read `initialize` / `capabilities/get` first
- use `document_generation.pptx.image_support` as the machine-readable PPT image contract
- when `result_meta.image_support.attention_required=true`, guide the user to `officecli config set-generation` or `--no-images`
