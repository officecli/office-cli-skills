# platform 生产发布说明

本文档记录 `platform/` 当前生产环境的实际发布流程。目标是让后续 agent 可以直接按文档操作，而不是重新猜部署方式。

## 推荐入口

如仓库主线存在 GitHub Actions 工作流 `Platform Deploy`（`.github/workflows/platform-deploy.yml`），优先用该 workflow 发版；它本质上仍调用仓库脚本 `scripts/deploy-platform-prod.sh` 作为底层执行器。

当前可用的手动入口：

- GitHub Actions `workflow_dispatch`，传入 `release_tag`
- 或在本地直接执行仓库脚本

优先使用仓库脚本：

```bash
./scripts/deploy-platform-prod.sh
```

如果要手动指定 tag：

```bash
./scripts/deploy-platform-prod.sh v0.1.0-prod-20260401-2
```

脚本会自动完成这些步骤：

- `go test ./...`
- 构建 `web/app`、`web/admin`、`web/site`
- 构建 linux/amd64 backend 镜像并导出 tar.gz
- 上传镜像到服务器并执行 `sudo k3s ctr images import -`
- 同步 Nginx 静态目录
- 更新 `officecli-platform` Deployment
- 强制 `imagePullPolicy=Never`
- 强制 `strategy=Recreate`
- 执行发布后公网验证

只有在脚本不可用或需要排障时，才按本文后面的分步命令手工执行。

## 生产环境现状

- 服务器：`ubuntu@18.141.191.80`
- 域名：
  - `officecli.io`
  - `platform.officecli.io`
- Kubernetes：单节点 `k3s`
- Namespace：`officecli`
- Deployment：`officecli-platform`
- 服务监听：
  - 容器内 `:8080`
  - 主机 `hostPort: 29001`
- Nginx 负责公网入口与静态资源：
  - `officecli.io` 直接由 Nginx 托管官网静态文件
  - `platform.officecli.io` 的 `/app`、`/admin` 直接由 Nginx 托管前端静态文件
  - `/api/*`、`/healthz` 反代到 `127.0.0.1:29001`

## 关键事实

- 当前生产发布不是“推到远程镜像仓库再让集群拉取”。
- 当前生产发布依赖把镜像导入服务器本机的 `k3s containerd`。
- Deployment 当前使用：
  - `image: docker.io/library/officecli-platform:v0.1.0-prod-20260401-1` 这一类完整引用
  - `imagePullPolicy: Never`
  - `strategy: Recreate`
- `Recreate` 是必需的，因为 Pod 使用了 `hostPort: 29001`。如果还用 RollingUpdate，新旧 Pod 会抢同一个 hostPort，导致发布卡住。
- 如果只把镜像导入普通 `ctr -n k8s.io`，kubelet 不一定能看到；要导入 `sudo k3s ctr images import -`。

## 发布前检查

在本地仓库根目录执行：

```bash
cd platform
go test ./...
cd web/app && npm run build
cd ../admin && npm run build
cd ../site && npm run build
```

确认构建产物存在：

```bash
platform/web/app/dist
platform/web/admin/dist
platform/web/site/dist
```

## 生产发布步骤

### 1. 本地构建生产镜像

在 `platform/` 目录使用临时 Dockerfile 构建 linux/amd64 镜像：

```Dockerfile
FROM --platform=linux/amd64 golang:1.25-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/officecli-platform ./cmd/platform

FROM --platform=linux/amd64 alpine:3.20
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app
COPY --from=builder /out/officecli-platform /app/officecli-platform
COPY migrations /app/migrations
COPY web /app/web
EXPOSE 8080
ENTRYPOINT ["/app/officecli-platform"]
```

示例命令：

```bash
cd platform
TAG=officecli-platform:v0.1.0-prod-YYYYMMDD-N
docker buildx build --platform linux/amd64 -f /tmp/officecli-platform.Dockerfile -t "$TAG" --load .
docker save "$TAG" | gzip > /tmp/${TAG##*:}.tar.gz
```

注意：

- `platform/go.mod` 当前要求 `go >= 1.25.0`，builder 基础镜像不能低于 Go 1.25。

### 2. 上传镜像到服务器

```bash
scp /tmp/<tag>.tar.gz ubuntu@18.141.191.80:/opt/officecli-platform/
```

### 3. 导入到 k3s containerd

必须使用：

```bash
ssh ubuntu@18.141.191.80
cd /opt/officecli-platform
gzip -dc <tag>.tar.gz | sudo k3s ctr images import -
```

不要只用下面这个：

```bash
sudo ctr -n k8s.io images import -
```

原因：这样导入后 `crictl` 可能看不到镜像，Pod 会报 `ErrImageNeverPull` 或继续去远端拉取。

导入后验证：

