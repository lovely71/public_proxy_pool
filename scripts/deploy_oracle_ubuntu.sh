#!/usr/bin/env bash
set -Eeuo pipefail

APP_NAME="public_proxy_pool"
DEFAULT_APP_DIR="/opt/${APP_NAME}"
DEFAULT_IMAGE="ghcr.io/lovely71/public_proxy_pool:latest"
DEFAULT_HOST_PORT="38482"

log() {
  printf '[INFO] %s\n' "$*"
}

warn() {
  printf '[WARN] %s\n' "$*" >&2
}

die() {
  printf '[ERROR] %s\n' "$*" >&2
  exit 1
}

capture_explicit_overrides() {
  declare -gA EXPLICIT_OVERRIDES=()
  local key
  for key in \
    APP_DIR IMAGE HOST_PORT API_KEYS API_KEY PUBLIC_BASE_URL FETCH_PROFILE SOURCE_INTERVAL_SEC GOMAXPROCS \
    AUTO_FETCH_ENABLED AUTO_VALIDATE_ENABLED FETCH_TICK_INTERVAL FETCH_MAX_PER_TICK \
    SOURCE_WORKERS SOURCE_TIMEOUT INGEST_MAX_PER_SOURCE VALIDATE_WORKERS \
    VALIDATE_TIMEOUT SOURCE_SAMPLE_VALIDATE MIN_FRESH_POOL_SIZE \
    NODEMAVEN_ENABLED PURITY_LOOKUP_ENABLED; do
    if [[ -n "${!key-}" ]]; then
      EXPLICIT_OVERRIDES["$key"]="${!key}"
    fi
  done
}

apply_override() {
  local key="$1"
  if [[ -n "${EXPLICIT_OVERRIDES[$key]-}" ]]; then
    printf -v "$key" '%s' "${EXPLICIT_OVERRIDES[$key]}"
  fi
}

ensure_root() {
  if [[ "${EUID}" -ne 0 ]]; then
    die "请用 root 或 sudo 运行，例如：sudo bash scripts/deploy_oracle_ubuntu.sh"
  fi
}

ensure_ubuntu() {
  if [[ ! -r /etc/os-release ]]; then
    die "无法识别系统版本，缺少 /etc/os-release"
  fi

  # shellcheck disable=SC1091
  source /etc/os-release
  if [[ "${ID:-}" != "ubuntu" ]]; then
    warn "当前系统是 ${PRETTY_NAME:-unknown}，脚本按 Ubuntu 流程继续执行。"
  fi
}

ensure_prerequisites() {
  export DEBIAN_FRONTEND=noninteractive
  apt-get update -y
  apt-get install -y ca-certificates curl gnupg lsb-release openssl ufw
}

ensure_sqlite3_if_needed() {
  if [[ -z "${SOURCE_INTERVAL_SEC}" ]]; then
    return
  fi

  if command -v sqlite3 >/dev/null 2>&1; then
    return
  fi

  export DEBIAN_FRONTEND=noninteractive
  apt-get update -y
  apt-get install -y sqlite3
}

install_docker() {
  if command -v docker >/dev/null 2>&1 && docker compose version >/dev/null 2>&1; then
    log "Docker 和 Docker Compose 已安装，跳过安装步骤。"
    systemctl enable --now docker >/dev/null 2>&1 || true
    return
  fi

  local arch codename
  arch="$(dpkg --print-architecture)"
  # shellcheck disable=SC1091
  source /etc/os-release
  codename="${VERSION_CODENAME:-}"
  [[ -n "${codename}" ]] || die "无法识别 Ubuntu codename"

  install -m 0755 -d /etc/apt/keyrings
  if [[ ! -f /etc/apt/keyrings/docker.asc ]]; then
    curl -fsSL "https://download.docker.com/linux/ubuntu/gpg" -o /etc/apt/keyrings/docker.asc
    chmod a+r /etc/apt/keyrings/docker.asc
  fi

  cat >/etc/apt/sources.list.d/docker.list <<EOF
deb [arch=${arch} signed-by=/etc/apt/keyrings/docker.asc] https://download.docker.com/linux/ubuntu ${codename} stable
EOF

  export DEBIAN_FRONTEND=noninteractive
  apt-get update -y
  apt-get install -y docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin
  systemctl enable --now docker

  if [[ -n "${SUDO_USER:-}" && "${SUDO_USER}" != "root" ]]; then
    usermod -aG docker "${SUDO_USER}" || true
  fi
}

