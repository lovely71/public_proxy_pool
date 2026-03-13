# public_proxy_pool

一个基于 Go 的公共代理聚合服务，支持抓取、检测、评分、多格式订阅导出、中文 Web UI、Docker 部署，以及通过 GitHub Actions 自动构建 Docker 镜像。

## 功能概览

- 聚合公共代理源与订阅源，统一入库
- 对节点做有效性、延迟、国家、纯净度等检测
- 导出 `JSON`、`plain`、`Clash`、`V2Ray` 等格式
- 内置中文 Web 管理后台，支持手机和桌面自适应
- 支持 `SQLite`、Docker、GitHub Actions 构建镜像

## 目录说明

- `cmd/proxypool`：主程序入口
- `internal/`：核心业务代码
- `scripts/run_light.sh`：适合 `1c1g` 机器的轻量启动脚本
- `docker-compose.yml`：容器部署示例
- `.github/workflows/docker-image.yml`：GitHub Actions Docker 镜像构建流程

## 配置准备

推荐先复制环境变量模板：

```bash
cp .env.example .env
```

最少需要配置：

```bash
API_KEYS=replace-with-a-strong-token
HTTP_ADDR=:38482
SQLITE_PATH=./data/proxypool.db
```

说明：

- `API_KEYS`：后台和 API 鉴权 token，可写多个，逗号分隔
- `HTTP_ADDR`：监听端口，建议用不常用高位端口，例如 `:38482`
- `SQLITE_PATH`：SQLite 数据文件路径
- `.env` 已加入 `.gitignore`，不会被提交

## 部署流程

### 方案一：本机轻量部署

适合单机、开发机、`1c1g` 云主机直接运行。

1. 准备环境

```bash
go version
```

建议使用项目 `go.mod` 对应的 Go 版本。

2. 配置 `.env`

```bash
cp .env.example .env
```

至少填写：

```bash
API_KEYS=your-token
HTTP_ADDR=:38482
SQLITE_PATH=./data/proxypool.db
```

3. 启动服务

```bash
./scripts/run_light.sh
```

这个脚本默认会：

- 自动读取根目录 `.env`
- 限制抓取和验证并发，适合小内存机器
- 默认关闭高开销的 `NodeMaven` 和纯净度外部查询

4. 验证服务

```bash
curl http://127.0.0.1:38482/healthz
curl -H 'X-API-Key: your-token' 'http://127.0.0.1:38482/api/v1/stats'
```

5. 访问后台

浏览器访问时要带 `token`：

```text
http://127.0.0.1:38482/ui/overview?token=your-token
```

原因是浏览器地址栏不会自动携带 `X-API-Key` 请求头。

### 方案二：Docker Compose 部署

适合服务器长期运行。

1. 准备 `.env`

```bash
cp .env.example .env
```

建议最少配置：

```bash
API_KEYS=your-token
HTTP_ADDR=:8080
```

默认推荐直接使用已经发布的公共镜像：

```text
ghcr.io/lovely71/public_proxy_pool:latest
```

如果你要暴露成不常用端口，直接改 compose 的端口映射，例如：

```yaml
ports:
  - "38482:8080"
```

2. 启动容器

如果 `docker-compose.yml` 使用公共镜像：

```yaml
services:
  proxypool:
    image: ghcr.io/lovely71/public_proxy_pool:latest
    container_name: proxypool
    restart: unless-stopped
    ports:
      - "38482:8080"
    environment:
      - API_KEYS=${API_KEYS}
      - AUTO_FETCH_ENABLED=true
      - AUTO_VALIDATE_ENABLED=true
    volumes:
      - ./data:/data
```

先拉镜像，再启动：

```bash
docker compose pull
docker compose up -d
```

如果你想在本地基于源码重新构建，再使用：

```bash
docker compose up -d --build
```

3. 查看日志

```bash
docker compose logs -f proxypool
```

4. 验证服务

```bash
curl http://127.0.0.1:38482/healthz
curl -H 'X-API-Key: your-token' 'http://127.0.0.1:38482/api/v1/stats'
```

5. 数据目录

默认数据挂载在：

```text
./data
```

如果你启用离线 GeoIP，也可以额外挂载：

```text
./geoip:/geoip:ro
```

### 方案三：使用 GitHub Actions 自动构建镜像

仓库内置了：

- [.github/workflows/docker-image.yml](.github/workflows/docker-image.yml)

触发规则：

- 推送到 `main` 时构建镜像
- 推送 `v*` 标签时构建并打版本标签
- `pull_request -> main` 时仅构建校验，不推送

镜像当前默认推送到：

```text
ghcr.io/lovely71/public_proxy_pool
```

常用标签建议：

