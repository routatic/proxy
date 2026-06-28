#!/bin/bash
set -e

# Version passed as first argument, fallback to git describe or dev
VERSION="${1:-$(git describe --tags --always --dirty 2>/dev/null || echo "dev")}"
VERSION_NO_V="${VERSION#v}"

echo "Packaging RoutaticProxy version: ${VERSION} (clean version: ${VERSION_NO_V})"

# Directories
APP_NAME="RoutaticProxy"
DMG_NAME="RoutaticProxy.dmg"
BUILD_DIR="bin"
APP_DIR="${BUILD_DIR}/${APP_NAME}.app"
CONTENTS_DIR="${APP_DIR}/Contents"
MAC_DIR="${CONTENTS_DIR}/MacOS"
RES_DIR="${CONTENTS_DIR}/Resources"

# 1. Verify CGO binary exists
if [ ! -f "${BUILD_DIR}/routatic-proxy" ]; then
    echo "Error: ${BUILD_DIR}/routatic-proxy not found. Please compile it first (e.g. make build-ui)."
    exit 1
fi

# 2. Build App Bundle structure
echo "Assembling App Bundle..."
rm -rf "${APP_DIR}"
mkdir -p "${MAC_DIR}"
mkdir -p "${RES_DIR}"

# 3. Copy binary and make executable
cp "${BUILD_DIR}/routatic-proxy" "${MAC_DIR}/routatic-proxy"
chmod +x "${MAC_DIR}/routatic-proxy"

# 4. Generate Info.plist
echo "Generating Info.plist..."
cat > "${CONTENTS_DIR}/Info.plist" <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>CFBundleExecutable</key>
    <string>routatic-proxy</string>
    <key>CFBundleIconFile</key>
    <string>icon.icns</string>
    <key>CFBundleIdentifier</key>
    <string>com.routatic.proxy</string>
    <key>CFBundleInfoDictionaryVersion</key>
    <string>6.0</string>
    <key>CFBundleName</key>
    <string>RoutaticProxy</string>
    <key>CFBundlePackageType</key>
    <string>APPL</string>
    <key>CFBundleShortVersionString</key>
    <string>${VERSION_NO_V}</string>
    <key>CFBundleVersion</key>
    <string>${VERSION_NO_V}</string>
    <key>LSMinimumSystemVersion</key>
    <string>10.15</string>
    <key>LSUIElement</key>
    <true/>
    <key>NSHighResolutionCapable</key>
    <true/>
</dict>
</plist>
EOF

# 5. Generate Icon (.icns) if scripts/icon.png exists
if [ -f "scripts/icon.png" ]; then
    echo "Generating icon.icns from scripts/icon.png..."
    ICON_SET="bin/icon.iconset"
    rm -rf "${ICON_SET}"
    mkdir -p "${ICON_SET}"
    
    # Ensure source icon is converted to true PNG data format first
    sips -s format png scripts/icon.png --out bin/icon_tmp.png > /dev/null 2>&1
    SRC_PNG="bin/icon_tmp.png"
    
    # Resize PNG to standard Apple Icon sizes
    sips -z 16 16     "${SRC_PNG}" --out "${ICON_SET}/icon_16x16.png" > /dev/null 2>&1 || true
    sips -z 32 32     "${SRC_PNG}" --out "${ICON_SET}/icon_16x16@2x.png" > /dev/null 2>&1 || true
    sips -z 32 32     "${SRC_PNG}" --out "${ICON_SET}/icon_32x32.png" > /dev/null 2>&1 || true
    sips -z 64 64     "${SRC_PNG}" --out "${ICON_SET}/icon_32x32@2x.png" > /dev/null 2>&1 || true
    sips -z 128 128   "${SRC_PNG}" --out "${ICON_SET}/icon_128x128.png" > /dev/null 2>&1 || true
    sips -z 256 256   "${SRC_PNG}" --out "${ICON_SET}/icon_128x128@2x.png" > /dev/null 2>&1 || true
    sips -z 256 256   "${SRC_PNG}" --out "${ICON_SET}/icon_256x256.png" > /dev/null 2>&1 || true
    sips -z 512 512   "${SRC_PNG}" --out "${ICON_SET}/icon_256x256@2x.png" > /dev/null 2>&1 || true
    sips -z 512 512   "${SRC_PNG}" --out "${ICON_SET}/icon_512x512.png" > /dev/null 2>&1 || true
    sips -z 1024 1024 "${SRC_PNG}" --out "${ICON_SET}/icon_512x512@2x.png" > /dev/null 2>&1 || true
    
    # Compile iconset to icns
    iconutil -c icns "${ICON_SET}"
    mv bin/icon.icns "${RES_DIR}/icon.icns"
    rm -rf "${ICON_SET}"
    rm -f bin/icon_tmp.png
else
    echo "Warning: scripts/icon.png not found, building without custom icon."
fi

# 6. Build DMG
echo "Packaging DMG installer..."
rm -f "${BUILD_DIR}/${DMG_NAME}"

# Check if create-dmg is available (e.g. from Homebrew)
if command -v create-dmg >/dev/null 2>&1; then
    echo "create-dmg found, building a styled DMG..."
    create-dmg \
      --volname "Routatic Proxy" \
      --volicon "${RES_DIR}/icon.icns" \
      --window-pos 200 120 \
      --window-size 600 400 \
      --icon-size 100 \
      --icon "${APP_NAME}.app" 175 190 \
      --hide-extension "${APP_NAME}.app" \
      --app-drop-link 425 190 \
      "${BUILD_DIR}/${DMG_NAME}" \
      "${APP_DIR}"
else
    echo "create-dmg not found, falling back to hdiutil..."
    # Create temp directory
    TEMP_DMG_DIR="bin/dmg_temp"
    rm -rf "${TEMP_DMG_DIR}"
    mkdir -p "${TEMP_DMG_DIR}"
    cp -R "${APP_DIR}" "${TEMP_DMG_DIR}/"
    ln -s /Applications "${TEMP_DMG_DIR}/Applications"
    
    # Create plain DMG
    hdiutil create -volname "Routatic Proxy" -srcfolder "${TEMP_DMG_DIR}" -ov -format UDZO "${BUILD_DIR}/${DMG_NAME}"
    rm -rf "${TEMP_DMG_DIR}"
fi

echo "Successfully built ${BUILD_DIR}/${DMG_NAME}!"
