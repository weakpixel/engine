# Third-party notices

The Weakpixel engine is MIT licensed ([LICENSE](LICENSE)). It builds on the software listed below. All of it allows commercial use; none of it is GPL, AGPL, LGPL or non-commercial. A game binary or web build you distribute links some of these modules and should carry their notices: the MIT, BSD and Apache-2.0 licenses ask for the copyright and license text to come with binary redistribution.

The license texts are in each module's source (`go mod download -json <module>` prints its folder in the module cache). Versions are those of [go.mod](go.mod).

## Go modules linked into games

| Module | License | Notes |
| --- | --- | --- |
| [github.com/weakpixel/ecs](https://github.com/weakpixel/ecs) | MIT | |
| [github.com/weakpixel/asset](https://github.com/weakpixel/asset) | MIT | |
| [github.com/weakpixel/voice](https://github.com/weakpixel/voice) | MIT | linked by `speech` |
| [github.com/hajimehoshi/ebiten/v2](https://github.com/hajimehoshi/ebiten) | Apache-2.0 | its NOTICE.md reproduces the notices of GLFW (zlib), Go (BSD-3-Clause), go-gl/gl and go-gl/glfw (MIT) and srwiley/oksvg (BSD-3-Clause) |
| [github.com/ebitengine/oto/v3](https://github.com/ebitengine/oto) | Apache-2.0 | |
| [github.com/ebitengine/purego](https://github.com/ebitengine/purego) | Apache-2.0 | |
| [github.com/ebitengine/hideconsole](https://github.com/ebitengine/hideconsole) | Apache-2.0 | |
| [github.com/ebitengine/gomobile](https://github.com/ebitengine/gomobile) | BSD-3-Clause | plus the Go patent grant |
| [github.com/hajimehoshi/go-mp3](https://github.com/hajimehoshi/go-mp3) | Apache-2.0 | |
| [github.com/jfreymuth/oggvorbis](https://github.com/jfreymuth/oggvorbis), [vorbis](https://github.com/jfreymuth/vorbis), [pulse](https://github.com/jfreymuth/pulse) | MIT | |
| [github.com/go-text/typesetting](https://github.com/go-text/typesetting) | Unlicense or BSD-3-Clause | the test fonts in its `font/testdata` are not compiled in |
| [github.com/srwiley/rasterx](https://github.com/srwiley/rasterx) | BSD-3-Clause | |
| [golang.org/x/image](https://pkg.go.dev/golang.org/x/image), [x/sync](https://pkg.go.dev/golang.org/x/sync), [x/sys](https://pkg.go.dev/golang.org/x/sys), [x/text](https://pkg.go.dev/golang.org/x/text) | BSD-3-Clause | plus the Go patent grant |
| `wasm_exec.js` | BSD-3-Clause | copied from the Go toolchain into a web build by `web/build.sh` |

## Go modules used only by the tools

These are linked by `weakpixel` and `weakpixel-editor`, never by a game.

| Module | License | Notes |
| --- | --- | --- |
| [github.com/spf13/cobra](https://github.com/spf13/cobra) | Apache-2.0 | |
| [github.com/spf13/pflag](https://github.com/spf13/pflag) | BSD-3-Clause | |
| [github.com/inconshreveable/mousetrap](https://github.com/inconshreveable/mousetrap) | Apache-2.0 | |
| [golang.org/x/tools](https://pkg.go.dev/golang.org/x/tools), [x/mod](https://pkg.go.dev/golang.org/x/mod) | BSD-3-Clause | the generator |
| [github.com/ebitengine/debugui](https://github.com/ebitengine/debugui) | Apache-2.0 | native `weakpixel-editor` only |
| [github.com/hajimehoshi/bitmapfont/v4](https://github.com/hajimehoshi/bitmapfont) | Apache-2.0 (code) | debugui's font; its embedded bitmap fonts carry their own licenses (OFL-1.1, Baekmuk, M+ Bitmap, public domain), listed in that module |
| [github.com/pierrec/lz4/v4](https://github.com/pierrec/lz4) | BSD-3-Clause | used by bitmapfont |
| [github.com/google/go-cmp](https://github.com/google/go-cmp) | BSD-3-Clause | tests only |

## Files in this repository

| Files | License |
| --- | --- |
| `editor/toolshell/icons/*.svg` and the PNGs rendered from them | [Tabler Icons](https://github.com/tabler/tabler-icons), MIT, Copyright (c) 2020-2026 Paweł Kuna ([editor/toolshell/icons/LICENSE](editor/toolshell/icons/LICENSE)) |
