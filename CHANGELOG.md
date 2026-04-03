# Changelog

## 0.1.0 - 2026-03-31

首个可用的 CLI 版本，重点是把仓库从可复用库推进到可直接给人使用的命令行工具。

### Added

- 新增 `officecli new <pptx|docx|xlsx> <topic> [brief]` 命令入口
- 新增 `--prompt`、`--prompt-file`、`--mode`、`--lang`、`--style`、`--audience`、`--out`、`--publish`、`--no-publish`、`--json`
- 新增默认人类可读输出和 `--json` 结构化输出
- 新增 `--help`、`--version` 与构建时版本注入
- 新增 `internal/providers/llm`，支持 OpenAI 兼容接口和内部 HTTP provider
- 新增 `internal/providers/publish`，支持生成后发布并返回访问地址/密码
- 新增 `examples/` 下的配置和三类提示词示例
- 新增 `Makefile`，支持 `build/test/install/run-help/demo/release`
- 新增 `scripts/demo.sh`，用于本地演示完整 CLI 流程

### Changed

- README 改写为面向人类用户的使用教程
- 引擎库之上新增运行时接线层，使 `pptx/docx/xlsx` 都能通过统一 CLI 执行

### Notes

- 当前发布目标默认产出 `darwin` 和 `linux` 的 `amd64/arm64` 二进制到 `dist/`
- 当前版本号默认是 `dev`，建议通过 `make build VERSION=...` 或 `make release VERSION=...` 注入