validate_port() {
  [[ "${HOST_PORT}" =~ ^[0-9]+$ ]] || die "HOST_PORT 必须是数字，当前值：${HOST_PORT}"
  if (( HOST_PORT < 1 || HOST_PORT > 65535 )); then
    die "HOST_PORT 必须在 1-65535 之间，当前值：${HOST_PORT}"
  fi
}

validate_fetch_profile() {
  case "${FETCH_PROFILE}" in
    ""|lite|full|aggressive|custom)
      ;;
    *)
      die "FETCH_PROFILE 仅支持 lite、full、aggressive、custom，当前值：${FETCH_PROFILE}"
      ;;
  esac
}

validate_source_interval() {
  if [[ -z "${SOURCE_INTERVAL_SEC}" ]]; then
    return
  fi
  [[ "${SOURCE_INTERVAL_SEC}" =~ ^[0-9]+$ ]] || die "SOURCE_INTERVAL_SEC 必须是数字，当前值：${SOURCE_INTERVAL_SEC}"
  if (( SOURCE_INTERVAL_SEC < 1 )); then
    die "SOURCE_INTERVAL_SEC 必须大于等于 1，当前值：${SOURCE_INTERVAL_SEC}"
  fi
}

apply_profile_defaults() {
  case "${FETCH_PROFILE}" in
    lite|"")
      GOMAXPROCS="1"
      SOURCE_INTERVAL_SEC=""
      AUTO_FETCH_ENABLED="true"
      AUTO_VALIDATE_ENABLED="true"
      FETCH_TICK_INTERVAL="60s"
      FETCH_MAX_PER_TICK="2"
      SOURCE_WORKERS="2"
      SOURCE_TIMEOUT="12s"
      INGEST_MAX_PER_SOURCE="1500"
      VALIDATE_WORKERS="20"
      VALIDATE_TIMEOUT="6s"
      SOURCE_SAMPLE_VALIDATE="5"
      MIN_FRESH_POOL_SIZE="50"
      NODEMAVEN_ENABLED="false"
      PURITY_LOOKUP_ENABLED="false"
      ;;
    full|aggressive)
      GOMAXPROCS="2"
      SOURCE_INTERVAL_SEC="60"
      AUTO_FETCH_ENABLED="true"
      AUTO_VALIDATE_ENABLED="true"
      FETCH_TICK_INTERVAL="1s"
      FETCH_MAX_PER_TICK="1000"
      SOURCE_WORKERS="50"
      SOURCE_TIMEOUT="12s"
      INGEST_MAX_PER_SOURCE="5000"
      VALIDATE_WORKERS="30"
      VALIDATE_TIMEOUT="6s"
      SOURCE_SAMPLE_VALIDATE="10"
      MIN_FRESH_POOL_SIZE="200"
      NODEMAVEN_ENABLED="true"
      PURITY_LOOKUP_ENABLED="false"
      ;;
    custom)
      ;;
  esac
}

load_existing_env_if_any() {
  local env_path="${APP_DIR}/.env"
  if [[ -f "${env_path}" ]]; then
    log "发现已有部署配置，正在复用 ${env_path}"
    set -a
    # shellcheck disable=SC1090
    source "${env_path}"
    set +a
  fi
}

generate_api_key_if_needed() {
  if [[ -z "${API_KEYS}" ]]; then
    if command -v openssl >/dev/null 2>&1; then
      API_KEYS="$(openssl rand -hex 32)"
    else
      API_KEYS="$(tr -dc 'a-f0-9' </dev/urandom | head -c 64)"
    fi
    log "未提供 API_KEYS，已自动生成一个随机 token。"
  fi
}

write_env_file() {
  local env_path="${APP_DIR}/.env"
  cat >"${env_path}" <<EOF
IMAGE=${IMAGE}
HOST_PORT=${HOST_PORT}
API_KEYS=${API_KEYS}
PUBLIC_BASE_URL=${PUBLIC_BASE_URL}
FETCH_PROFILE=${FETCH_PROFILE}
SOURCE_INTERVAL_SEC=${SOURCE_INTERVAL_SEC}

GOMAXPROCS=${GOMAXPROCS}

AUTO_FETCH_ENABLED=${AUTO_FETCH_ENABLED}
AUTO_VALIDATE_ENABLED=${AUTO_VALIDATE_ENABLED}

FETCH_TICK_INTERVAL=${FETCH_TICK_INTERVAL}
FETCH_MAX_PER_TICK=${FETCH_MAX_PER_TICK}
SOURCE_WORKERS=${SOURCE_WORKERS}
SOURCE_TIMEOUT=${SOURCE_TIMEOUT}
INGEST_MAX_PER_SOURCE=${INGEST_MAX_PER_SOURCE}

VALIDATE_WORKERS=${VALIDATE_WORKERS}
VALIDATE_TIMEOUT=${VALIDATE_TIMEOUT}
SOURCE_SAMPLE_VALIDATE=${SOURCE_SAMPLE_VALIDATE}
MIN_FRESH_POOL_SIZE=${MIN_FRESH_POOL_SIZE}

NODEMAVEN_ENABLED=${NODEMAVEN_ENABLED}
PURITY_LOOKUP_ENABLED=${PURITY_LOOKUP_ENABLED}
EOF
  chmod 600 "${env_path}"
}

