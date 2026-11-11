#!/bin/sh
# Reproduce the narrowly patched dependency without editing the module cache.
set -eu
root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$root"
stamp=$(cat patches/mygo-editor-policy.patch patches/mygo-line-dividers.patch patches/mygo-richtext-transfer.patch patches/mygo-inlay-cursor.patch patches/mygo-editor-hit-test.patch patches/mygo-inlay-padding.patch | cksum)
if [ -f .deps/mygo/.yohaku-patch ]; then
    if [ "$(cat .deps/mygo/.yohaku-patch)" = "$stamp" ]; then exit 0; fi
    echo 'Refreshing disposable MyGo cache for the updated patches.'
else
    [ ! -e .deps/mygo ] || { echo '.deps/mygo already exists without a patch stamp.' >&2; exit 1; }
fi
src=$(go mod download -json github.com/egoist/mygo@v0.8.3 | python3 -c 'import json,sys; print(json.load(sys.stdin)["Dir"])')
mkdir -p .deps
temp=$(mktemp -d "$root/.deps/mygo.XXXXXX")
trap 'rm -rf "$temp"' EXIT HUP INT TERM
cp -R "$src/." "$temp/"
chmod -R u+w "$temp"
patch -s -p1 -d "$temp" < patches/mygo-editor-policy.patch
patch -s -p1 -d "$temp" < patches/mygo-line-dividers.patch
patch -s -p1 -d "$temp" < patches/mygo-richtext-transfer.patch
patch -s -p1 -d "$temp" < patches/mygo-inlay-cursor.patch
patch -s -p1 -d "$temp" < patches/mygo-editor-hit-test.patch
patch -s -p1 -d "$temp" < patches/mygo-inlay-padding.patch
printf '%s\n' "$stamp" > "$temp/.yohaku-patch"
# Only a cache bearing our stamp can reach this point. Prepare the replacement
# completely before removing it; authored changes belong in patches/.
[ ! -e .deps/mygo ] || rm -rf .deps/mygo
mv "$temp" .deps/mygo
echo 'MyGo v0.8.3 + local editor patches ready.'
