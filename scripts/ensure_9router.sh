#!/usr/bin/env bash
set -euo pipefail

# Idempotent 9Router guard for Quantum Swarm.
# Checks the OpenAI-compatible models endpoint first. If it is not live,
# starts the managed 9Router process through `qsm deploy`.

QSM_ROOT="${QSM_ROOT:-$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)}"
QSM_HARNESS="${QSM_HARNESS:-langchain}"
QSM_WAIT="${QSM_WAIT:-75s}"
QSM_9ROUTER_URL="${QSM_9ROUTER_URL:-http://127.0.0.1:20128/v1}"
QSM_BIN="${QSM_BIN:-$QSM_ROOT/qsm}"
QSM_ENSURE_BUILD="${QSM_ENSURE_BUILD:-auto}"

MODEL_URL="${QSM_9ROUTER_URL%/}/models"

curl_args=(-fsS --max-time "${QSM_CURL_TIMEOUT:-2}")
if [[ -n "${QSM_9ROUTER_API_KEY:-}" ]]; then
  curl_args+=(-H "Authorization: Bearer ${QSM_9ROUTER_API_KEY}")
fi

check_router() {
  curl "${curl_args[@]}" "$MODEL_URL" >/dev/null
}

build_qsm() {
  echo "Building qsm binary..."
  (cd "$QSM_ROOT" && go build -o "$QSM_BIN" ./cmd/qsm)
}

if check_router; then
  echo "9Router live: $MODEL_URL"
  exit 0
fi

echo "9Router not live at $MODEL_URL"

if [[ ! -x "$QSM_BIN" ]]; then
  build_qsm
elif [[ "$QSM_ENSURE_BUILD" == "1" || "$QSM_ENSURE_BUILD" == "true" ]]; then
  build_qsm
fi

if [[ ! -x "$QSM_BIN" ]]; then
  echo "qsm binary is not executable: $QSM_BIN" >&2
  exit 1
fi

echo "Activating managed 9Router via qsm deploy..."
"$QSM_BIN" deploy \
  -root "$QSM_ROOT" \
  -harness "$QSM_HARNESS" \
  -build=false \
  -start-router=true \
  -wait "$QSM_WAIT"

if check_router; then
  echo "9Router live after activation: $MODEL_URL"
  exit 0
fi

echo "9Router still not live after activation." >&2
echo "Inspect log: $QSM_ROOT/.state/9router.log" >&2
exit 1
