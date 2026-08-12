#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
API_BASE_URL="${API_BASE_URL:-http://127.0.0.1:8080/api/v1}"
WEB_BASE_URL="${WEB_BASE_URL:-http://127.0.0.1:5173}"
API_HEALTH_URL="${API_BASE_URL%/api/v1}/healthz"
API_PID=""
WEB_PID=""

cleanup() {
  if [ -n "$WEB_PID" ]; then
    kill "$WEB_PID" 2>/dev/null || true
  fi
  if [ -n "$API_PID" ]; then
    kill "$API_PID" 2>/dev/null || true
  fi
}
trap cleanup EXIT

cd "$ROOT_DIR"

go test ./...
pnpm --dir web build
pnpm --dir web lint

if ! curl -fsS "$API_HEALTH_URL" >/dev/null 2>&1; then
  docker compose -f docker-compose-dev-db.yaml up -d
  APP_ENV=development \
    DATABASE_URL="${DATABASE_URL:-postgres://devops:devops@localhost:5432/devops?sslmode=disable}" \
    REDIS_ADDR="${REDIS_ADDR:-redis://localhost:6379/0}" \
    PUBLIC_BASE_URL="${WEB_BASE_URL}" \
    APP_CORS_ORIGINS="${WEB_BASE_URL}" \
    go run ./cmd/api &
  API_PID="$!"
  for _ in $(seq 1 60); do
    if curl -fsS "$API_HEALTH_URL" >/dev/null 2>&1; then
      break
    fi
    sleep 1
  done
fi

if ! curl -fsS "$API_HEALTH_URL" >/dev/null 2>&1; then
  echo "API is not healthy at $API_HEALTH_URL" >&2
  exit 1
fi

if [ ! -d tests/node_modules ]; then
  pnpm --dir tests install
fi

API_BASE_URL="$API_BASE_URL" pnpm --dir tests api

if ! curl -fsS "$WEB_BASE_URL" >/dev/null 2>&1; then
  pnpm --dir web dev --host 127.0.0.1 >/tmp/luna-devops-vite.log 2>&1 &
  WEB_PID="$!"
  for _ in $(seq 1 60); do
    if curl -fsS "$WEB_BASE_URL" >/dev/null 2>&1; then
      break
    fi
    sleep 1
  done
fi

if ! curl -fsS "$WEB_BASE_URL" >/dev/null 2>&1; then
  echo "Web is not healthy at $WEB_BASE_URL" >&2
  exit 1
fi

WEB_BASE_URL="$WEB_BASE_URL" pnpm --dir tests browser
