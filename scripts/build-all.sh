#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
"$ROOT/scripts/build-linux-amd64.sh"
"$ROOT/scripts/build-windows-amd64.sh"
