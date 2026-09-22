#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
command -v go >/dev/null || { echo 'Go 1.26 atau lebih baru diperlukan untuk build.' >&2; exit 1; }
go test ./...
go vet ./...
go build -trimpath -o lspfi-dms .
echo 'Build selesai. Jalankan: ./scripts/start.sh'