```bash
sudo k3s ctr images ls | grep officecli-platform
sudo crictl images | grep officecli-platform
```

### 4. 更新 Deployment

设置新镜像，并确保 `imagePullPolicy=Never`：

```bash
kubectl -n officecli set image deployment/officecli-platform \
  platform=docker.io/library/officecli-platform:<tag>

kubectl -n officecli patch deployment officecli-platform --type json -p \
  '[{"op":"replace","path":"/spec/template/spec/containers/0/imagePullPolicy","value":"Never"}]'
```

如果 Deployment 不是 `Recreate`，先改回：

```bash
kubectl -n officecli patch deployment officecli-platform --type json -p \
  '[{"op":"remove","path":"/spec/strategy/rollingUpdate"},{"op":"replace","path":"/spec/strategy/type","value":"Recreate"}]'
```

### 5. 重建 Pod 并等待完成

```bash
kubectl -n officecli delete pod -l app=officecli-platform --force --grace-period=0
kubectl -n officecli rollout status deployment/officecli-platform --timeout=180s
kubectl -n officecli get pods -o wide
```

## Nginx 与静态资源

当前生产环境的公网前端不是完全依赖 Go 服务托管，而是由服务器 Nginx 直接托管。

### 官网静态资源

- 目录：`/var/www/officecli.io/dist`
- 站点：`/etc/nginx/sites-available/officecli.io`

需要更新官网时：

```bash
rsync -az --delete platform/web/site/dist/ ubuntu@18.141.191.80:/var/www/officecli.io/dist/
```

### 平台前端静态资源

- app：`/var/www/platform.officecli.io/app`
- admin：`/var/www/platform.officecli.io/admin`
- 站点：`/etc/nginx/sites-available/platform.officecli.io`

需要更新平台前端时：

```bash
rsync -az --delete platform/web/app/dist/ ubuntu@18.141.191.80:/var/www/platform.officecli.io/app/
rsync -az --delete platform/web/admin/dist/ ubuntu@18.141.191.80:/var/www/platform.officecli.io/admin/
```

如果改过 Nginx 配置，要执行：

```bash
sudo nginx -t
sudo systemctl reload nginx
```

## 发布后验证

### 公网验证

```bash
curl -I https://officecli.io/
curl -I https://officecli.io/api/pricing
curl -I https://officecli.io/app
curl -I https://platform.officecli.io/
curl -I https://platform.officecli.io/app/
curl -I https://platform.officecli.io/admin/
curl https://platform.officecli.io/healthz
```

预期：

- `officecli.io/` -> `200`
- `officecli.io/api/pricing` -> `200`
- `officecli.io/app` -> `302` 到 `platform.officecli.io/app`
- `platform.officecli.io/` -> `302` 到 `/app`
- `platform.officecli.io/app/` -> `200`
- `platform.officecli.io/admin/` -> `200`
- `platform.officecli.io/healthz` -> JSON 正常

### 服务器本机验证

```bash
curl -H 'Host: officecli.io' http://127.0.0.1:29001/
curl -H 'Host: platform.officecli.io' http://127.0.0.1:29001/app/
curl http://127.0.0.1:29001/healthz
kubectl -n officecli get pods -o wide
```

## 常见故障

### 1. Pod 一直 `Pending`

通常是旧 Deployment 用 `RollingUpdate`，新旧 Pod 同时争抢 `hostPort: 29001`。

处理：

- 把 Deployment 改成 `Recreate`
- 删除旧 Pod

### 2. `ImagePullBackOff`

通常是镜像名写了完整引用，但只导入到了错误的 containerd namespace，或者还在尝试远端拉取。

处理：

- 用 `sudo k3s ctr images import -` 重新导入
- 把 Deployment 镜像写成完整引用
- 把 `imagePullPolicy` 设为 `Never`

### 3. `ErrImageNeverPull`

说明 Deployment 设成了 `Never`，但 k3s 运行时里没有这张镜像。

处理：

- `sudo k3s ctr images ls | grep officecli-platform`
- 如果没有，重新导入镜像

### 4. `platform.officecli.io` 白屏

通常是前端 HTML 还在引用旧资源路径，或者静态目录没有更新。

处理：

- 重新构建 `platform/web/app` 和 `platform/web/admin`
- 同步到 `/var/www/platform.officecli.io/...`
- 确认 HTML 引用的是：
  - `/app/assets/...`
  - `/admin/assets/...`

## 当前已知偏差

- `officecli.io/api/pricing` 当前由 Nginx 静态返回默认套餐 JSON，不是后端动态提供。
- 原因是现网后端历史版本没有正确提供这个接口；目前这样做是为了保证官网 pricing 正常工作。
- 如果后续后端接口稳定了，可以把这部分从 Nginx 静态返回迁回应用层。
