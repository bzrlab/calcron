#!/usr/bin/env sh
set -eu

if [ "${CALCRON_E2E_SELF_TEST:-}" = "1" ]; then
  if ! awk '/^compose\(\) / { found=1 } END { exit found ? 0 : 1 }' "$0"; then
    echo "missing project-scoped compose wrapper" >&2
    exit 1
  fi
  if awk '$0 ~ /^[[:space:]]*docker[[:space:]]+compose[[:space:]]+down/ { bad=1 } END { exit bad ? 0 : 1 }' "$0"; then
    echo "unsafe unscoped docker compose down" >&2
    exit 1
  fi
  echo "E2E SELF TEST PASS"
  exit 0
fi

CALCRON_E2E_PROJECT=${CALCRON_E2E_PROJECT:-calcron-e2e-$$}
CALCRON_E2E_COMPOSE_FILE=${CALCRON_E2E_COMPOSE_FILE:-compose.yaml}
CALCRON_E2E_OVERRIDE_FILE=$(mktemp "${TMPDIR:-/tmp}/calcron-e2e-compose.XXXXXX.yml")
POSTGRES_PASSWORD=calcron
CALCRON_ADMIN_TOKEN=change-me
export CALCRON_E2E_PROJECT CALCRON_E2E_COMPOSE_FILE CALCRON_E2E_OVERRIDE_FILE POSTGRES_PASSWORD CALCRON_ADMIN_TOKEN

cat >"$CALCRON_E2E_OVERRIDE_FILE" <<'YAML'
services:
  calcron:
    ports: !reset
      - "127.0.0.1::8080"
    networks: !reset
      - default
YAML

compose() {
  docker compose --project-name "$CALCRON_E2E_PROJECT" --file "$CALCRON_E2E_COMPOSE_FILE" --file "$CALCRON_E2E_OVERRIDE_FILE" "$@"
}

SECOND_CONTAINER="${CALCRON_E2E_PROJECT}-second"
cleanup() { docker rm -f "$SECOND_CONTAINER" >/dev/null 2>&1 || true; compose down --volumes --remove-orphans >/dev/null 2>&1 || true; rm -f "$CALCRON_E2E_OVERRIDE_FILE"; }
trap cleanup EXIT
wait_http() {
  url=$1
  for _ in $(seq 1 30); do
    if curl -fsS "$url/health" >/dev/null 2>&1; then return 0; fi
    sleep 1
  done
  compose logs >&2 || true
  return 1
}
CALCRON_RETRY_BASE=20ms compose up -d --build
PRIMARY_ADDR=$(compose port calcron 8080)
PRIMARY_URL="http://127.0.0.1:${PRIMARY_ADDR##*:}"
wait_http "$PRIMARY_URL"
CALCRON_IMAGE=$(compose images -q calcron)
docker run -d --rm --name "$SECOND_CONTAINER" --network "${CALCRON_E2E_PROJECT}_default" -p 127.0.0.1::8080 -e 'DATABASE_URL=postgres://calcron:calcron@postgres:5432/calcron?sslmode=disable' -e CALCRON_ADMIN_TOKEN=change-me -e CALCRON_RETRY_BASE=20ms "$CALCRON_IMAGE" >/dev/null
SECOND_ADDR=$(docker port "$SECOND_CONTAINER" 8080/tcp)
SECOND_URL="http://127.0.0.1:${SECOND_ADDR##*:}"
wait_http "$SECOND_URL"
CALCRON_URL="$PRIMARY_URL" CALCRON_SECOND_URL="$SECOND_URL" node scripts/e2e.mjs
