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
echo "→ building bin/console"
GOROOT=/opt/homebrew/opt/go/libexec go build -o bin/console ./cmd/console

echo "→ starting console on :8080 (kube context: ${CONTEXT})"
./bin/console &
CONSOLE_PID=$!

cleanup() {
  echo ""
  echo "→ stopping console (pid ${CONSOLE_PID})"
  kill "${CONSOLE_PID}" 2>/dev/null || true
}
trap cleanup EXIT INT TERM

echo "→ starting vite dev server pid ${CONSOLE_PID} on :5173 — open http://localhost:5173 (Ctrl+C stops both)"
cd web && npm run dev