write_compose_file() {
  local compose_path="${APP_DIR}/docker-compose.yml"
  cat >"${compose_path}" <<'EOF'
services:
  proxypool:
    image: ${IMAGE}
    container_name: proxypool
    restart: unless-stopped
    ports:
      - "${HOST_PORT}:8080"
    environment:
      HTTP_ADDR: ":8080"
      SQLITE_PATH: "/data/proxypool.db"
      API_KEYS: "${API_KEYS}"
      PUBLIC_BASE_URL: "${PUBLIC_BASE_URL}"
      GOMAXPROCS: "${GOMAXPROCS}"
      AUTO_FETCH_ENABLED: "${AUTO_FETCH_ENABLED}"
      AUTO_VALIDATE_ENABLED: "${AUTO_VALIDATE_ENABLED}"
      FETCH_TICK_INTERVAL: "${FETCH_TICK_INTERVAL}"
      FETCH_MAX_PER_TICK: "${FETCH_MAX_PER_TICK}"
      SOURCE_WORKERS: "${SOURCE_WORKERS}"
      SOURCE_TIMEOUT: "${SOURCE_TIMEOUT}"
      INGEST_MAX_PER_SOURCE: "${INGEST_MAX_PER_SOURCE}"
      VALIDATE_WORKERS: "${VALIDATE_WORKERS}"
      VALIDATE_TIMEOUT: "${VALIDATE_TIMEOUT}"
      SOURCE_SAMPLE_VALIDATE: "${SOURCE_SAMPLE_VALIDATE}"
      MIN_FRESH_POOL_SIZE: "${MIN_FRESH_POOL_SIZE}"
      NODEMAVEN_ENABLED: "${NODEMAVEN_ENABLED}"
      PURITY_LOOKUP_ENABLED: "${PURITY_LOOKUP_ENABLED}"
    volumes:
      - ./data:/data
      # 如需离线 GeoIP，可手动创建 ./geoip 并追加以下挂载：
      # - ./geoip:/geoip:ro
EOF
}

open_firewall_if_needed() {
  if command -v ufw >/dev/null 2>&1; then
    if ufw status 2>/dev/null | grep -q "Status: active"; then
      ufw allow "${HOST_PORT}/tcp" >/dev/null 2>&1 || true
      log "已在 UFW 中放行 TCP/${HOST_PORT}"
    else
      log "UFW 未启用，跳过主机防火墙配置。"
    fi
  fi
}

warn_if_port_busy() {
  if command -v ss >/dev/null 2>&1 && ss -lnt "( sport = :${HOST_PORT} )" | tail -n +2 | grep -q .; then
    warn "检测到主机端口 ${HOST_PORT} 已被占用，若部署失败请改用其他 HOST_PORT。"
  fi
}

deploy_container() {
  cd "${APP_DIR}"
  docker compose pull
  docker compose up -d
}

apply_source_interval_override() {
  if [[ -z "${SOURCE_INTERVAL_SEC}" ]]; then
    return
  fi

  local db_path="${APP_DIR}/data/proxypool.db"
  [[ -f "${db_path}" ]] || die "未找到数据库文件：${db_path}"

  sqlite3 "${db_path}" \
    "UPDATE sources SET interval_sec=${SOURCE_INTERVAL_SEC}, next_fetch_at=strftime('%s','now') WHERE enabled=1;"
  log "已将所有启用抓取源的间隔统一调整为 ${SOURCE_INTERVAL_SEC} 秒，并触发立即进入下一轮抓取。"
}

