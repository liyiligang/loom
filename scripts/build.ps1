$ErrorActionPreference = 'Stop'
$projectRoot = Split-Path $PSScriptRoot -Parent
Push-Location $projectRoot
try {
    npm --prefix web ci
    if ($LASTEXITCODE -ne 0) { throw 'Frontend dependency installation failed.' }
    npm --prefix web run build
    if ($LASTEXITCODE -ne 0) { throw 'Frontend build failed.' }
    go build -o bin/loom.exe ./cmd/loom
    if ($LASTEXITCODE -ne 0) { throw 'Go build failed.' }
    Write-Output 'Built bin/loom.exe. Run it to open the workspace at http://127.0.0.1:7331.'
} finally {
    Pop-Location
}
