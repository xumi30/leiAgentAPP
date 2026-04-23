$ErrorActionPreference = "Stop"

$Root = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
Set-Location $Root

$goCache = if ($env:GOCACHE) { $env:GOCACHE } else { Join-Path $Root ".cache/go-build" }
New-Item -ItemType Directory -Force -Path $goCache | Out-Null
$env:GOCACHE = $goCache

& (Join-Path $PSScriptRoot "install-deps.ps1")

$isMacOS = [System.Runtime.InteropServices.RuntimeInformation]::IsOSPlatform(
    [System.Runtime.InteropServices.OSPlatform]::OSX
)

if ($isMacOS) {
    $existingCGOLDFlags = if ($env:CGO_LDFLAGS) { "$($env:CGO_LDFLAGS) " } else { "" }
    $env:CGO_LDFLAGS = "${existingCGOLDFlags}-framework UniformTypeIdentifiers -mmacosx-version-min=10.13"

    Write-Host "==> frontend build"
    Push-Location (Join-Path $Root "frontend")
    try {
        npm run build
    } finally {
        Pop-Location
    }

    Write-Host "==> go build"
    go build -buildvcs=false -trimpath -tags desktop,wv2runtime.download,production -ldflags "-w -s" -o (Join-Path $Root "build/bin/leiAgent")

    Write-Host "==> stage config example"
    $configOutDir = Join-Path $Root "build/bin/config"
    New-Item -ItemType Directory -Force -Path $configOutDir | Out-Null
    Copy-Item -Force (Join-Path $Root "config/config.example.yaml") (Join-Path $configOutDir "config.example.yaml")

    Write-Host "==> package app bundle"
    bash (Join-Path $PSScriptRoot "package-app-macos.sh")
} else {
    Write-Host "==> wails build (production: strip via Wails; -trimpath removes host paths from binary)"
    # Skip Wails' go.mod sync/tidy pass. In this repo it can walk frontend/node_modules
    # under the module root and fail on package-like paths such as @types/*.
    $wailsArgs = @("build", "-trimpath", "-m") + $args
    & wails @wailsArgs
}

Write-Host "==> Output under $Root\build\bin\"
