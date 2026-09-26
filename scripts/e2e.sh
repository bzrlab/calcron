#!/usr/bin/env sh
set -eu

if [ "${CALCRON_E2E_SELF_TEST:-}" = "1" ]; then
  if ! awk '/^compose\(\) / { found=1 } END { exit found ? 0 : 1 }' "$0"; then
    echo "missing project-scoped compose wrapper" >&2
    exit 1
  fi
  if awk 'index($0, "docker compose down") && $0 !~ /awk/ { bad=1 } END { exit bad ? 0 : 1 }' "$0"; then
    echo "unsafe unscoped docker compose down" >&2
    exit 1
  fi
  echo "E2E SELF TEST PASS"
  exit 0
fi

cleanup() { docker rm -f calcron-e2e-second >/dev/null 2>&1 || true; docker compose down >/dev/null 2>&1 || true; }
trap cleanup EXIT
wait_http() {
  url=$1
  for _ in $(seq 1 30); do
    if curl -fsS "$url/health" >/dev/null 2>&1; then return 0; fi
    sleep 1
  done
  docker compose logs >&2 || true
  return 1
}
CALCRON_RETRY_BASE=20ms docker compose up -d --build
wait_http http://127.0.0.1:8080
docker run -d --rm --name calcron-e2e-second --network calcron_default -p 8081:8080 -e 'DATABASE_URL=postgres://calcron:calcron@postgres:5432/calcron?sslmode=disable' -e CALCRON_ADMIN_TOKEN=change-me -e CALCRON_RETRY_BASE=20ms calcron-calcron >/dev/null
wait_http http://127.0.0.1:8081
CALCRON_SECOND_URL=http://127.0.0.1:8081 node scripts/e2e.mjs
