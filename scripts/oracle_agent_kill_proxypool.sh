#!/usr/bin/env bash
set -Eeuo pipefail

APP_DIR="${APP_DIR:-/opt/public_proxy_pool}"
COMPOSE_FILE="${COMPOSE_FILE:-${APP_DIR}/docker-compose.yml}"
CONTAINER_NAME="${CONTAINER_NAME:-proxypool}"

log() {
  printf '[INFO] %s\n' "$*"
}

warn() {
  printf '[WARN] %s\n' "$*" >&2
}

stop_systemd_units() {
  if ! command -v systemctl >/dev/null 2>&1; then
    return
  fi

  local unit
  for unit in \
    proxypool.service \
    public_proxy_pool.service \
    public-proxy-pool.service; do
    if systemctl list-unit-files --type=service --no-legend 2>/dev/null | awk '{print $1}' | grep -Fxq "${unit}"; then
      log "停止并禁用 systemd 服务: ${unit}"
      systemctl stop "${unit}" >/dev/null 2>&1 || true
      systemctl disable "${unit}" >/dev/null 2>&1 || true
    fi
  done
}

stop_docker_compose() {
  if ! command -v docker >/dev/null 2>&1; then
    return
  fi

  if [[ -f "${COMPOSE_FILE}" ]]; then
    log "尝试停止 compose 项目: ${COMPOSE_FILE}"
    docker compose -f "${COMPOSE_FILE}" down --remove-orphans --timeout 5 >/dev/null 2>&1 || true
  fi
}

stop_docker_containers() {
  if ! command -v docker >/dev/null 2>&1; then
    return
  fi

  local ids=""
  ids="$(docker ps -aq --filter "name=^${CONTAINER_NAME}$" 2>/dev/null || true)"
  if [[ -n "${ids}" ]]; then
    log "强制删除容器: ${CONTAINER_NAME}"
    docker update --restart=no ${ids} >/dev/null 2>&1 || true
    docker rm -f ${ids} >/dev/null 2>&1 || true
  fi

  ids="$(docker ps -aq --filter "ancestor=ghcr.io/lovely71/public_proxy_pool:latest" 2>/dev/null || true)"
  if [[ -n "${ids}" ]]; then
    log "强制删除镜像对应容器"
    docker update --restart=no ${ids} >/dev/null 2>&1 || true
    docker rm -f ${ids} >/dev/null 2>&1 || true
  fi
}

kill_host_processes() {
  local patterns=(
    "/app/proxypool"
    "cmd/proxypool"
    "go run ./cmd/proxypool"
    "public_proxy_pool"
    "proxypool"
  )

  local pattern
  for pattern in "${patterns[@]}"; do
    if pgrep -f "${pattern}" >/dev/null 2>&1; then
      log "终止宿主机进程匹配: ${pattern}"
      pkill -TERM -f "${pattern}" >/dev/null 2>&1 || true
      sleep 1
      pkill -KILL -f "${pattern}" >/dev/null 2>&1 || true
    fi
  done
}

verify_stopped() {
  local still_running=0

  if command -v docker >/dev/null 2>&1; then
    if docker ps --format '{{.Names}}' 2>/dev/null | grep -Fxq "${CONTAINER_NAME}"; then
      warn "容器仍在运行: ${CONTAINER_NAME}"
      still_running=1
    fi
  fi

  if pgrep -fa 'proxypool|cmd/proxypool|/app/proxypool|go run ./cmd/proxypool' >/dev/null 2>&1; then
    warn "仍检测到宿主机 proxypool 相关进程"
    pgrep -fa 'proxypool|cmd/proxypool|/app/proxypool|go run ./cmd/proxypool' || true
    still_running=1
  fi

  if (( still_running == 0 )); then
    log "proxypool 已停止。"
    return 0
  fi

  return 1
}

main() {
  log "开始紧急停止 proxypool"
  stop_systemd_units
  stop_docker_compose
  stop_docker_containers
  kill_host_processes

  if verify_stopped; then
    exit 0
  fi

  warn "仍有残留进程或容器，请检查上面的输出。"
  exit 1
}

main "$@"
