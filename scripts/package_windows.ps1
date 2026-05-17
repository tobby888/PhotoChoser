$ErrorActionPreference = "Stop"

$scriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$root = Split-Path -Parent $scriptDir
. (Join-Path $scriptDir "libraw-env.ps1")
$appName = if ($env:APP_NAME) { $env:APP_NAME } else { "PhotoChoser" }
$version = if ($env:APP_VERSION) { $env:APP_VERSION } else { "0.1.0" }
$arch = if ($env:GOARCH) { $env:GOARCH } else { "amd64" }
$distDir = if ($env:DIST_DIR) { Join-Path $root $env:DIST_DIR } else { Join-Path $root "dist" }
$releaseDir = if ($env:RELEASE_DIR) { Join-Path $root $env:RELEASE_DIR } else { Join-Path $distDir "release" }
$buildDir = Join-Path $distDir "windows"
$packageRoot = Join-Path $buildDir "$appName-$version-windows-$arch"
$output = Join-Path $packageRoot "$appName.exe"
$zipPath = Join-Path $releaseDir "$appName-$version-windows-$arch.zip"

$env:GOCACHE = if ($env:GOCACHE) { $env:GOCACHE } else { Join-Path $root ".cache\go-build" }
$env:GOMODCACHE = if ($env:GOMODCACHE) { $env:GOMODCACHE } else { Join-Path $root ".cache\mod" }
$env:GOPATH = if ($env:GOPATH) { $env:GOPATH } else { Join-Path $root ".cache\go" }
$env:GOOS = "windows"
$env:GOARCH = $arch
Set-LibRawBuildEnv "Windows release packages use embedded LibRaw and produce a single GUI exe."

Remove-Item -Recurse -Force $packageRoot -ErrorAction SilentlyContinue
Remove-Item -Force $zipPath -ErrorAction SilentlyContinue
New-Item -ItemType Directory -Force -Path $packageRoot, $releaseDir | Out-Null

Push-Location $root
try {
    & go build -tags=libraw -trimpath -ldflags="-s -w -H=windowsgui" -o $output ./cmd/photochoser
    if ($LASTEXITCODE -ne 0) {
        throw "go build failed with exit code $LASTEXITCODE"
    }
}
finally {
    Pop-Location
}

Compress-Archive -Path $output -DestinationPath $zipPath -Force

Write-Host "Created:"
Write-Host "  $output"
Write-Host "  $zipPath"
