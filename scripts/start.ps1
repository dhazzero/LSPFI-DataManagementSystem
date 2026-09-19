$ErrorActionPreference = 'Stop'
$projectDirectory = Split-Path -Parent $PSScriptRoot
Set-Location -LiteralPath $projectDirectory
if (-not (Test-Path -LiteralPath 'config.json')) {
    throw 'Konfigurasi belum disiapkan. Ikuti README.md bagian Persiapan komputer utama.'
}
$configuration = Get-Content -LiteralPath 'config.json' -Raw | ConvertFrom-Json
$appUrl = 'http://' + $configuration.address
$ready = $false
try {
    $health = Invoke-RestMethod -Uri ($appUrl + '/api/health') -TimeoutSec 2
    $ready = $health.application -eq 'lspfi-dms'
} catch { }
if (-not $ready) {
    New-Item -ItemType Directory -Path '.local' -Force | Out-Null
    $process = Start-Process -FilePath (Join-Path $projectDirectory 'LSPFI-Arsip.exe') -WorkingDirectory $projectDirectory -WindowStyle Hidden -RedirectStandardOutput (Join-Path $projectDirectory '.local/server-output.log') -RedirectStandardError (Join-Path $projectDirectory '.local/server-error.log') -PassThru
    for ($attempt = 0; $attempt -lt 20; $attempt++) {
        Start-Sleep -Milliseconds 500
        try {
            $health = Invoke-RestMethod -Uri ($appUrl + '/api/health') -TimeoutSec 2
            if ($health.application -eq 'lspfi-dms') { $ready = $true; break }
        } catch { }
        if ($process.HasExited) { break }
    }
    if (-not $ready) { throw 'Aplikasi belum dapat dijalankan. Pastikan MySQL aktif; periksa .local/server-error.log.' }
}
Start-Process -FilePath $appUrl -WindowStyle Hidden
Write-Host "Arsip LSPFI dibuka: $appUrl"
Write-Host 'Aplikasi tetap aktif di latar belakang. Tutup proses LSPFI-Arsip dari Task Manager untuk menghentikannya.'
