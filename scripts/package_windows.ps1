$ErrorActionPreference = "Stop"

$scriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$root = Split-Path -Parent $scriptDir
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
$env:CGO_ENABLED = "1"

if (-not $env:LIBRAW_DIR -and (-not $env:CGO_CFLAGS -or -not $env:CGO_LDFLAGS)) {
    throw "Set LIBRAW_DIR to a static LibRaw install root, or set CGO_CFLAGS and CGO_LDFLAGS. Windows release packages use embedded LibRaw and produce a single GUI exe."
}

if ($env:LIBRAW_DIR) {
    $includeDir = Join-Path $env:LIBRAW_DIR "include"
    $libDir = Join-Path $env:LIBRAW_DIR "lib"
    if (-not (Test-Path $includeDir)) {
        throw "LibRaw include directory not found: $includeDir"
    }
    if (-not (Test-Path $libDir)) {
        throw "LibRaw library directory not found: $libDir"
    }
    $env:CGO_CFLAGS = "$($env:CGO_CFLAGS) -I`"$includeDir`" -DLIBRAW_NODLL".Trim()
    $env:CGO_LDFLAGS = "$($env:CGO_LDFLAGS) -L`"$libDir`" -Wl,-Bstatic -lraw -lstdc++ -static-libgcc -static-libstdc++ -Wl,-Bdynamic -lws2_32 -lole32 -luuid".Trim()
}

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

Compress-Archive -Path $packageRoot -DestinationPath $zipPath -Force

Write-Host "Created:"
Write-Host "  $output"
Write-Host "  $zipPath"
