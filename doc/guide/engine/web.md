# Running in the browser

A game built on `game.Engine` runs in a browser as WebAssembly with no code changes: Ebiten's js backend draws with WebGL2 and plays audio with WebAudio, and nothing in the engine uses cgo. This page covers building, the web shell, loading assets, and what behaves differently in a browser.

## Build

Standard Go, `GOOS=js GOARCH=wasm` (TinyGo is not supported by Ebiten). A game builds with the engine's script:

```bash
web/build.sh ./mygame build/web/mygame   # -> build/web/mygame/{index.html,wasm_exec.js,game.wasm}
npx http-server build/web/mygame                # any static server; file:// does not work
```

`web/build.sh <pkgdir> <outdir>` builds the main package in `<pkgdir>` (of any module, from any directory) and copies the shell next to it. The output is three static files. A game with embedded assets is 25 to 30 MB raw and a third of that gzipped; serve it compressed.

`make wasm-build` cross-compiles every package for the browser; it is an explicit opt-in target, not part of `make all`. `web/smoke.cjs` runs builds in headless Chromium and fails on a Go panic, a non-zero exit or a page error ([tools](../tools.md#make-targets)).

## The web shell

`web/index.html` loads `game.wasm` with a progress text, then shows "Click to start". The click (or the first key) starts the game, focuses the page and is the user gesture browsers require before audio can play. Starting the game only at the click means its [startup screen](engine.md#startup-screen) runs in view; assets start loading then too. Ebiten adds its own canvas and scales it to the window. Go's panic output, a fatal error or a non-zero exit code is also shown in a red box at the bottom of the page, not only in the console. Copy and change the file for your game; it has no build step.

## Assets

`Config.FS` works as on the desktop:

- **Embedded** (`embed.FS`): nothing to serve, but the assets are part of the download. An [asset library](asset-library.md#embedding) generates its `go:embed` patterns, so files its `.assetignore` files ignore stay out.
- **Over HTTP** with [`httpfs`](assets.md#reading-assets-over-http) (`github.com/weakpixel/asset/httpfs`): the game downloads files as it loads them.

`os.DirFS` does not exist in a browser. Hot reload by polling (`WatchInterval`) is for local files; over HTTP, the dev server `wpix serve` finds the changes and pushes them to the page, which feeds them to `Config.AssetChanges` with `httpfs.WatchChanges` ([hot reload](assets.md#hot-reload)). A game made with `wpix new` does this with `make web-dev`: it builds with the tags `webassets,webdev`, serves `assets/` as loose files (the server builds the catalog the browser cannot list) and reloads whatever you save in the open page, shaders included.

**Texture size.** Browsers often limit textures to 8192 pixels on a side. A larger image fails its asset with an error naming its size (see [Texture](asset-kinds.md#texture)); keep sheets within 8192.

### Packed assets over HTTP

An [asset library](asset-library.md) in a [packed build](asset-library.md#packs) is the way to serve a larger game: the page downloads the catalog, then one zip per group as the game loads it, and the hashed pack names cache forever. The setup:

```bash
web/build.sh -packed ./mygame build/web/mygame-packed
```

`web/build.sh -packed <pkgdir> <outdir>` builds with the `webassets` tag and runs `go run . -pack <outdir>/assets` in `<pkgdir>`. The game decides what the tag means: a packed build embeds nothing and opens the library on `httpfs` at `assets/` next to the page. `smoke.cjs` checks a build whose name ends in `-packed` too and fails if it fetches an asset outside the catalog and the packs.

## What is different in a browser

| Topic | Browser behaviour |
| --- | --- |
| Threads | One. `GOMAXPROCS` is 1, decoding runs between frames, and a large PNG, map or WAV stalls the frame it decodes in. Loading costs frames, but the pipeline stays correct. |
| Audio | Silent until the first click or key (the shell's start overlay). Music started at boot begins then. |
| Keys | The browser keeps some keys (F5, F11, F12, Ctrl+W, sometimes F1) before the game sees them. Bind important actions elsewhere too. The [screenshot hotkey](engine.md#hotkey-and-environment-variables) is F9 in a browser build for this reason (`game.DevScreenshotKey`). |
| Gamepads | Appear only after a button press on them (Gamepad API rule). |
| Lua `TickBudget` | Works: the deadline is polled by the VM, no timer goroutine is needed. |
| Files | No `os` file access: `SetWindowSize`/`SetWindowTitle` do nothing, the frame log reports an error and stays off. Store settings and saves (for example `input.Profile.Marshal` output) in `localStorage` through `syscall/js`. |
| Screenshots | [`Engine.Screenshot`](engine.md#screenshots) offers the PNG as a browser download instead of writing a file; the encode runs on the one thread. |
| Exit | `ebiten.Termination` or a `log.Fatal` stops the game and leaves the last frame on the canvas; the shell shows "The game has ended." or the error. |
