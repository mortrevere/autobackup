#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DOCKER_CONTEXT_ARGS=()
if [[ -n "${DOCKER_CONTEXT:-}" ]]; then
  DOCKER_CONTEXT_ARGS=(--context "$DOCKER_CONTEXT")
fi
COMPOSE=(docker "${DOCKER_CONTEXT_ARGS[@]}" compose -f "$ROOT/docker-compose.e2e.yml" -p autobackup-e2e)

cd "$ROOT"

mkdir -p "$ROOT/e2e/ssh"
if [[ ! -f "$ROOT/e2e/ssh/id_ed25519" || ! -f "$ROOT/e2e/ssh/id_ed25519.pub" ]]; then
  rm -f "$ROOT/e2e/ssh/id_ed25519" "$ROOT/e2e/ssh/id_ed25519.pub"
  ssh-keygen -q -t ed25519 -N "" -C "autobackup-e2e" -f "$ROOT/e2e/ssh/id_ed25519"
fi

scripts/build-linux-amd64.sh

"${COMPOSE[@]}" down -v --remove-orphans
"${COMPOSE[@]}" build
"${COMPOSE[@]}" up -d ssh-server
"${COMPOSE[@]}" run --rm autobackup

"${COMPOSE[@]}" run --rm --no-deps --entrypoint sh ssh-server -c '
  set -eu
  test -f "/tmp/disk/backup root/e2e host/root-note.txt"
  test -f "/tmp/disk/backup root/e2e host/top level space file.txt"
  test -f "/tmp/disk/backup root/e2e host/docs/report.txt"
  test -f "/tmp/disk/backup root/e2e host/docs/folder with spaces/nested note.txt"
  test -f "/tmp/disk/backup root/e2e host/photos/image-001.txt"
  test -f "/tmp/disk/backup root/e2e host/project/src/app.txt"
  test ! -e "/tmp/disk/backup root/e2e host/excluded folder"
  test ! -e "/tmp/disk/backup root/e2e host/excluded folder/should-not-copy.txt"
  test ! -e "/tmp/disk/backup root/e2e host/project/.venv"
  test ! -e "/tmp/disk/backup root/e2e host/project/.venv/should-not-copy.txt"
  grep -q "root level source file" "/tmp/disk/backup root/e2e host/root-note.txt"
  grep -q "top level file with spaces" "/tmp/disk/backup root/e2e host/top level space file.txt"
  grep -q "quarterly backup report" "/tmp/disk/backup root/e2e host/docs/report.txt"
  grep -q "nested file inside paths with spaces" "/tmp/disk/backup root/e2e host/docs/folder with spaces/nested note.txt"
  grep -q "pretend this is a photo" "/tmp/disk/backup root/e2e host/photos/image-001.txt"
  grep -q "normal project source file" "/tmp/disk/backup root/e2e host/project/src/app.txt"
'

"${COMPOSE[@]}" down -v --remove-orphans
printf 'docker e2e passed\n'
