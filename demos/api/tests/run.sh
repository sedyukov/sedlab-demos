#!/usr/bin/env bash
# Contract tests for both implementations over one shared database, for one edition.
#   tests/run.sh [ru|intl]      (needs: go, python venv with '.[test]' + sedlab_accounts — see README)
# Starts Go on :18080 (it proxies /py → Python on :18081, like Caddy does in prod), runs the suite against
# /go, then /py, then the parity suite (both at once). Quiet output (public CI: SECURITY.md).
set -euo pipefail
EDITION="${1:-intl}"
HERE="$(cd "$(dirname "$0")/.." && pwd)"
PY="${PYTHON:-$HERE/python/.venv/bin/python}"
WORK="$(mktemp -d)"
export DB_PATH="$WORK/api.db" EDITION SANDBOX_PER_HOUR=100000 RATE_PER_MIN=100000 \
       API_TOKEN_SECRET="test-secret-$RANDOM$RANDOM" ACCOUNTS_PROVIDER=mock \
       CONTRACT_DIR="$HERE/contract" MIGRATIONS_DIR="$HERE/migrations" WEB_DIR="$HERE/web"
export TEST_TOKEN_SECRET="$API_TOKEN_SECRET"
GO_PORT="${GO_PORT:-18080}" PY_PORT="${PY_PORT:-18081}"
pids=()
cleanup() { for p in "${pids[@]}"; do kill "$p" 2>/dev/null || true; done; rm -rf "$WORK"; }
trap cleanup EXIT

(cd "$HERE/go" && go build -o "$WORK/api-go" .)
# start both at the same moment: migrations must apply exactly once
ADDR="127.0.0.1:$GO_PORT" DEV_PY_PROXY="http://127.0.0.1:$PY_PORT" "$WORK/api-go" >"$WORK/go.log" 2>&1 & pids+=($!)
(cd "$HERE/python" && exec "$PY" -m uvicorn --factory api_py.app:create_app --host 127.0.0.1 --port "$PY_PORT" \
   --log-level warning) >"$WORK/py.log" 2>&1 & pids+=($!)
for _ in $(seq 1 50); do
  curl -sf "http://127.0.0.1:$GO_PORT/go/health" >/dev/null && curl -sf "http://127.0.0.1:$GO_PORT/py/health" >/dev/null && break
  sleep 0.2
done
B="http://127.0.0.1:$GO_PORT"
rc=0
TB="--tb=short"; [ "${QUIET:-0}" = 1 ] && TB="--tb=no"   # QUIET=1 in public CI (SECURITY.md p.3)
run() { echo "== $EDITION · $1"; shift; (cd "$HERE" && "$@" "$PY" -m pytest -q $TB -p no:cacheprovider tests/contract) || rc=1; }
run "go"       env API_BASE_URL="$B/go"
run "python"   env API_BASE_URL="$B/py"
run "parity"   env API_BASE_URL="$B/go" API_OTHER_BASE_URL="$B/py" PYTEST_ADDOPTS="-k parity"
if [ $rc -ne 0 ] && [ "${QUIET:-0}" != 1 ]; then echo "--- go.log"; tail -20 "$WORK/go.log"; echo "--- py.log"; tail -20 "$WORK/py.log"; fi
exit $rc
