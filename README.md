# public_proxy_pool

[![docker-image](https://github.com/lovely71/public_proxy_pool/actions/workflows/docker-image.yml/badge.svg)](https://github.com/lovely71/public_proxy_pool/actions/workflows/docker-image.yml)

`public_proxy_pool` 是一个基于 Go 的公共代理聚合服务：从多个公开代理源与订阅源抓取节点，统一入库、验活、评分，并导出 `JSON`、`plain`、`Clash`、`V2Ray` 等格式，同时提供中文 Web UI 方便查看与管理。

适合这些场景：

- 需要长期聚合公开代理并持续去重、检测、筛选
- 需要把节点直接导出给脚本、服务或代理客户端使用
- 需要一个带中文后台的轻量代理池，而不是维护一套重型平台
- 需要在 `1c1g` 到 `4c4g+` 的 VPS 上快速部署

## 亮点

- 基于 Go + SQLite，部署简单，依赖少
- 支持抓取文本源、订阅源，以及可选 `NodeMaven` 数据源
- 支持有效性、延迟、国家、匿名度、纯净度等信息检测
- 内置中文 Web UI，适配手机、平板和桌面
- 支持 `JSON`、`plain`、`Clash`、`V2Ray` 多格式导出
- 内置 Oracle Cloud Ubuntu 一键部署脚本
- 推送到 `main` 后可自动构建并发布 Docker 镜像到 `GHCR`

## 快速开始

### 方式一：Oracle Cloud Ubuntu 一键部署

适合全新 Ubuntu 主机，脚本会自动安装 Docker、生成部署目录、拉取镜像并启动服务。

`4c4g` 机器推荐直接用下面这条：

```bash
curl -fsSL https://raw.githubusercontent.com/lovely71/public_proxy_pool/main/scripts/deploy_oracle_ubuntu.sh -o /tmp/deploy_oracle_ubuntu.sh && \
sudo HOST_PORT=7171 API_KEY=bailu PUBLIC_BASE_URL=http://YOUR_SERVER_IP:7171 \
FETCH_PROFILE=full SQLITE_MAX_OPEN_CONNS=8 SQLITE_BUSY_TIMEOUT=20s STATS_QUERY_TIMEOUT=3s \
bash /tmp/deploy_oracle_ubuntu.sh
```

部署完成后：

- Web 后台：`http://YOUR_SERVER_IP:7171/ui/overview?token=bailu`
- 统计接口：`curl -H 'X-API-Key: bailu' 'http://YOUR_SERVER_IP:7171/api/v1/stats'`

如果你是小规格机器，建议改成：

```bash
sudo HOST_PORT=7171 API_KEY=bailu PUBLIC_BASE_URL=http://YOUR_SERVER_IP:7171 \
FETCH_PROFILE=lite bash /tmp/deploy_oracle_ubuntu.sh
```

### 方式二：Docker Compose

适合本地开发或服务器长期运行。

1. 准备配置

```bash
cp .env.example .env
```

至少填写：

```bash
API_KEYS=your-token
HTTP_ADDR=:8080
SQLITE_PATH=./data/proxypool.db
```

2. 启动

```bash
docker compose up -d --build
```

3. 验证

```bash
curl http://127.0.0.1:8080/healthz
curl -H 'X-API-Key: your-token' 'http://127.0.0.1:8080/api/v1/stats'
```

### 方式三：本地轻量运行

适合 `1c1g`、开发调试或不想走 Docker 的场景。

```bash
cp .env.example .env
./scripts/run_light.sh
```

这个脚本会自动读取根目录 `.env`，并采用更保守的抓取与校验参数。

## 访问方式

### 浏览器访问后台

浏览器地址栏不会自动带 `X-API-Key` 请求头，所以 Web UI 需要把 token 放进 URL：

```text
http://127.0.0.1:8080/ui/overview?token=your-token
```

### 脚本访问 API

更推荐使用请求头：

```bash
curl -H 'X-API-Key: your-token' 'http://127.0.0.1:8080/api/v1/stats'
curl -H 'X-API-Key: your-token' 'http://127.0.0.1:8080/api/v1/nodes?verify=1&limit=20'
```

### 订阅导出

```bash
curl -H 'X-API-Key: your-token' 'http://127.0.0.1:8080/sub/plain?verify=1&limit=50'
curl -H 'X-API-Key: your-token' 'http://127.0.0.1:8080/sub/clash?verify=1&limit=200'
curl -H 'X-API-Key: your-token' 'http://127.0.0.1:8080/sub/v2ray?verify=1&limit=200'
```

如果你要直接给客户端导入，也可以用 `?token=` 形式：

```text
http://127.0.0.1:8080/sub/clash?verify=1&limit=200&token=your-token
```

### 本地检测订阅节点有效性

仓库内置了一个本地脚本，可以直接拿本项目生成的订阅链接做批量检测，并同时输出四档结果：

- `alive`：节点本身 TCP 可连
- `google_ok`：可通过节点访问 `https://www.gstatic.com/generate_204`
- `huggingface_ok`：可通过节点访问 `https://huggingface.co/robots.txt`
- `checkip_ok`：可通过节点访问 Cloudflare 的 `https://www.cloudflare.com/cdn-cgi/trace` 并拿到出口 IP

最常见的用法：

```bash
./scripts/check_subscription.sh \
  -url 'http://127.0.0.1:8080/sub/v2ray?verify=1&limit=50&token=your-token'
```

如果你的订阅链接没有把 token 放进 URL，也可以单独传请求头：

```bash
./scripts/check_subscription.sh \
  -url 'http://127.0.0.1:8080/sub/clash?verify=1&limit=100' \
  -api-key 'your-token'
```

也支持 JSON 输出，方便后续脚本筛选：

```bash
./scripts/check_subscription.sh \
  -url 'http://127.0.0.1:8080/sub/plain?verify=1&limit=100&token=your-token' \
  -json
```

说明：

- `generate_204` 检测地址固定为 `https://www.gstatic.com/generate_204`
- `huggingface` 检测地址默认是 `https://huggingface.co/robots.txt`
- `checkip` 检测地址默认是 `https://www.cloudflare.com/cdn-cgi/trace`
- 会自动识别本项目导出的 `plain`、`v2ray(base64)`、`clash yaml` 三种格式
- `http` / `https` / `socks4` / `socks5` 节点可直接本地检测
- `ss` / `vmess` / `vless` / `trojan` 节点需要本机可用 `sing-box`，并默认通过 `-v2ray-mode sing-box` 检测
- 文本输出里会显示 `A/G/H/C` 四档标记，分别对应 `alive / google_ok / huggingface_ok / checkip_ok`
- JSON 输出会包含 `alive`、`google_ok`、`huggingface_ok`、`checkip_ok` 汇总统计，以及每个节点对应的四档布尔值和错误信息
- 退出码仍以 `checkip_ok` 为准：当至少有一个节点 `checkip_ok=true` 时返回 `0`，否则返回非 `0`

## 部署方式对比

| 方式 | 适合场景 | 特点 |
| --- | --- | --- |
| Oracle 一键脚本 | 全新 Ubuntu / Oracle Cloud | 自动装 Docker、拉镜像、生成配置，最快上手 |
| Docker Compose | 本地开发 / 服务器长期运行 | 配置清晰，便于管理挂载目录与重启 |
| 本地轻量运行 | 小内存机器 / 调试 | 不依赖容器，参数更保守 |

## Oracle Cloud 一键部署说明

仓库内置脚本：[`scripts/deploy_oracle_ubuntu.sh`](scripts/deploy_oracle_ubuntu.sh)

它会自动：

- 安装 Docker 和 Docker Compose
- 创建 `/opt/public_proxy_pool`
- 生成 `.env` 与 `docker-compose.yml`
- 拉取 `ghcr.io/lovely71/public_proxy_pool:latest`
- 做健康检查并输出后台地址

常见用法：

```bash
sudo bash scripts/deploy_oracle_ubuntu.sh
```

自定义端口和 token：

```bash
sudo HOST_PORT=38482 API_KEY='your-strong-token' PUBLIC_BASE_URL='http://YOUR_SERVER_IP:38482' \
  bash scripts/deploy_oracle_ubuntu.sh
```

通过 SSH 远程执行：

```bash
ssh ubuntu@YOUR_SERVER_IP \
  "sudo env HOST_PORT=7171 API_KEY=bailu PUBLIC_BASE_URL=http://YOUR_SERVER_IP:7171 FETCH_PROFILE=full bash -s" \
  < scripts/deploy_oracle_ubuntu.sh
```

额外注意：

- Oracle 控制台里的 `Security List` 或 `NSG` 仍然需要手动放行入站 `TCP/端口`
- 如果主机启用了 `ufw`，脚本会自动尝试放行该端口
- 若后续手动改了 `/opt/public_proxy_pool/.env`，执行 `docker compose up -d` 即可让配置生效

## 常用环境变量

| 变量 | 说明 | 常见值 |
| --- | --- | --- |
| `API_KEYS` | API 与后台鉴权 token，支持多个值，逗号分隔 | `token-a,token-b` |
| `HTTP_ADDR` | 服务监听地址 | `:8080` |
| `PUBLIC_BASE_URL` | 对外可访问地址，用于 `/probe/echo` 与匿名度探测 | `http://IP:8080` |
| `SQLITE_PATH` | SQLite 数据文件路径 | `./data/proxypool.db` |
| `SQLITE_MAX_OPEN_CONNS` | SQLite 最大连接数，适合多核机器提升读并发 | `2` 到 `8` |
| `SQLITE_BUSY_TIMEOUT` | SQLite 锁等待超时 | `10s`、`15s`、`20s` |
| `STATS_QUERY_TIMEOUT` | `/api/v1/stats` 与 UI 统计查询超时 | `2s`、`3s` |
| `AUTO_FETCH_ENABLED` | 是否自动抓取 | `true` / `false` |
| `AUTO_VALIDATE_ENABLED` | 是否自动校验 | `true` / `false` |
| `FETCH_PROFILE` | 部署脚本使用的抓取参数档位 | `lite` / `full` / `custom` |
| `SOURCE_INTERVAL_SEC` | 统一覆盖启用源抓取间隔 | `60`、`1800` |

完整配置可参考：

- [`.env.example`](.env.example)
- [`internal/config/config.go`](internal/config/config.go)

## 抓取模式

部署脚本支持以下档位：

- `FETCH_PROFILE=lite`：适合 `1c1g` 或希望更稳更省资源的机器
- `FETCH_PROFILE=full`：默认档位，会按 CPU 核数自动提高抓取与校验强度
- `FETCH_PROFILE=aggressive`：`full` 的兼容别名
- `FETCH_PROFILE=custom`：完全手动控制各项参数

`full` 模式下的大致建议：

- `1c`：优先稳态，不追求极限抓取速度
- `2c`：可进入更高并发稳态
- `4c+`：建议配合 `SQLITE_MAX_OPEN_CONNS=8`、`SQLITE_BUSY_TIMEOUT=20s`

如果你希望强行覆盖默认策略，也可以直接在环境变量里设置：

- `FETCH_TICK_INTERVAL`
- `FETCH_MAX_PER_TICK`
- `SOURCE_WORKERS`
- `VALIDATE_WORKERS`
- `SOURCE_SAMPLE_VALIDATE`
- `MIN_FRESH_POOL_SIZE`
- `STARTUP_WARMUP_*`

## GitHub Actions 与镜像

仓库内置工作流：[`.github/workflows/docker-image.yml`](.github/workflows/docker-image.yml)

触发规则：

- 推送到 `main`：构建并推送镜像
- 推送 `v*` 标签：构建并打版本标签
- `pull_request -> main`：只做构建校验，不推送

默认镜像地址：

```text
ghcr.io/lovely71/public_proxy_pool
```

常用标签：

```text
ghcr.io/lovely71/public_proxy_pool:latest
ghcr.io/lovely71/public_proxy_pool:main
ghcr.io/lovely71/public_proxy_pool:sha-<commit>
```

## 项目结构

- `cmd/proxypool`：程序入口
- `internal/`：核心业务代码
- `internal/ui/`：中文 Web UI
- `scripts/run_light.sh`：轻量启动脚本
- `scripts/deploy_oracle_ubuntu.sh`：Oracle Ubuntu 一键部署脚本
- `docker-compose.yml`：Compose 示例
- `.github/workflows/docker-image.yml`：镜像构建工作流

## 开发与测试

运行测试：

```bash
go test ./...
```

接口烟测：

```bash
bash scripts/smoke.sh
```

本地重新构建容器：

```bash
docker compose up -d --build
```

## 常见问题

### 为什么浏览器打开 UI 会提示未授权？

因为浏览器地址栏不会自动带 `X-API-Key`，需要改成：

```text
/ui/overview?token=your-token
```

### 为什么 `/ui/events` 会一直是 pending？

这是 Web UI 的实时事件流连接，属于正常长连接行为；只要概览页主体能正常打开、数字会刷新，就不是故障。

### 为什么新机器上 `full` 模式下首页会比较慢？

`full` 模式下抓取和校验更积极，如果机器核数高、抓取源多，建议一起设置：

```bash
SQLITE_MAX_OPEN_CONNS=8
SQLITE_BUSY_TIMEOUT=20s
STATS_QUERY_TIMEOUT=3s
```

这样可以明显改善统计页和 `/api/v1/stats` 的响应表现。
