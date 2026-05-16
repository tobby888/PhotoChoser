#!/usr/bin/env bash
set -euo pipefail

if [[ "$(uname -s)" != "Darwin" ]]; then
	echo "macOS packaging must be run on macOS." >&2
	exit 1
fi

APP_NAME="${APP_NAME:-PhotoChoser}"
BUNDLE_ID="${BUNDLE_ID:-com.photochoser.desktop}"
DIST_DIR="${DIST_DIR:-dist}"
BUILD_DIR="$DIST_DIR/macos"
APP_DIR="$BUILD_DIR/$APP_NAME.app"
CONTENTS_DIR="$APP_DIR/Contents"
MACOS_DIR="$CONTENTS_DIR/MacOS"
RESOURCES_DIR="$CONTENTS_DIR/Resources"
DMG_ROOT="$BUILD_DIR/dmgroot"
TMP_DMG="$DIST_DIR/$APP_NAME-macos.tmp.dmg"
DMG_PATH="$DIST_DIR/$APP_NAME-macos.dmg"
GOCACHE="${GOCACHE:-$PWD/.cache/go-build}"
GOMODCACHE="${GOMODCACHE:-$PWD/.cache/mod}"
GOARCH_VALUE="${GOARCH:-$(go env GOARCH)}"
APP_VERSION="${APP_VERSION:-0.1.0}"
BUILD_VERSION="${BUILD_VERSION:-1}"

rm -rf "$APP_DIR" "$DMG_ROOT" "$TMP_DMG" "$DMG_PATH"
mkdir -p "$MACOS_DIR" "$RESOURCES_DIR" "$DMG_ROOT"

echo "Building $APP_NAME for darwin/$GOARCH_VALUE..."
GOOS=darwin GOARCH="$GOARCH_VALUE" CGO_ENABLED=1 GOCACHE="$GOCACHE" GOMODCACHE="$GOMODCACHE" \
	go build -trimpath -ldflags "-s -w" -o "$MACOS_DIR/$APP_NAME" ./cmd/photochoser

cat > "$CONTENTS_DIR/Info.plist" <<PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>CFBundleDevelopmentRegion</key>
	<string>zh_CN</string>
	<key>CFBundleDisplayName</key>
	<string>$APP_NAME</string>
	<key>CFBundleExecutable</key>
	<string>$APP_NAME</string>
	<key>CFBundleIdentifier</key>
	<string>$BUNDLE_ID</string>
	<key>CFBundleInfoDictionaryVersion</key>
	<string>6.0</string>
	<key>CFBundleName</key>
	<string>$APP_NAME</string>
	<key>CFBundlePackageType</key>
	<string>APPL</string>
	<key>CFBundleShortVersionString</key>
	<string>$APP_VERSION</string>
	<key>CFBundleVersion</key>
	<string>$BUILD_VERSION</string>
	<key>LSMinimumSystemVersion</key>
	<string>11.0</string>
	<key>NSHighResolutionCapable</key>
	<true/>
</dict>
</plist>
PLIST

echo "APPL????" > "$CONTENTS_DIR/PkgInfo"
chmod +x "$MACOS_DIR/$APP_NAME"

if command -v codesign >/dev/null 2>&1; then
	echo "Applying ad-hoc signature..."
	codesign --force --deep --sign - "$APP_DIR" >/dev/null
fi

cp -R "$APP_DIR" "$DMG_ROOT/"
ln -s /Applications "$DMG_ROOT/Applications"

echo "Creating DMG..."
hdiutil create -volname "$APP_NAME" -srcfolder "$DMG_ROOT" -ov -format UDRW "$TMP_DMG" >/dev/null
hdiutil convert "$TMP_DMG" -format UDZO -imagekey zlib-level=9 -o "$DMG_PATH" >/dev/null
rm -f "$TMP_DMG"

echo "Created:"
echo "  $APP_DIR"
echo "  $DMG_PATH"
