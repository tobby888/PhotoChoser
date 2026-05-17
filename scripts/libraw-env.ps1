$ErrorActionPreference = "Stop"

function Set-LibRawBuildEnv {
    param(
        [string]$Purpose = "Windows builds use embedded LibRaw and produce a single GUI exe."
    )

    $env:CGO_ENABLED = "1"

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
    if ($pkgConfig) {
        & $pkgConfig.Source --exists libraw
        if ($LASTEXITCODE -eq 0) {
            $cflags = (& $pkgConfig.Source --cflags libraw) -join " "
            if ($LASTEXITCODE -ne 0) {
                throw "pkg-config --cflags libraw failed with exit code $LASTEXITCODE"
            }
            $libs = (& $pkgConfig.Source --libs --static libraw) -join " "
            if ($LASTEXITCODE -ne 0) {
                throw "pkg-config --libs --static libraw failed with exit code $LASTEXITCODE"
            }
            $env:CGO_CFLAGS = "$($env:CGO_CFLAGS) $cflags -DLIBRAW_NODLL".Trim()
            $env:CGO_LDFLAGS = "$($env:CGO_LDFLAGS) $libs".Trim()
            return
        }
    }

    throw "Set LIBRAW_DIR to a static LibRaw install root, set CGO_CFLAGS and CGO_LDFLAGS, or install pkg-config with a libraw.pc file. $Purpose"
}
