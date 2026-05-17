$ErrorActionPreference = "Stop"

$scriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$root = Split-Path -Parent $scriptDir
$binDir = Join-Path $root "bin"
$output = Join-Path $binDir "PhotoChoser.exe"

$env:GOCACHE = Join-Path $root ".cache\go-build"
$env:GOMODCACHE = Join-Path $root ".cache\mod"
$env:GOPATH = Join-Path $root ".cache\go"
$env:GOOS = "windows"
$env:CGO_ENABLED = "1"

if (-not $env:LIBRAW_DIR -and (-not $env:CGO_CFLAGS -or -not $env:CGO_LDFLAGS)) {
    throw "Set LIBRAW_DIR to a static LibRaw install root, or set CGO_CFLAGS and CGO_LDFLAGS. Windows builds use embedded LibRaw and produce a single GUI exe."
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

New-Item -ItemType Directory -Force -Path $binDir | Out-Null

Push-Location $root
try {
    & go build -tags=libraw -ldflags="-H=windowsgui" -o $output ./cmd/photochoser
    if ($LASTEXITCODE -ne 0) {
        throw "go build failed with exit code $LASTEXITCODE"
    }
}
finally {
    Pop-Location
}

Write-Host "Built $output"
