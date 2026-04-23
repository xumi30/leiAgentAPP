$ErrorActionPreference = "Stop"

$Root = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
Set-Location $Root

$goCache = if ($env:GOCACHE) { $env:GOCACHE } else { Join-Path $Root ".cache/go-build" }
New-Item -ItemType Directory -Force -Path $goCache | Out-Null
$env:GOCACHE = $goCache

Write-Host "==> go mod download"
go mod download

Write-Host "==> frontend dependencies"
Push-Location (Join-Path $Root "frontend")
try {
    npm install
} finally {
    Pop-Location
}

Write-Host "==> install-deps done"
