$ErrorActionPreference = 'Stop'
$projectDirectory = Split-Path -Parent $PSScriptRoot
Set-Location -LiteralPath $projectDirectory
$goBinary = Join-Path $projectDirectory '.tools/go/bin/go.exe'
if (-not (Test-Path -LiteralPath $goBinary)) { $goBinary = (Get-Command go -ErrorAction Stop).Source }
$env:GOPATH = Join-Path $projectDirectory '.tools/gopath'
$env:GOCACHE = Join-Path $projectDirectory '.tools/gocache'
& $goBinary test ./...
if ($LASTEXITCODE -ne 0) { throw 'Pengujian gagal.' }
& $goBinary vet ./...
if ($LASTEXITCODE -ne 0) { throw 'Pemeriksaan kode gagal.' }
& $goBinary build -trimpath -ldflags '-s -w' -o LSPFI-Arsip.exe .
if ($LASTEXITCODE -ne 0) { throw 'Build gagal.' }
Write-Host 'Selesai: LSPFI-Arsip.exe'
