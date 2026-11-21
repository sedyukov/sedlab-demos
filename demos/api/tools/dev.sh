#!/usr/bin/env bash
# Local run of both backends like in prod: http://127.0.0.1:18080 (Go serves the page and /go, proxies /py to Python).
#   tools/dev.sh [ru|intl]     Ctrl-C stops both. Data: ./data/api.db
set -euo pipefail
HERE="$(cd "$(dirname "$0")/.." && pwd)"
export EDITION="${1:-intl}" DB_PATH="${DB_PATH:-$HERE/data/api.db}" ACCOUNTS_PROVIDER="${ACCOUNTS_PROVIDER:-mock}" \
       CONTRACT_DIR="$HERE/contract" MIGRATIONS_DIR="$HERE/migrations" WEB_DIR="$HERE/web" SANDBOX_PER_HOUR="${SANDBOX_PER_HOUR:-1000}"
[ -f "$HERE/web/vendor/swagger-ui/swagger-ui-bundle.js" ] || "$HERE/tools/fetch-swagger.sh" "$HERE/web"
(cd "$HERE/go" && go build -o api-go .)
trap 'kill 0' EXIT
(cd "$HERE/python" && exec .venv/bin/python -m uvicorn --factory api_py.app:create_app --host 127.0.0.1 --port 18081 --log-level warning) &
ADDR=127.0.0.1:18080 DEV_PY_PROXY=http://127.0.0.1:18081 "$HERE/go/api-go"
