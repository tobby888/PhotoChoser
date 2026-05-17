$ErrorActionPreference = "Stop"

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
        $env:CGO_LDFLAGS = "$($env:CGO_LDFLAGS) -L`"$libDir`" -Wl,-Bstatic -lraw -lstdc++ -static-libgcc -static-libstdc++ -Wl,-Bdynamic -lws2_32 -lole32 -luuid".Trim()
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
            $env:CGO_LDFLAGS = "$($env:CGO_LDFLAGS) $libs".Trim()
            return
        }
    }

    throw "Set LIBRAW_DIR to a static LibRaw install root, set CGO_CFLAGS and CGO_LDFLAGS, or install pkg-config with a libraw.pc file. $Purpose"
}
