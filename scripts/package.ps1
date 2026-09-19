$ErrorActionPreference = 'Stop'
$projectDirectory = Split-Path -Parent $PSScriptRoot
Set-Location -LiteralPath $projectDirectory
$distribution = Join-Path $projectDirectory 'dist/LSPFI-Arsip-Windows'
New-Item -ItemType Directory -Path $distribution,(Join-Path $distribution 'scripts'),(Join-Path $distribution 'licenses') -Force | Out-Null
foreach ($fileName in @('LSPFI-Arsip.exe','Mulai-LSPFI.cmd','config.example.json','README.md','go.mod','go.sum')) {
    Copy-Item -LiteralPath (Join-Path $projectDirectory $fileName) -Destination (Join-Path $distribution $fileName) -Force
}
Copy-Item -LiteralPath 'scripts/start.ps1' -Destination (Join-Path $distribution 'scripts/start.ps1') -Force
if (Test-Path -LiteralPath 'docs/registerweb-database.md') {
    New-Item -ItemType Directory -Path (Join-Path $distribution 'docs') -Force | Out-Null
    Copy-Item -LiteralPath 'docs/registerweb-database.md','docs/registerweb-schema.json' -Destination (Join-Path $distribution 'docs') -Force
}
$goBinary = Join-Path $projectDirectory '.tools/go/bin/go.exe'
if (-not (Test-Path -LiteralPath $goBinary)) { $goBinary = (Get-Command go -ErrorAction Stop).Source }
$env:GOPATH = Join-Path $projectDirectory '.tools/gopath'
$env:GOCACHE = Join-Path $projectDirectory '.tools/gocache'
$moduleLines = & $goBinary list -deps -f '{{if .Module}}{{.Module.Path}}|{{.Module.Version}}|{{.Module.Dir}}{{end}}' .
if ($LASTEXITCODE -ne 0) { throw 'Daftar dependensi tidak tersedia.' }
$moduleLines = $moduleLines | Where-Object { $_ } | Sort-Object -Unique
$notices = @('# Third-party components', '', 'This application uses the Go runtime and the following unmodified open-source components. Licenses are included in this folder. Source code for each component is available at its module URL and exact version listed below.', '')
foreach ($line in $moduleLines) {
    $parts = $line.Split('|')
    if ($parts[0] -eq 'lspfi-dms') { continue }
    $notices += ('- https://' + $parts[0] + ' — ' + $parts[1])
    $prefix = $parts[0].Replace('/','_')
    Get-ChildItem -LiteralPath $parts[2] -File | Where-Object { $_.Name -match '^(LICENSE|COPYING|NOTICE)' } | ForEach-Object {
        Copy-Item -LiteralPath $_.FullName -Destination (Join-Path $distribution ('licenses/' + $prefix + '-' + $_.Name)) -Force
    }
}
$notices | Set-Content -LiteralPath (Join-Path $distribution 'licenses/NOTICE.md') -Encoding utf8
$goDirectory = Split-Path -Parent (Split-Path -Parent $goBinary)
Copy-Item -LiteralPath (Join-Path $goDirectory 'LICENSE') -Destination (Join-Path $distribution 'licenses/Go-LICENSE') -Force
$archivePath = Join-Path $projectDirectory 'dist/LSPFI-Arsip-Windows.zip'
Compress-Archive -LiteralPath $distribution -DestinationPath $archivePath -Force
Get-FileHash -LiteralPath $archivePath -Algorithm SHA256 | Select-Object Hash,Path
