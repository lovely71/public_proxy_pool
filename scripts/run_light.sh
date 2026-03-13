#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

# Optional: load local env file (do not commit).
if [[ -f "$ROOT_DIR/.env" ]]; then
  set -a
  # shellcheck disable=SC1091
  source "$ROOT_DIR/.env"
  set +a
fi

# Lightweight defaults for 1c1g.
export GOMAXPROCS="${GOMAXPROCS:-1}"

export HTTP_ADDR="${HTTP_ADDR:-:8080}"
export SQLITE_PATH="${SQLITE_PATH:-./data/proxypool.db}"

# Security
export API_KEYS="${API_KEYS:-changeme}"

# Fetch (limit bandwidth/CPU)
export AUTO_FETCH_ENABLED="${AUTO_FETCH_ENABLED:-true}"
export FETCH_TICK_INTERVAL="${FETCH_TICK_INTERVAL:-60s}"
export FETCH_MAX_PER_TICK="${FETCH_MAX_PER_TICK:-2}"
export SOURCE_WORKERS="${SOURCE_WORKERS:-2}"
export SOURCE_TIMEOUT="${SOURCE_TIMEOUT:-12s}"
export INGEST_MAX_PER_SOURCE="${INGEST_MAX_PER_SOURCE:-1500}"

# Validation (limit concurrency)
export AUTO_VALIDATE_ENABLED="${AUTO_VALIDATE_ENABLED:-true}"
export VALIDATE_WORKERS="${VALIDATE_WORKERS:-20}"
export VALIDATE_TIMEOUT="${VALIDATE_TIMEOUT:-6s}"
export SOURCE_SAMPLE_VALIDATE="${SOURCE_SAMPLE_VALIDATE:-5}"
export MIN_FRESH_POOL_SIZE="${MIN_FRESH_POOL_SIZE:-50}"

# Optional (disable by default in light mode)
export NODEMAVEN_ENABLED="${NODEMAVEN_ENABLED:-false}"
export PURITY_LOOKUP_ENABLED="${PURITY_LOOKUP_ENABLED:-false}"

exec go run ./cmd/proxypool