```text
ghcr.io/lovely71/public_proxy_pool:latest
ghcr.io/lovely71/public_proxy_pool:main
ghcr.io/lovely71/public_proxy_pool:sha-<commit>
```

如果你已经把仓库推到 GitHub，后续可以直接在服务器上拉镜像部署：

```bash
docker pull ghcr.io/lovely71/public_proxy_pool:latest
docker run -d \
  --name proxypool \
  -p 38482:8080 \
  -e API_KEYS=your-token \
  -v $(pwd)/data:/data \
  ghcr.io/lovely71/public_proxy_pool:latest
```

## 部署后检查清单

上线后建议按这个顺序检查：

1. 健康检查是否正常

```bash
curl http://127.0.0.1:38482/healthz
curl http://127.0.0.1:38482/readyz
```

2. 鉴权是否生效

```bash
curl -i http://127.0.0.1:38482/api/v1/stats
curl -i -H 'X-API-Key: your-token' http://127.0.0.1:38482/api/v1/stats
```

3. 后台是否可访问

```text
http://127.0.0.1:38482/ui/overview?token=your-token
```

4. 订阅是否能返回内容

```bash
curl -H 'X-API-Key: your-token' 'http://127.0.0.1:38482/sub/clash?verify=1&limit=10'
curl -H 'X-API-Key: your-token' 'http://127.0.0.1:38482/sub/v2ray?verify=1&limit=10'
```

## 常用环境变量

- `HTTP_ADDR`：监听地址，例如 `:38482`
- `SQLITE_PATH`：SQLite 文件路径
- `API_KEYS`：鉴权 token，多个值用逗号分隔
- `PUBLIC_BASE_URL`：用于 `/probe/echo` 与匿名度检测
- `AUTO_FETCH_ENABLED`：是否自动抓取
- `AUTO_VALIDATE_ENABLED`：是否自动检测
- `GEOIP_COUNTRY_MMDB`：国家 GeoIP 数据库路径
- `GEOIP_ASN_MMDB`：ASN GeoIP 数据库路径
- `V2RAY_VALIDATE_MODE`：`tcp` 或 `sing-box`
- `SING_BOX_PATH`：`sing-box` 可执行文件路径

完整变量可参考：

- [.env.example](.env.example)
- [internal/config/config.go](internal/config/config.go)

## 常用命令

本机运行：

```bash
./scripts/run_light.sh
```

测试：

```bash
go test ./...
```

接口烟测：

```bash
bash scripts/smoke.sh
```

Docker 部署：

```bash
docker compose pull
docker compose up -d
```

源码重新构建 Docker：

```bash
docker compose up -d --build
```

## Oracle Cloud Ubuntu 一键部署

如果你是在一台全新的 Oracle Cloud Ubuntu 主机上部署，最省事的方式是直接跑仓库里的自动部署脚本。脚本会自动：

- 安装 Docker 和 Docker Compose
- 创建 `/opt/public_proxy_pool`
- 生成 `.env` 和 `docker-compose.yml`
- 拉取公共镜像 `ghcr.io/lovely71/public_proxy_pool:latest`
- 以更适合小机的保守参数启动服务
- 做健康检查并输出后台访问地址

### 用法一：先把仓库传上服务器再执行

```bash
sudo bash scripts/deploy_oracle_ubuntu.sh
```

如果你想自定义端口或 token：

```bash
sudo HOST_PORT=38482 API_KEY='your-strong-token' PUBLIC_BASE_URL='http://YOUR_SERVER_IP:38482' \
  bash scripts/deploy_oracle_ubuntu.sh
```

### 用法二：本地直接通过 SSH 远程执行

```bash
ssh ubuntu@YOUR_SERVER_IP 'sudo bash -s' < scripts/deploy_oracle_ubuntu.sh
```

带自定义参数的例子：

```bash
ssh ubuntu@YOUR_SERVER_IP \
  "sudo env HOST_PORT=38482 API_KEY='your-strong-token' bash -s" \
  < scripts/deploy_oracle_ubuntu.sh
```

部署完成后，脚本会输出：

- 本机检查命令
- API Token
- Web 后台地址：`http://公网IP:端口/ui/overview?token=你的token`

### Oracle Cloud 额外注意事项

脚本只能处理主机内部环境，Oracle 云控制台里的网络策略还需要你手动放行：

- 在实例所在子网的 `Security List` 或绑定的 `NSG` 中，放行入站 `TCP/你的端口`
- 如果主机自己启用了 `ufw`，脚本会自动尝试放行该端口

脚本默认使用轻量参数，适合小规格机器。如果你后面想调高抓取/验证并发，可以直接修改服务器上的：

```text
/opt/public_proxy_pool/.env
```

然后执行：

```bash
cd /opt/public_proxy_pool
docker compose up -d
```
