# Asset library (`asset/library`)

An opt-in standard layout for a game's assets. Every top folder of the asset root is a **package**: a collection of assets that loads, mounts and packs together. The engine loads a level and mounts the packages it needs with one call, `weakpixel assets` checks references between packages, and each package ships as one pack file. You don't have to use it: any layout works with plain keys, as in [assets.md](assets.md).

## The layout

```
assets/                          the library root (assets.root in .weakpixel.json)
  library.json                   optional: entries, fences
  boot/                          optional, tiny: mounted first (loading screen, logo, one font)
  global/                        required: loaded at boot into Global(), never released
    fonts/  ui/  sfx/  music/  shaders/  input/  prefabs/  fx/
  forest/                        a package: any name of [a-z0-9._-]
    cave.level.json  cave.tmj    the level forest/cave
    clearing.level.json  clearing.tmj
    prefabs/  music/  ...
  temple/                        a package other packages use (tilesets)
```

| Folder | Loaded by | Lifetime |
| --- | --- | --- |
| `boot/` | `game.New`, blocking, first | global scope |
| `global/` | `game.New` into `Global()` ([Engine](#engine)) | global scope |
| any other top folder | whatever loads one of its files: a level, `LoadPackage`, a key | the scope that loads it |
| `packs/`, folders starting with `.` or `_` | nothing: reserved | |
| files at the root | plain keys | |

A package's inner folders are yours. The ones above are a recommendation and only `music/` changes how a file loads (music instead of sound). There is no nesting: a package is always the first folder below the root, and deeper folders are ordinary folders inside it.

**Packages use each other.** A file of `forest/` that references `temple/tiles.tsj` makes `temple` a package `forest` uses. Loading anything in `forest` mounts `temple`'s pack too, into the same scope, so switching between two levels that both use `temple` the documented way (load the next level, wait for `Done`, unload the previous one) never reloads it.

**`global/` is required** for a game that uses the library. The asset root is the folder that holds it. **`boot/` is optional**: with it, `game.New` mounts `boot/` first and loads `global/` in the background, so a startup screen drawn with `boot/`'s assets can show progress while `global/` downloads. Anything `New` or a system's `Init` reads synchronously (input bindings, a font loaded with `LoadSync`) then belongs in `boot/`. Keep a title menu in its own package: its art and music are large and can be released after the menu.

**Keys never change.** A key is the path from the library root (`"forest/tiles/trees.tsj"`), whether the file is loose on disk, embedded, inside its package's pack, or served over HTTP. Packing is a build step, not a code change.

## File names

A package loads without a list of files because the file name says the kind. Loaders keep accepting any `*.json` as before; the suffixes only matter to the library.

| Suffix | Kind | Loaded as |
| --- | --- | --- |
| `*.png`, `*.jpg`, `*.jpeg` | texture | `render.Texture` (usually a dependency of an atlas or map) |
| `*.sheet.json` | atlas | grid sheet |
| `*.atlas.json` | atlas | TexturePacker JSON |
| `*.aseprite.json`, `*.anim.json` | clip set | Aseprite export, animation definition |
| `*.tsj`, `*.tmj`, `*.world` | tileset, map, world | Tiled |
| `*.ttf`, `*.otf` | font | only as `file.ttf#size`, listed in `library.json` |
| `*.kage` | shader | |
| `*.wav`, `*.ogg`, `*.mp3` | music inside a `music/` folder, sound elsewhere | |
| `*.bindings.json` | bindings | input bindings |
| `*.level.json` | level | GOD document; the engine spawns levels itself |
| `*.fx.json` | effect | [particle effect](particles.md#the-effect-file) (`particles`) |
| `*.prefab.json` | prefab | [prefab](game-objects.md#prefabs) asset (`prefab`) |
| `*.voice.json`, `*.lang.json`, `*.dialogue.json` | voice, language, dialogue | `speech.Voice`, `speech.Language`, `speech.Dialogue` (`speech`; a voice names its language and samples, a dialogue its voice files, relative to itself) |

Use lower-case names of `[a-z0-9._-]`. Embedded files, zips and HTTP are case-sensitive while macOS and Windows are not, so `Hero.png` referenced as `player.png` works on your machine and fails in the browser. `weakpixel assets check` warns about both.

## What a package loads

A package's **entries** are its files of a loadable kind that no other file of the package references, plus the entries `library.json` adds. A map is an entry, its tilesets and images are not: they load as its dependencies. A clip set's `*.anim.json` is an entry; the Aseprite export and image behind it are not. A prefab nothing in its package references is an entry, so code can spawn it once the package is loaded. Prefab keys in GOD documents count as references, so a level that places another package's prefab uses that package.

Entries load only when you ask for them with `LoadPackage` (and for `boot/` and `global/` at boot). Loading a level loads its parts and what they reference, never the rest of its package.

```json
{
  "entries": { "global": ["global/fonts/ui.ttf#16", "global/fonts/ui.ttf#24"] },
  "packages": { "forest": { "uses": ["temple"] } }
}
```

`library.json` has two fields:

| Field | |
| --- | --- |
| `entries` | Keys added to a package's entries, by package name: font sizes, and files that something references but that should also load on their own. |
| `packages.<name>.uses` | A fence: the packages `<name>` may use besides `global` and `boot`. A reference to any other package is an error in `weakpixel assets check`. Without it a package may use any package. |

## Engine

```go
lib, err := library.Open(os.DirFS("assets")) // or embed.FS, or a packed build
if err != nil {
	log.Fatal(err)
}
e := game.New(game.Config{Library: lib}) // not FS: the engine reads lib.FS()

e.Start(func(e *game.Engine) {
	cave, _ = e.LoadLevelBase("forest/cave", tilemap.SpawnOptions{}) // mounts forest/ and temple/
	golem := prefab.Load(cave.Scope(), "bosses/golem/golem.prefab.json") // mounts bosses/ too
	e.LoadPackage(cave.Scope(), "forest") // explicit preload of forest/'s entries
	_ = golem
})
// your own boot progress (the engine's startup screen already waits for it,
// see engine.md#startup-screen):
ready, total := e.Boot().Progress()
```

| Call | |
| --- | --- |
| `game.Config.Library` | The library. `Config.FS` must stay nil (`game: set Config.FS or Config.Library, not both`). |
| `e.Boot()` | The load of `global/` that `New` started in the global scope: `Progress`, `Done`, `Err`. nil without a `global/` folder. In a packed build without `boot/`, `New` mounts the `global/` pack before it returns (blocking, like `LoadSync`), so `Config.Input.Bindings`, `Start` hooks and `Init` of systems can load from `global/` at once. With `boot/`, `New` mounts `boot/` that way instead and `global/` mounts in the background. A pack that fails goes to `OnAssetError`. |
| `e.LoadLevelBase(base, opts)` | Loads the level named by its base key (`"forest/cave"`) into a new scope: `<base>.level.json` like `LoadLevel`, `<base>.tmj` or `<base>.world` like `LoadMap`, which also spawns the documents paired with the map (`<base>.level.json` is the pair of `<base>.tmj` and spawns once; a world's children bring their own `<map>.level.json`). Every part is optional, but one must exist. `SpawnOptions.NoDocuments` and `SkipDocuments` leave documents out. Works without a library too. |
| `e.LoadLevel(key)`, `e.LoadMap(key, opts)` | As in [assets.md](assets.md); with a library they mount the key's package (and the packages it uses) first. |
| `e.LoadPackage(sc, name)` | Loads every entry of a package into `sc` and returns a `*library.Load`. The explicit preload, for prefabs, effects or sounds that code spawns or plays by key. Load into `Level.Scope()` to release them with the level. Panics with `game: LoadPackage: Config.Library is nil`. |
| `e.UnloadLevel(l)` | Deletes the level's entities, releases its scope and unmounts the packs only it held. |
| `e.Library()` | The library, for `LoadPackage`, `Mount`, `Mounted` and the catalog. |

With loose files, `LoadLevelBase` and `LoadLevel` spawn the GOD objects before they return. From a pack they spawn in the assets stage of the tick in which the pack is mounted, before your systems run. `Level.Spawned()` is true once every part has spawned (the map's layers included), `Level.Progress()` counts the packs first and then the scope, and `Level.Err()` includes a failed pack or a GOD document that did not spawn (a paired document that fails never blocks the map; both go to `OnAssetError`).

Errors read `game: LoadLevelBase "<base>": ...`: `the library has no package <name>`, `no <base>.level.json, <base>.tmj or <base>.world`, or the GOD document's error. From a pack they go to `Level.Err` instead.

## Without the engine

```go
lib, _ := library.Open(fsys)
srv := asset.NewServer(asset.Options{FS: lib.FS()})
// register your loaders, then:
lib.Attach(srv)
lib.SetLoader(library.KindAtlas, func(sc *asset.Scope, key string) { render.LoadAtlas(sc, key) })

load := lib.LoadPackage(srv.NewScope("forest"), "forest")
// every tick:
srv.Update()
lib.Update() // starts the entries once the packs are mounted
```

| API | |
| --- | --- |
| `library.Open(fs)` | Reads `assets.catalog.json` if the FS has one, else lists the FS. An FS that cannot list directories (HTTP) and has no catalog fails: `library: the file system cannot list directories and has no assets.catalog.json`. |
| `lib.FS()` | The FS to give the asset server: the base FS plus mounted packs and overlays. Safe for concurrent reads. |
| `lib.Attach(srv)` | Registers the `library.Pack` asset type. Once (`library: Attach called twice`). |
| `lib.SetLoader(kind, fn)` | How entries of a kind load. Kinds without a loader are skipped. `game.New` sets all built-in kinds except the speech kinds, which `speech.SetLibraryLoaders(lib)` sets. |
| `lib.LoadPackage(sc, name)` | Mounts the packs of the package and of the packages it uses, then loads the entries into `sc`. Returns a `*library.Load` with `Progress`, `Done`, `Err`, `Mounted`, `Package`. Panics with `library: unknown package "<name>"` and `library: LoadPackage before Attach`. |
| `lib.HoldPackage(sc, name)` | The same without the entries: makes the package's files readable for loads that name them (the engine calls it before a level spawns). Panics like `LoadPackage`. |
| `lib.Update()` | Call after each `Server.Update`. |
| `lib.MountPackage(sc, name)` | Loads the packs of a package (and of the packages it uses) into `sc` and blocks until they are mounted; returns their joined errors. Nothing to do for loose files. For boot code only, like `LoadSync`. |
| `lib.Mounted()` | The names of the packages whose packs are mounted now, sorted. Always empty for loose files. |
| `lib.Mount(prefix, fs)`, `lib.Unmount(prefix, fs)` | Overlays: keys under `prefix` (`""` for everything) are read from `fs` first. The newest overlay wins, missing files fall through, and overlays always stay above packs, even packs mounted later, so a patch is a mount. |
| `lib.Catalog()`, `lib.Package(name)` | Every package with its files, entries, the packages it uses and its pack (file and size in a packed build), natively and in a browser, before anything is downloaded. |
| `library.PackageOf(path)`, `library.KindOf(path)` | The convention as functions. |

## Packs

`weakpixel assets pack assets -o dist/assets` writes a packed build:

```
dist/assets/
  assets.catalog.json              packages, entries, packs
  packs/global.3f2a1c4d5e6f.zip    one zip per package, named by content hash
  packs/forest.9b0e....zip
  ...                              files at the root, copied as they are
```

Open it like any library: `library.Open(os.DirFS("dist/assets"))`, an `embed.FS` of it, or [`httpfs`](assets.md#reading-assets-over-http) in a browser build ([web: packed assets](web.md#packed-assets-over-http)). Loading a package (or a level or key in it) mounts its pack at its folder for as long as the scope holds it; the pack is an ordinary asset (`library.Pack`), so its download shows in the progress, a broken pack fails the load, and releasing the scope unmounts it. Over HTTP a pack is one request per package, downloaded whole the first time something needs it, and the hash in its name lets the browser cache it forever. A large package with many levels downloads all of them on first entry, so split areas that load separately into their own packages.

A key inside a packed package whose pack is not mounted fails instead of reading a stale loose copy: `library: package forest is packed and not loaded (load it with LoadPackage, or a level of it)`. `LoadLevel`, `LoadMap` and `LoadLevelBase` mount the package themselves; for other loads, hold the package first (`LoadPackage` or `HoldPackage` into your scope). `boot/` and `global/` are mounted at boot.

Packs never hot-reload (zip entries have a fixed time). Develop with loose files and `WatchInterval` or `Assets().Changed`; pack for release and the web.

## Generated keys (optional)

`weakpixel assets keys` writes Go constants for a library, so code names assets by identifier and a renamed file breaks the build instead of drawing a placeholder. Nothing requires it: plain key strings keep working, and a game opts in with one `go:generate` line.

```go
//go:generate go run github.com/weakpixel/engine/tools/weakpixel assets keys assets --pkg main -o zz__gen.assets.go
```

| Constant | For | Example |
| --- | --- | --- |
| `Package<Name>` | each package | `PackageWorld1 = "world-1"` |
| `Key<Path>` | each file of a known kind; the path in CamelCase, a `.json` extension dropped, others kept | `KeyGlobalPlayerPlayerAnim = "global/player/player.anim.json"`, `KeyTempleTemplePng` |
| `Key<Path><Tag>` | each tag of an `*.anim.json`, and of an `*.aseprite.json` that is a package entry | `KeyGlobalPlayerPlayerAnimIdle = "global/player/player.anim.json#Idle"` |

Two files that make the same identifier (`a-b.png`, `a_b.png`) are an error naming both. Files that only exist in code (a font added to the FS at run time) get no constant. The file is regenerated by `go generate`, so `make all` fails if it is stale.

For a browser build the library is served as packs: see [packed assets over HTTP](web.md#packed-assets-over-http).

## weakpixel assets

```bash
go run ./tools/weakpixel assets check   [--json] [root]       # findings, exit 1 on errors
go run ./tools/weakpixel assets ls      [root]                 # packages, sizes, entries, uses
go run ./tools/weakpixel assets catalog [root] [-o file|-]     # catalog for a loose library served over HTTP
go run ./tools/weakpixel assets pack    [root] [-o out]        # packed build; out (default build/assets) must be empty
go run ./tools/weakpixel assets keys    [root] [--pkg p] [-o f] # optional Go constants, see above
```

`check` reports, all at once:

| Finding | Severity |
| --- | --- |
| A reference to a file that does not exist | error |
| A reference whose case differs from the file's | error |
| A reference that leaves the library root | error |
| No `global/` package | error |
| `global/` using a package other than `boot/` (it would stay mounted for good) | error |
| `boot/` using any other package (nothing else is loaded yet when it is) | error |
| A reference to a package outside the fence `packages.<name>.uses` in `library.json` | error |
| An entry or fence in `library.json` for a missing file or package | error |
| Packages that use each other in a cycle | warning |
| A name outside `[a-z0-9._-]` | warning |
| A file of no known kind that nothing references (source files such as `*.aseprite` belong outside the library) | warning |
| A texture larger than 8192 pixels, which fails in most browsers | warning |

`check --json` prints one JSON object instead, for editors: `{"findings": [{"severity": "error|warn", "path": "forest/cave.level.json", "msg": "..."}], "errors": 1, "warnings": 0}` (`findings` is `[]` when clean). A library that cannot be read at all, for example an invalid `library.json`, prints `{"fatal": "..."}` and exits 1. The exit codes are the same as without the flag.

References are what the loaders read as dependencies (Tiled `source`, `image`, `fileName`; sheet and export `image`; the definition's `sheet`), plus every string in a GOD document whose name has a known kind. `catalog`, `pack` and `keys` refuse a library with errors.

Write a catalog for development only when you serve loose files over HTTP; a catalog in a folder you edit goes stale, and `Open` prefers it over listing.

How it is built: [design/asset-system.md](../../design/asset-system.md#asset-library).
