param([switch]$InstallDependencies)
$ErrorActionPreference = 'Stop'
$projectRoot = Split-Path $PSScriptRoot -Parent

function Invoke-Check([scriptblock]$Command) {
    & $Command
    if ($LASTEXITCODE -ne 0) { throw "Check failed: $Command" }
}

Push-Location $projectRoot
try {
    node scripts/dependencies.mjs web
    if ($InstallDependencies -or $LASTEXITCODE -ne 0) {
        Invoke-Check { npm --prefix web ci }
    }
    node scripts/dependencies.mjs bridge/claude
    if ($InstallDependencies -or $LASTEXITCODE -ne 0) {
        Invoke-Check { npm --prefix bridge/claude ci }
    }
    Invoke-Check { node scripts/skills.mjs check }
    Invoke-Check { node --test scripts/tests/*.test.mjs }
    Invoke-Check { npm --prefix web test }
    Invoke-Check { npm --prefix web run build }
    Invoke-Check { npm --prefix bridge/claude test }
    Invoke-Check { npm --prefix bridge/claude run build }
    Invoke-Check { node scripts/go-format.mjs }
    Invoke-Check { go vet ./... }
    Invoke-Check { go test ./... }
    Write-Output 'Repository checks passed.'
} finally {
    Pop-Location
}
