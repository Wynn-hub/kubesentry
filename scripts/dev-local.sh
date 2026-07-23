#!/usr/bin/env bash
# One-command local dev loop for the console: builds bin/console, runs it
# against the current kubeconfig context, and runs the vite dev server with
# hot reload — no Docker image build, no Helm deploy.
#
# Only Go backend changes need a re-run of this script (Ctrl+C, then run it
# again); .vue/.ts frontend edits hot-reload automatically via vite.
set -euo pipefail
cd "$(dirname "$0")/.."

CONTEXT="$(kubectl config current-context 2>/dev/null || echo unknown)"
NAMESPACE="${LOCAL_NAMESPACE:-kubesentry-system}"
DB_PORT="${DEV_LOCAL_DB_PORT:-15432}"
DB_LOG="$(mktemp -t kubesentry-dev-db.XXXXXX)"
DB_FORWARD_PID=""
CONSOLE_PID=""

cleanup() {
  if [ -n "${CONSOLE_PID}" ]; then
    echo ""
    echo "→ stopping console (pid ${CONSOLE_PID})"
    kill "${CONSOLE_PID}" 2>/dev/null || true
  fi
  if [ -n "${DB_FORWARD_PID}" ]; then
    echo "→ stopping PostgreSQL port-forward (pid ${DB_FORWARD_PID})"
    kill "${DB_FORWARD_PID}" 2>/dev/null || true
  fi
  rm -f "${DB_LOG}"
}
trap cleanup EXIT INT TERM

echo "→ forwarding PostgreSQL service from ${NAMESPACE} to localhost:${DB_PORT}"
kubectl -n "${NAMESPACE}" port-forward service/kubesentry-postgres "${DB_PORT}:5432" >"${DB_LOG}" 2>&1 &
DB_FORWARD_PID=$!

for _ in {1..30}; do
  if nc -z 127.0.0.1 "${DB_PORT}" 2>/dev/null; then
    break
  fi
  if ! kill -0 "${DB_FORWARD_PID}" 2>/dev/null; then
    cat "${DB_LOG}" >&2
    exit 1
  fi
  sleep 1
done
if ! nc -z 127.0.0.1 "${DB_PORT}" 2>/dev/null; then
  echo "PostgreSQL port-forward did not become ready" >&2
  cat "${DB_LOG}" >&2
  exit 1
fi

DATABASE_DSN="$(kubectl -n "${NAMESPACE}" get secret kubesentry-postgres-credentials -o jsonpath='{.data.dsn}' | base64 --decode)"
DATABASE_DSN="${DATABASE_DSN/kubesentry-postgres.${NAMESPACE}.svc:5432/127.0.0.1:${DB_PORT}}"

echo "→ building bin/console"
GOROOT=/opt/homebrew/opt/go/libexec go build -o bin/console ./cmd/console

echo "→ starting console on :8080 (kube context: ${CONTEXT})"
DATABASE_DSN="${DATABASE_DSN}" ./bin/console &
CONSOLE_PID=$!

echo "→ starting vite dev server pid ${CONSOLE_PID} on :5173 — open http://localhost:5173 (Ctrl+C stops both)"
cd web && npm run dev
