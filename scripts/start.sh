#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
[[ -f config.json ]] || { echo 'Siapkan config.json mengikuti README.md.' >&2; exit 1; }
[[ -x ./lspfi-dms ]] || { echo 'Executable belum tersedia. Jalankan ./scripts/build.sh dahulu.' >&2; exit 1; }
echo 'Buka alamat yang ditampilkan di bawah melalui browser. Tekan Ctrl+C untuk menghentikan aplikasi.'
exec ./lspfi-dms --config config.json