wait_for_healthcheck() {
  local url="http://127.0.0.1:${HOST_PORT}/healthz"
  local attempt
  for attempt in $(seq 1 45); do
    if curl -fsS --max-time 3 "${url}" >/dev/null 2>&1; then
      log "健康检查通过：${url}"
      return
    fi
    sleep 2
  done

  warn "服务在预期时间内未通过健康检查，下面输出最近日志："
  (cd "${APP_DIR}" && docker compose logs --tail=120 proxypool) || true
  die "部署未完成，请根据日志排查。"
}

show_summary() {
  local public_hint=""
  local guessed_ip=""

  guessed_ip="$(curl -4fsS --max-time 3 https://api64.ipify.org 2>/dev/null || true)"
  if [[ -n "${PUBLIC_BASE_URL}" ]]; then
    public_hint="${PUBLIC_BASE_URL}"
  elif [[ -n "${guessed_ip}" ]]; then
    public_hint="http://${guessed_ip}:${HOST_PORT}"
  else
    public_hint="http://你的服务器公网IP:${HOST_PORT}"
  fi

  cat <<EOF

部署完成。

应用目录: ${APP_DIR}
镜像地址: ${IMAGE}
监听端口: ${HOST_PORT}
API Token: ${API_KEYS}
抓取模式: ${FETCH_PROFILE:-lite}
源抓取间隔: ${SOURCE_INTERVAL_SEC:-保留各源默认值}

本机检查:
  curl http://127.0.0.1:${HOST_PORT}/healthz
  curl -H 'X-API-Key: ${API_KEYS}' 'http://127.0.0.1:${HOST_PORT}/api/v1/stats'

后台地址:
  ${public_hint}/ui/overview?token=${API_KEYS}

重要提醒:
  1. Oracle Cloud 控制台里还需要放行入站 TCP/${HOST_PORT}（Security List 或 NSG）。
  2. 如果后续想启用匿名度探测，可把 PUBLIC_BASE_URL 改成外网可访问地址后重新运行本脚本。
  3. 若你刚通过 sudo 安装 Docker，普通用户想直接执行 docker 命令，重新登录一次即可生效。
EOF
}

main() {
  capture_explicit_overrides
  ensure_root
  ensure_ubuntu

  APP_DIR="${EXPLICIT_OVERRIDES[APP_DIR]-${DEFAULT_APP_DIR}}"
  IMAGE="${DEFAULT_IMAGE}"
  HOST_PORT="${DEFAULT_HOST_PORT}"
  API_KEYS=""
  PUBLIC_BASE_URL=""
  FETCH_PROFILE="full"
  SOURCE_INTERVAL_SEC="60"

  apply_profile_defaults

  load_existing_env_if_any

  if [[ -n "${EXPLICIT_OVERRIDES[FETCH_PROFILE]-}" ]]; then
    FETCH_PROFILE="${EXPLICIT_OVERRIDES[FETCH_PROFILE]}"
    validate_fetch_profile
    apply_profile_defaults
  else
    validate_fetch_profile
  fi

  apply_override IMAGE
  apply_override HOST_PORT
  apply_override API_KEYS
  if [[ -z "${EXPLICIT_OVERRIDES[API_KEYS]-}" && -n "${EXPLICIT_OVERRIDES[API_KEY]-}" ]]; then
    API_KEYS="${EXPLICIT_OVERRIDES[API_KEY]}"
  fi
  apply_override PUBLIC_BASE_URL
  apply_override SOURCE_INTERVAL_SEC
  apply_override GOMAXPROCS
  apply_override AUTO_FETCH_ENABLED
  apply_override AUTO_VALIDATE_ENABLED
  apply_override FETCH_TICK_INTERVAL
  apply_override FETCH_MAX_PER_TICK
  apply_override SOURCE_WORKERS
  apply_override SOURCE_TIMEOUT
  apply_override INGEST_MAX_PER_SOURCE
  apply_override VALIDATE_WORKERS
  apply_override VALIDATE_TIMEOUT
  apply_override SOURCE_SAMPLE_VALIDATE
  apply_override MIN_FRESH_POOL_SIZE
  apply_override NODEMAVEN_ENABLED
  apply_override PURITY_LOOKUP_ENABLED

  validate_port
  validate_source_interval
  ensure_prerequisites
  ensure_sqlite3_if_needed
  install_docker

  mkdir -p "${APP_DIR}/data"
  generate_api_key_if_needed
  write_env_file
  write_compose_file
  warn_if_port_busy
  open_firewall_if_needed
  deploy_container
  wait_for_healthcheck
  apply_source_interval_override
  show_summary
}

main "$@"
