# officecli-dev 测试环境本地联调

`officecli-dev` 是连接测试环境的本地 CLI 入口。它和正式 `officecli` 使用同一套 Go 逻辑，但默认启用 dev profile，避免测试环境登录态、API key、发布预览配置污染生产配置。

## 默认连接

- Platform base URL: `https://officecli.shimodev.com`
- Config file: `~/.config/officecli-dev/config.json`
- Runtime mode: `hosted`
- License/access check: enabled
- Online preview publishing: enabled

`officecli.shimodev.com` 可以经过网关层转发到测试环境；`officecli-dev doctor` 不要求域名直接解析到业务服务器 IP。

## 本地源码构建

```bash
bash scripts/build-officecli-dev.sh
dist/dev/officecli-dev doctor
```

如需临时指定测试环境入口：

```bash
OFFICECLI_DEV_PLATFORM_BASE_URL=https://officecli.shimodev.com bash scripts/build-officecli-dev.sh
```

## 常用联调命令

```bash
officecli-dev doctor
officecli-dev login
officecli-dev whoami
officecli-dev auth status
officecli-dev new pptx "测试环境联调" --json
```

生成命令应通过测试环境 Hosted LLM 完成，并返回 `officecli.shimodev.com` 下的 preview URL。对象存储仍由测试环境 namespace 内的 MinIO 承接，CLI 不直接连接 k3s 内部 Service。

## TLS 排查

默认使用真实 HTTPS 证书访问 `https://officecli.shimodev.com`。如果测试环境网关或证书临时异常，可仅对 dev profile 使用：

```bash
OFFICECLI_DEV_INSECURE_TLS=1 officecli-dev doctor
```

该开关只在 `OFFICE_CLI_PROFILE=dev` 时生效，不影响正式 `officecli`。

## 验收

```bash
officecli-dev doctor
officecli-dev login
officecli-dev whoami
officecli-dev auth status
officecli-dev new pptx "测试环境联调" --json
officecli config status
```

最后一步用于确认生产 `officecli` 配置仍指向独立的正式配置文件，未被测试环境登录覆盖。
