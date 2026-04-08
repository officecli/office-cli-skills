# OfficeCLI 闭源分发架构

本文档描述 `officecli` 在“源码闭源、二进制公开分发”前提下的推荐仓库结构与发布流水线。

## 仓库角色

### 1. 私有源码仓

当前仓库保持 private，负责：

- 源码
- 测试与 CI
- 正式构建
- GoReleaser 配置
- 用于同步公共仓的脚本与模板

### 2. 公共分发仓

当前仓库名：`officecli/officecli-dist`

职责：

- GitHub Releases
- 二进制压缩包
- `checksums.txt`
- Linux 安装脚本
- 面向终端用户的最小安装说明

发布形态：

- 版本化 release：`vX.Y.Z`
- 滚动 latest release：`latest`

注意：

- 不提交源码
- 不提交源码 patch
- 不提交会暴露源码结构的构建中间文件

### 3. 公共 Homebrew tap 仓

当前仓库名：`officecli/homebrew-officecli`

职责：

- 存放 `Formula/officecli.rb`
- 指向公共分发仓 release asset URL

### 4. 公共 skills 仓

当前仓库名：`officecli/officecli-skills`

职责：

- 同步 `skills/` 目录
- 对外提供 `SKILL.md` 与示例说明
- 不包含闭源实现细节

## 发布流水线

1. 在私有源码仓打 `vX.Y.Z` tag
2. GitHub Actions 触发 `CLI Release`
3. `GoReleaser` 构建 darwin/linux x amd64/arm64 资产并发布到公共分发仓 Releases
4. 同步公共分发仓的 README 与 Linux 安装脚本
5. 更新公共 Homebrew tap 仓 formula

另有一条滚动最新分发链路：

1. 私有源码仓 `main` 更新
2. GitHub Actions 触发 `CLI Publish Latest`
3. 构建 darwin/linux x amd64/arm64 的 `officecli_latest_*` 资产
4. 覆盖公共分发仓 `latest` release
5. 同步公共分发仓 README 与 Linux 安装脚本

## 需要配置的仓库变量

建议在私有源码仓配置以下 repository variables：

- `PUBLIC_DIST_REPO_OWNER=officecli`
- `PUBLIC_DIST_REPO_NAME=officecli-dist`
- `PUBLIC_DIST_REPO=officecli/officecli-dist`
- `PUBLIC_DIST_DEFAULT_BRANCH=main`
- `HOMEBREW_TAP_REPO=officecli/homebrew-officecli`
- `HOMEBREW_TAP_DEFAULT_BRANCH=main`
- `PUBLIC_SKILLS_REPO=officecli/officecli-skills`
- `PUBLIC_SKILLS_DEFAULT_BRANCH=main`

## 需要配置的仓库 secrets

- `PUBLIC_DIST_REPO_TOKEN`
- `HOMEBREW_TAP_TOKEN`
- `PUBLIC_SKILLS_REPO_TOKEN`

这些 token 至少需要对应公共仓的写权限。

### 推荐最小权限对应关系

- `PUBLIC_DIST_REPO_TOKEN`：对 `officecli/officecli-dist` 有 contents write
- `HOMEBREW_TAP_TOKEN`：对 `officecli/homebrew-officecli` 有 contents write
- `PUBLIC_SKILLS_REPO_TOKEN`：对 `officecli/officecli-skills` 有 contents write

## 本地验证

发布前至少验证：

```bash
go test ./...
bash -n scripts/install-officecli.sh
bash -n scripts/sync-public-dist-repo.sh
bash -n scripts/sync-homebrew-tap.sh
bash -n scripts/sync-public-skills-repo.sh
```

## 公开边界

公共仓不得包含：

- 私有源码仓地址
- 私有模块路径
- 内部 CI 密钥名以外的业务凭据
- 部署环境信息
- 内部文档与提交历史
