$ErrorActionPreference = "Stop"

$scriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$root = Split-Path -Parent $scriptDir
. (Join-Path $scriptDir "libraw-env.ps1")
$testDir = Join-Path $root ".cache\libraw-test"
$testExe = Join-Path $testDir "photochoser-preview-libraw.test.exe"

$env:GOCACHE = if ($env:GOCACHE) { $env:GOCACHE } else { Join-Path $root ".cache\go-build" }
$env:GOMODCACHE = if ($env:GOMODCACHE) { $env:GOMODCACHE } else { Join-Path $root ".cache\mod" }
$env:GOPATH = if ($env:GOPATH) { $env:GOPATH } else { Join-Path $root ".cache\go" }
$env:GOOS = "windows"
Set-LibRawBuildEnv "Windows LibRaw tests link the same static LibRaw as release builds."

New-Item -ItemType Directory -Force -Path $testDir | Out-Null

Push-Location $root
try {
    & go test -c -tags=libraw "-ldflags=$WindowsStaticExtLdFlags" -o $testExe ./internal/preview
    if ($LASTEXITCODE -ne 0) {
        throw "go test -c failed with exit code $LASTEXITCODE"
    }
}
finally {
    Pop-Location
}

Assert-WindowsExeSelfContained $testExe

# Run with only the Windows system directories on PATH so a leftover dependency
# on MSYS2, MinGW, or LibRaw DLLs fails here instead of on a user's machine.
$buildPath = $env:PATH
$env:PATH = "$env:SystemRoot\System32;$env:SystemRoot"
Push-Location $testDir
try {
    # Quote the flag: Windows PowerShell 5.1 splits bare "-test.v" at the dot.
    & $testExe "-test.v"
    if ($LASTEXITCODE -ne 0) {
        throw "LibRaw tests failed with exit code $LASTEXITCODE without MinGW/LibRaw on PATH"
    }
}
finally {
    Pop-Location
    $env:PATH = $buildPath
}
