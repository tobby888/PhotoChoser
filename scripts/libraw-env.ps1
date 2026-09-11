$ErrorActionPreference = "Stop"

# Link LibRaw, its codec dependencies, and the MinGW C/C++/OpenMP runtimes
# statically so Windows builds only import DLLs that ship with Windows.
$WindowsStaticExtLdFlags = "-extldflags=-static"

function Set-LibRawBuildEnv {
    param(
        [string]$Purpose = "Windows builds use embedded LibRaw and produce a single GUI exe."
    )

    $env:CGO_ENABLED = "1"

    $msysRoots = @()
    if ($env:MSYS2_LOCATION) {
        $msysRoots += $env:MSYS2_LOCATION
    }
    $msysRoots += "C:\msys64"
    if ($env:RUNNER_TEMP) {
        $msysRoots += (Join-Path $env:RUNNER_TEMP "msys64")
    }
    foreach ($root in $msysRoots) {
        if (-not $root) {
            continue
        }
        $ucrtBin = Join-Path $root "ucrt64\bin"
        $usrBin = Join-Path $root "usr\bin"
        if ((Test-Path $ucrtBin) -and ($env:PATH -notlike "*$ucrtBin*")) {
            $env:PATH = "$ucrtBin;$env:PATH"
        }
        if ((Test-Path $usrBin) -and ($env:PATH -notlike "*$usrBin*")) {
            $env:PATH = "$usrBin;$env:PATH"
        }
        $pkgConfigDir = Join-Path $root "ucrt64\lib\pkgconfig"
        if ((Test-Path $pkgConfigDir) -and ($env:PKG_CONFIG_PATH -notlike "*$pkgConfigDir*")) {
            $env:PKG_CONFIG_PATH = "$pkgConfigDir;$env:PKG_CONFIG_PATH".TrimEnd(";")
        }
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
        $env:CGO_LDFLAGS = "$($env:CGO_LDFLAGS) -L`"$libDir`" -lraw -lstdc++ -lws2_32 -lole32 -luuid".Trim()
        return
    }

    if ($env:CGO_CFLAGS -and $env:CGO_LDFLAGS) {
        $env:CGO_CFLAGS = "$($env:CGO_CFLAGS) -DLIBRAW_NODLL".Trim()
        return
    }

    $pkgConfig = Get-Command pkg-config -ErrorAction SilentlyContinue
    if (-not $pkgConfig) {
        $pkgConfig = Get-Command pkgconf -ErrorAction SilentlyContinue
    }
    if ($pkgConfig) {
        $packageNames = @("libraw", "libraw_r", "LibRaw")
        foreach ($packageName in $packageNames) {
            & $pkgConfig.Source --exists $packageName
            if ($LASTEXITCODE -ne 0) {
                continue
            }
            $cflags = (& $pkgConfig.Source --cflags $packageName) -join " "
            if ($LASTEXITCODE -ne 0) {
                throw "$($pkgConfig.Name) --cflags $packageName failed with exit code $LASTEXITCODE"
            }
            $libs = (& $pkgConfig.Source --libs --static $packageName) -join " "
            if ($LASTEXITCODE -ne 0) {
                throw "$($pkgConfig.Name) --libs --static $packageName failed with exit code $LASTEXITCODE"
            }
            $env:CGO_CFLAGS = "$($env:CGO_CFLAGS) $cflags -DLIBRAW_NODLL".Trim()
            # MSYS2's libraw.pc omits the C++ runtime and zlib, which a static
            # libraw.a still needs.
            $env:CGO_LDFLAGS = "$($env:CGO_LDFLAGS) $libs -lstdc++ -lz -lws2_32".Trim()
            return
        }
    }

    throw "Set LIBRAW_DIR to a static LibRaw install root, set CGO_CFLAGS and CGO_LDFLAGS, or install pkg-config with a libraw.pc file. $Purpose"
}

function Assert-WindowsExeSelfContained {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Path
    )

    $objdump = Get-Command objdump -ErrorAction SilentlyContinue
    if (-not $objdump) {
        throw "objdump not found on PATH; install MinGW binutils to verify $Path"
    }
    $headers = & $objdump.Source -p $Path
    if ($LASTEXITCODE -ne 0) {
        throw "objdump -p $Path failed with exit code $LASTEXITCODE"
    }

    $system32 = Join-Path $env:SystemRoot "System32"
    $dlls = @($headers | Select-String -Pattern "DLL Name:\s*(\S+)" | ForEach-Object { $_.Matches[0].Groups[1].Value } | Sort-Object -Unique)
    if ($dlls.Count -eq 0) {
        throw "objdump reported no DLL imports for $Path"
    }
    $external = @($dlls | Where-Object {
        $name = $_
        $isApiSet = $name -match "^(api|ext)-ms-win-"
        $isSystemDll = ($name -notmatch "^lib") -and (Test-Path (Join-Path $system32 $name))
        -not ($isApiSet -or $isSystemDll)
    })
    if ($external.Count -gt 0) {
        throw "$Path depends on non-system DLLs: $($external -join ', '). Windows builds must link LibRaw and the MinGW runtime statically."
    }

    Write-Host "Verified $Path only imports Windows system DLLs: $($dlls -join ', ')"
}
