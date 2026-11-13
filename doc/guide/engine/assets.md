# Assets in the engine

The engine loads every file through the asset system of module [`github.com/weakpixel/asset`](https://github.com/weakpixel/asset): handles, keys, stores, scopes, states, events, dependencies, sub-assets, hot reload, keep-alive and writing a loader are described in its [guide](https://github.com/weakpixel/asset/blob/main/doc/guide.md). This page covers what the engine adds: the server it creates, the handles components hold, and how hot reload and HTTP reach a running game.

## The engine's server

`game.New` creates one `asset.Server` from `game.Config` (`FS`, `Workers`, `CommitBudget`, `KeepAlive`, `WatchInterval`, `OnAssetError`; see [engine.md](engine.md#config)) and registers the loaders of every engine kind. `e.Assets()` returns it, and `extract.AssetSystem` calls `Update` as the first stage of every tick, so assets change only between ticks. The tick's `*asset.Events` is also the World resource `*asset.Events`.

```go
level := e.Assets().NewScope("level1")
hero := render.LoadSprite(level, "chars/player.png") // gfx.SpriteHandle, returns at once

sp := e.Stores().Sprites().Get(hero) // placeholder until Ready, never nil
```

Without `game.Engine` (a tool, a test), create the server yourself and register the kinds you need, for example `render.RegisterLoaders(srv, render.LoaderOptions{})`.

## Organizing assets

Any layout works with keys. For a standard one, with packages that load and mount together (`e.LoadLevelBase("forest/cave", ...)`), a checker and pack files for release and the web, see the [asset library](asset-library.md).

## Game-facing handles

Every asset kind that game code loads has one concrete handle type and concrete helpers in the package that owns the asset ([the pattern](https://github.com/weakpixel/asset/blob/main/doc/guide.md#concrete-handles)). The handles that components hold live in `gfx`; the others live next to their asset type. [asset-kinds.md](asset-kinds.md) explains what each kind is for and which files it reads.

| Kind | Handle | Load | Store |
| --- | --- | --- | --- |
| sprite | `gfx.SpriteHandle` | `render.LoadSprite`, `LoadSpriteSync`, `HoldSprite` | `render.SpriteStore` (`SpriteStoreOf`, `StoresOf(srv).Sprites`) |
| font | `gfx.FontHandle` | `render.LoadFont`, `LoadFontSync`, `HoldFont` | `render.FontStore` (`FontStoreOf`, `StoresOf(srv).Fonts`) |
| shader | `gfx.ShaderHandle` | `render.LoadShader`, `LoadShaderSync`, `HoldShader` | `render.ShaderStore` (`ShaderStoreOf`, `StoresOf(srv).Shaders`) |
| clip | `gfx.ClipHandle` | `anim.LoadClip`, `LoadClipSync`, `HoldClip` | `anim.ClipStore` (`ClipStoreOf`) |
| atlas | `render.AtlasHandle` | `render.LoadAtlas`, `LoadAtlasSync`, `HoldAtlas` | `render.AtlasStore` (`AtlasStoreOf`) |
| clip set | `anim.ClipSetHandle` | `anim.LoadClipSet`, `LoadClipSetSync`, `HoldClipSet` | `anim.ClipSetStore` (`ClipSetStoreOf`) |

The stores have `Get`, `Lookup`, `Ready`, `Err` and `Key`, as `asset.Store[T]` does, plus `Generic()` for the underlying `*asset.Store[T]`. Each call inlines to the generic one: `Get` is 0 allocs and costs the same (`BenchmarkSpriteGet/facade` vs `/generic`).

With an `*game.Engine` at hand, `e.Stores()` returns every store of the engine's server by method, so `e.Stores().ClipSets().Get(h)` replaces `anim.ClipSetStoreOf(e.Assets()).Get(h)`. The methods are `Sprites`, `Fonts`, `Atlases`, `Shaders`, `Clips`, `ClipSets`, `Tilesets`, `Maps`, `Worlds`, `Sounds`, `Music`, `Bindings` and `Prefabs`. Each resolves its store when called (one map lookup) and panics like `XStoreOf` if that kind's loaders are not registered, so cache the store in hot paths. `game.StoresOf(srv)` is the same for code that only has a server; loaders and kind packages keep using `XStoreOf(srv)`. A custom kind uses `asset.StoreOf[T](e.Assets())`.

`render.StoresOf(srv)` bundles the stores the renderer reads (`Sprites`, `Fonts`, `Shaders`); like every `...StoreOf` it panics if the loaders are not registered.

Why there is one concrete handle per kind instead of `Handle[T]` everywhere: [design/asset-system.md](../../design/asset-system.md#handles).

Engine-internal kinds (`Texture`, `FontSource`: only loaders and the renderer use them) and your own asset types use the generic API (`asset.Load[T]`, `asset.StoreOf[T]`) directly. Give a type of your own a facade the same way when components hold its handle.

## Keep-alive

`game.Config.KeepAlive` is the server's `Options.KeepAlive` ([how keep-alive behaves](https://github.com/weakpixel/asset/blob/main/doc/guide.md#keep-alive)): going level A to B and back to A then reloads nothing. The engine's loaders report their sizes: `Texture` its GPU bytes, `Sound` its PCM, `Music` its file; other kinds count the file length. Art that a prefab holds through its own scope counts as in use until the prefab itself is evicted.

## Hot reload

Edit an asset file while the game runs: the asset changes on a following tick, under the same handle ([how hot reload behaves](https://github.com/weakpixel/asset/blob/main/doc/guide.md#hot-reload)).

- **Polling:** `game.Config{WatchInterval: 500 * time.Millisecond}` in development builds; 0 (off) in release.
- **Pushed changes:** `e.Assets().Changed("player.png")`, or set `Config.AssetChanges`, a `<-chan []string` of keys: at the start of its assets stage the engine receives every pending batch without blocking and passes it to `Changed`, so a reload starts in the tick the change arrived in. nil is off; the sender owns the channel. In VSCode the extension pushes the files you save. For a browser build, `httpfs.WatchChanges(url)` returns such a channel from the [dev server](../tools.md#serve)'s event stream (`/_weakpixel/changes`).
- Systems that derived data from an asset rebuild it on `EventModified` or when `store.Version(h)` changes. `inputsys.InputSystem.Watch` does this for bindings files; shaders, clips, tilesets and sounds follow on their own ([shaders](shaders.md), [animation](animation.md), [audio](audio.md)).

Hot reload changes values, never handles, and never touches the `World`.

## Reading assets over HTTP

A browser build that loads assets from its web server sets `Config.FS` to an [`httpfs`](https://github.com/weakpixel/asset/blob/main/doc/guide.md#reading-assets-over-http) file system (`github.com/weakpixel/asset/httpfs`):

```go
fsys, err := httpfs.New("https://example.com/game/assets/", nil) // nil: http.DefaultClient
cfg.FS = fsys
```

Keep `Workers` at its default (at least 1) and `WatchInterval` off; in development `weakpixel serve` pushes changes (`httpfs.WatchChanges` into `Config.AssetChanges`) and sends `Cache-Control: no-cache`. The editor build reads the workspace the same way. Web builds as a whole: [web.md](web.md).
