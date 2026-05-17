$ErrorActionPreference = "Stop"

$scriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$root = Split-Path -Parent $scriptDir
. (Join-Path $scriptDir "libraw-env.ps1")
$binDir = Join-Path $root "bin"
$output = Join-Path $binDir "PhotoChoser.exe"

$env:GOCACHE = Join-Path $root ".cache\go-build"
$env:GOMODCACHE = Join-Path $root ".cache\mod"
$env:GOPATH = Join-Path $root ".cache\go"
$env:GOOS = "windows"
Set-LibRawBuildEnv "Windows builds use embedded LibRaw and produce a single GUI exe."

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
