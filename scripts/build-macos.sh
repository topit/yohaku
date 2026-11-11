#!/bin/sh
# Local unsigned bundle. No signing, notarization, upload or installation.
set -eu
[ "$(uname -s)" = Darwin ] || { echo 'Run this script on macOS.' >&2; exit 1; }
root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$root"
version=$(cat VERSION)
printf '%s\n' "$version" | grep -Eq '^[0-9]+\.[0-9]+\.[0-9]+$' || {
    echo 'VERSION must contain a numeric major.minor.patch version.' >&2
    exit 1
}
sh scripts/bootstrap.sh
app="$root/build/Yohaku.app"
mkdir -p "$app/Contents/MacOS" "$app/Contents/Resources"
CGO_ENABLED=0 go build -trimpath -tags mygo_noinspector \
    -ldflags '-s -w -X github.com/egoist/mygo.production=1' \
    -o "$app/Contents/MacOS/Yohaku" ./cmd/yohaku
cat > "$app/Contents/Info.plist" <<PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
<key>CFBundleExecutable</key><string>Yohaku</string>
<key>CFBundleIdentifier</key><string>app.moeit.yohaku</string>
<key>CFBundleName</key><string>Yohaku</string>
<key>CFBundleDisplayName</key><string>Yohaku · 余白</string>
<key>CFBundleDevelopmentRegion</key><string>en</string>
<key>CFBundleLocalizations</key><array><string>en</string><string>zh-Hans</string></array>
<key>CFBundlePackageType</key><string>APPL</string>
<key>CFBundleIconFile</key><string>Yohaku.icns</string>
<key>CFBundleShortVersionString</key><string>$version</string>
<key>CFBundleVersion</key><string>$version</string>
<key>LSMinimumSystemVersion</key><string>12.0</string>
<key>NSHighResolutionCapable</key><true/>
</dict></plist>
PLIST
temp=$(mktemp -d)
trap 'rm -rf "$temp"' EXIT HUP INT TERM
iconset="$temp/Yohaku.iconset"
mkdir -p "$iconset"
for size in 16 32 128 256 512; do
    sips -z "$size" "$size" assets/icon.png --out "$iconset/icon_${size}x${size}.png" >/dev/null
    double=$((size * 2))
    sips -z "$double" "$double" assets/icon.png --out "$iconset/icon_${size}x${size}@2x.png" >/dev/null
done
iconutil -c icns "$iconset" -o "$app/Contents/Resources/Yohaku.icns"
cp -f LICENSE "$app/Contents/Resources/LICENSE"
cp -f THIRD_PARTY_NOTICES.md "$app/Contents/Resources/THIRD_PARTY_NOTICES.md"
licenses="$app/Contents/Resources/licenses"
# A rebuild may change linked modules; do not carry stale dependency notices.
rm -rf "$licenses"
mkdir -p "$licenses/go"
mkdir -p "$licenses/mathjs"
mkdir -p "$licenses/fonts"
cp -f internal/desktop/fonts/*-OFL.txt internal/desktop/fonts/README.md "$licenses/fonts/"
cp -f internal/mathnote/vendor/LICENSE internal/mathnote/vendor/NOTICE \
    internal/mathnote/vendor/math.js.LICENSE.txt \
    internal/mathnote/vendor/THIRD_PARTY_LICENSES.txt "$licenses/mathjs/"
goLicense="$(go env GOROOT)/LICENSE"
[ -f "$goLicense" ] || goLicense="$(go env GOROOT)/../LICENSE" # Homebrew
cp -f "$goLicense" "$licenses/go/"
go list -deps -f '{{if .Module}}{{.Module.Dir}}{{end}}' ./cmd/yohaku | sort -u |
while IFS= read -r dir; do
    [ -n "$dir" ] && [ "$dir" != "$root" ] || continue
    target="$licenses/$(basename "$dir")"
    mkdir -p "$target"
    for file in "$dir"/LICENSE* "$dir"/NOTICE* "$dir"/THIRD_PARTY_NOTICES*; do
        [ ! -f "$file" ] || cp -f "$file" "$target/"
    done
done
plutil -lint "$app/Contents/Info.plist"
echo "$app"
