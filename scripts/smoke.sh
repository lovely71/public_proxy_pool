#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

PORT="${PORT:-18082}"
API_KEYS="${API_KEYS:-changeme}"

LOG_FILE="${LOG_FILE:-/tmp/proxypool_smoke.log}"
PID_FILE="${PID_FILE:-/tmp/proxypool_smoke.pid}"

cleanup() {
  if [[ -f "$PID_FILE" ]]; then
    kill "$(cat "$PID_FILE")" >/dev/null 2>&1 || true
    rm -f "$PID_FILE"
  fi
}
trap cleanup EXIT

HTTP_ADDR=":${PORT}" API_KEYS="${API_KEYS}" AUTO_FETCH_ENABLED=false AUTO_VALIDATE_ENABLED=false RATE_LIMIT_RPS=0 \
  go run ./cmd/proxypool >"$LOG_FILE" 2>&1 &
echo $! >"$PID_FILE"

base="http://127.0.0.1:${PORT}"

for _ in $(seq 1 50); do
  if curl -fsS "${base}/healthz" >/dev/null 2>&1; then
    break
  fi
  sleep 0.1
done

code() { curl -s -o /dev/null -w "%{http_code}" "$1"; }
code_key() { curl -s -o /dev/null -w "%{http_code}" -H "X-API-Key: ${API_KEYS}" "$1"; }

echo "healthz: $(code "${base}/healthz")"
echo "readyz: $(code "${base}/readyz")"
echo "metrics(no key): $(code "${base}/metrics")"
echo "metrics(with key): $(code_key "${base}/metrics")"
echo "api stats(with key): $(code_key "${base}/api/v1/stats")"
echo "api sources(with key): $(code_key "${base}/api/v1/sources")"
echo "ui overview(no token): $(code "${base}/ui/overview")"
echo "ui overview(with token): $(code "${base}/ui/overview?token=${API_KEYS}")"
echo "ui sources(with token): $(code "${base}/ui/sources?token=${API_KEYS}")"
echo "ui nodes(with token): $(code "${base}/ui/nodes?token=${API_KEYS}")"
echo "ui api(with token): $(code "${base}/ui/api?token=${API_KEYS}")"
echo "ui sub(with token): $(code "${base}/ui/sub?token=${API_KEYS}")"
echo "ui static(css): $(code "${base}/ui/static/app.css")"
echo "sub plain(with key): $(code_key "${base}/sub/plain")"
echo "sub clash(with key): $(code_key "${base}/sub/clash")"
echo "sub v2ray(with key): $(code_key "${base}/sub/v2ray")"

echo "log: ${LOG_FILE}"

