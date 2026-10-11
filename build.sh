#!/usr/bin/env bash
# Builds the agent for the Mac and for Windows, from a Mac, and puts in dist/ the two zips the page
# "Scansiona" of Family Manager offers for download, with version.json.
#
#   ./build.sh         the two zips and version.json, in dist/
#   ./build.sh probe   only the probes, in build/: they say what the system sees of the scanner
#
# Needs Go (brew install go) and the command line tools of Xcode, swift included. Neither build/
# nor dist/ is versioned: release.sh publishes the zips as a release on GitHub.
set -euo pipefail
cd "$(dirname "$0")"

VERSION="$(tr -d '[:space:]' < VERSION)"
OUT=build
DEST=dist
APP="$OUT/Family Manager Agent.app"
EXE="$OUT/Family Manager Agent.exe"
MAC_ZIP=FamilyManagerAgent-mac.zip
WINDOWS_ZIP=FamilyManagerAgent-windows.zip
# the icon of the agent, which the program carries inside too (icon.go)
ICON=icon.png
WINRES=github.com/tc-hib/go-winres@v0.3.3

export MACOSX_DEPLOYMENT_TARGET=13.0
mac() { GOOS=darwin GOARCH=arm64 CGO_ENABLED=1 "$@"; }
windows() { GOOS=windows GOARCH=amd64 CGO_ENABLED=0 "$@"; }

mkdir -p "$OUT"

if [ "${1:-}" = "probe" ]; then
    mac go build -trimpath -ldflags "-X main.mode=probe" -o "$OUT/scanner-probe" .
    windows go build -trimpath -ldflags "-X main.mode=probe" -o "$OUT/scanner-probe.exe" .
    echo "Probes in $OUT/: scanner-probe (Mac) and scanner-probe.exe (Windows)."
    echo "Each writes scanner-probe.txt next to itself, without feeding a sheet."
    exit 0
fi

echo "== checks"
mac go vet ./...
windows go vet ./...
# the tests run on the Mac only: of the ones for Windows vet says that they compile. Built for this
# Mac and not for the oldest macOS of the agent, which only fills the output with linker warnings
(unset MACOSX_DEPLOYMENT_TARGET; mac go test -race ./...)

echo "== Mac (arm64)"
rm -rf "$APP"
mkdir -p "$APP/Contents/MacOS" "$APP/Contents/Resources"
mac go build -trimpath -ldflags "-s -w" -o "$APP/Contents/MacOS/FamilyManagerAgent" .
sed "s/__VERSION__/$VERSION/g" Info.plist > "$APP/Contents/Info.plist"

# the icon of the app, from the one of the agent, in the shape macOS wants (see mac-icon.swift)
ICONSET="$OUT/AppIcon.iconset"
rm -rf "$ICONSET" && mkdir -p "$ICONSET"
swift mac-icon.swift "$ICON" "$OUT/AppIcon.png"
for size in 16 32 128 256; do
    sips -z "$size" "$size" "$OUT/AppIcon.png" --out "$ICONSET/icon_${size}x${size}.png" > /dev/null
    sips -z "$((size * 2))" "$((size * 2))" "$OUT/AppIcon.png" --out "$ICONSET/icon_${size}x${size}@2x.png" > /dev/null
done
iconutil -c icns "$ICONSET" -o "$APP/Contents/Resources/AppIcon.icns"
rm -rf "$ICONSET" "$OUT/AppIcon.png"

# a local signature, without a certificate: macOS on Apple processors does not run unsigned code
codesign --force -s - "$APP"

echo "== Windows (amd64)"
rm -f "$EXE"
# the icon of the program, in a file "go build" links by itself; no manifest, which would change how
# the windows look. The first time Go downloads go-winres.
go run "$WINRES" simply --arch amd64 --manifest none --icon "$ICON" --out rsrc
windows go build -trimpath -ldflags "-s -w -H=windowsgui" -o "$EXE" .

echo "== zips"
mkdir -p "$DEST"
rm -f "$DEST/$MAC_ZIP" "$DEST/$WINDOWS_ZIP"
ditto -c -k --keepParent "$APP" "$DEST/$MAC_ZIP"
(cd "$OUT" && zip -q -X "../$DEST/$WINDOWS_ZIP" "Family Manager Agent.exe")
# the fingerprints are what an agent checks before installing the zip it downloaded (update.go)
fingerprint() { shasum -a 256 "$1" | cut -d ' ' -f 1; }
cat > "$DEST/version.json" <<JSON
{
    "version": "$VERSION",
    "built": "$(date +%Y-%m-%d)",
    "files": {
        "mac": "$MAC_ZIP",
        "windows": "$WINDOWS_ZIP"
    },
    "sha256": {
        "mac": "$(fingerprint "$DEST/$MAC_ZIP")",
        "windows": "$(fingerprint "$DEST/$WINDOWS_ZIP")"
    }
}
JSON

ls -lh "$DEST"
echo "Agent $VERSION built, in $DEST/. To publish it: ./release.sh"
