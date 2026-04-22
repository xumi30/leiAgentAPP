$ErrorActionPreference = "Stop"

$Root = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
Set-Location $Root

$goCache = if ($env:GOCACHE) { $env:GOCACHE } else { Join-Path $Root ".cache/go-build" }
New-Item -ItemType Directory -Force -Path $goCache | Out-Null
$env:GOCACHE = $goCache

& (Join-Path $PSScriptRoot "install-deps.ps1")

Write-Host "==> wails build (production: strip via Wails; -trimpath removes host paths from binary)"
$wailsArgs = @("build", "-trimpath") + $args
& wails @wailsArgs

Write-Host "==> Output under $Root\build\bin\"
