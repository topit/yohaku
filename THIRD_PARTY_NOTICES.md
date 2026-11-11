# MyGo

Yohaku depends on github.com/egoist/mygo v0.8.3. The native window/menu
bootstrap in cmd/blockspike follows its examples/code-editor/main.go.
The image-and-text editor uses its native ui/richtext package. Six explicit
patches under patches/ add editor transaction policy, line dividers, and rich
clipboard delegates/scroll persistence plus the macOS flat-RTFD type mapping.
The inlay cursor patch adds opt-in placement before display-only Math results.
The hit-test patch exposes shaped line/inlay targets for gutter actions and
result copying, without changing the editor's input or mutation behavior.
The inlay padding patch adds display-only spacing for aligned Math results;
lines too wide to fit retain normal wrapping, source offsets and undo history.
scripts/bootstrap.sh reproduces these changes in the project-local dependency;
the shared module cache is not modified. MyGo upgrades require patch review.

MIT License

Copyright (c) 2026 MyGo contributors

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.

# Bundled editor fonts

Yohaku embeds unmodified Regular faces of JetBrains Mono (JetBrains Mono
Project Authors, 2020), IBM Plex Mono (IBM Corp., 2017, Reserved Font Name
"Plex"), and Source Code Pro (Adobe, 2023, Reserved Font Name "Source").
All three permit bundling under the SIL Open Font License 1.1. Full copyright
notices, licenses, pinned download sources and SHA-256 checksums are under
internal/desktop/fonts; build-macos.sh also places the notices in the app's
Contents/Resources/licenses/fonts directory. They are not installed system-wide.

# Heynote protocol reference (not bundled)

The importer independently reads the Heynote text-file protocol, observed at
heyman/heynote c301e42f9d432d1449212d36324f5028ca18880c. No Heynote implementation,
runtime, or Heynote-distributed bundle is included in Yohaku. Heynote's own license includes
the Commons Clause condition in addition to MIT; it is not treated here as an
unrestricted MIT dependency. The Math implementation is independent.

# Math engine

Yohaku embeds the official mathjs 14.8.1 browser distribution, obtained directly
from its npm package and verified against its SHA-512 integrity. It is NOT the
copy distributed by Heynote. Upstream: https://github.com/josdejong/mathjs .
math.js is Apache-2.0, Copyright (C) 2013-2025 Jos de Jong.
The unmodified bundle, full LICENSE, NOTICE, bundle notices and dependency
licenses are in internal/mathnote/vendor; these notices accompany the .app.

The JavaScript VM is github.com/MoeForge/moejs
v0.1.1-0.20261109125924-934171643e14, Apache-2.0. Its full LICENSE and
THIRD_PARTY_NOTICES (including Unicode/ICU/V8 notices), plus licenses of linked
Go modules, are collected by build-macos.sh. No optional upstream benchmark
plugins are included.

# Yohaku artwork

assets/logo.png was generated for this Yohaku thread with Amp Painter.
assets/icon.png is its rounded-corner macOS app-icon derivative. It does not
reuse Heynote or mojian artwork. The warm ivory tile is intentionally opaque;
only the exterior corner pixels are transparent.

# Mojian

The compact title-bar tab track, tab sizing/reveal logic and paper-card palette
in internal/desktop/view.go are adapted from ox0pine/mojian (MIT), inspected at
466c169523d0f0f2b6d1734d1aaf69b59eec15c6. No project tree, split workspace,
terminal, Git or remote-project implementation is copied into Yohaku.

MIT License

Copyright (c) 2026 ox0pine

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
