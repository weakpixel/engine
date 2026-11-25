# Asset system design

How the engine uses the asset system of module [`github.com/weakpixel/asset`](https://github.com/weakpixel/asset), and why.
The asset system's own design (handles, stores, the two-phase pipeline, scopes, dependencies, hot reload, keep-alive) is in its [design page](https://github.com/weakpixel/asset/blob/main/doc/design.md).
The engine side of the API is in the guide: [assets in the engine](../guide/engine/assets.md) and [asset kinds](../guide/engine/asset-kinds.md).

## Requirements

- Components stay **pointer-free** (GC scanning, cache locality; [performance guide rule 5](https://github.com/weakpixel/ecs/blob/main/doc/performance-guide.md#performance-practices)): they hold asset handles, never Ebiten objects.
- The `World` stays single-goroutine, and Ebiten objects are created on the game goroutine: in a loader's `Finish`, inside `extract.AssetSystem`, the first stage of every tick.
- Authored data (levels, prefabs) names assets by path; runtime data uses handles.

## Handles

**One concrete handle type per game-facing kind.**
Game code uses `gfx.SpriteHandle`, `gfx.FontHandle`, `gfx.ClipHandle` (each `struct{ asset.Ref }`) plus `render.AtlasHandle` and `anim.ClipSetHandle`, with typed store facades (`render.SpriteStore`, ...), instead of `asset.Handle[T]`.
The reason: `gfx` holds the components but cannot import `render` or `anim` (they import `gfx`), so with only `Handle[T]` every kind needed two handle types, one over a marker type in `gfx` and one over the real asset type, with conversions at every call site, and those conversions compiled between *any* two kinds.
Now there is one type per kind, and the only conversions (`Handle.Ref`, `asset.HandleOf[T]`) live inside the facades.
The facades inline to the generic calls: `Get` costs the same and stays 0 allocs (`BenchmarkSpriteGet/facade` vs `/generic`).

## Authored data

Levels and prefabs are GOD documents.
Component fields that hold handles are marked `+game:asset:kind=k` and appear as key strings in JSON ([GOD asset fields](../guide/ecs/game-object-definition.md#asset-fields)).
`god` defines the `AssetResolver` interface and calls it from `Deserialize` and `Serialize`; `resolver` implements it on an `asset.Server`, loading into a settable scope.
It knows no kind itself: the package that owns an asset type registers its kinds (`sprite`, `clip`, `font`, `shader`, `sound`, `music`, `prefab`, `effect`, `voice`, `dialogue`) in the leaf package `assetkind` from an `init` function.
The registry is its own package because `render` owns three kinds and must not import `ecs`, which `resolver` does.
The `prefab` handle is `gfx.PrefabHandle` (`prefab.Handle` is an alias, so `gfx` never imports `prefab`).
A prefab named by a field is held by the scope of the level or prefab that names it, not declared as a dependency, and reference cycles between prefabs through such fields are not detected: a prefab that names itself (or `A`→`B`→`A`) is never released by scope release.
A stale handle on save is an error, so a key that no longer means anything is never written.
Lua sees the same keys.

Prefabs are also assets (`prefab`): parsed and validated once on a decode worker, bound to the World in `Finish` and spawned many times without JSON ([guide](../guide/engine/game-objects.md#prefabs)).
A variant's base and every prefab nested in a [multi-object prefab](../guide/engine/game-objects.md#prefabs-with-several-objects) are `asset.Dep`s, so readiness, reloads and cycles come from the dependency graph.
The art a prefab references is **not** declared as dependencies: a variant's object type, and so its asset fields, is only known once its base is Ready, after `Decode`.
Instead `Finish` resolves the keys into a scope the prefab owns, which `Unload` releases; the art loads in the background with placeholders, like a level's. A changed base reaches its variants like any dependency: re-finished with `WatchInterval` on, read from their files again when the change came through `Changed` without it (the level editor's case).

Tiled object layers stay map data: nothing turns a Tiled object into a Game Object.
Game Objects are placed in GOD levels, which the level editor validates and edits with prefabs and overrides; a second, untyped source of objects in Tiled would need its own property mapping and a rule for which file wins ([tilemaps.md](../guide/engine/tilemaps.md#where-to-place-objects)).

## Hot reload

Asset hot reload follows one mental model: development only, opt-in, atomic per asset, the old value keeps serving until the new one is complete ([how the asset system does it](https://github.com/weakpixel/asset/blob/main/doc/design.md#hot-reload)).
What the engine adds:

- **Sources of pushed changes.**
  The VSCode extension's file watcher sends `assetsChanged`; in the browser the dev server (`tools/internal/devserver`, `wpix serve`) polls the asset tree on the server, where a stat is cheap, and streams batches of keys as server-sent events, which the page reads with `httpfs.WatchChanges`.
  `game.Config.AssetChanges` hands every pending batch to `Server.Changed` at the start of the assets stage, so a reload starts in the tick the change arrived in.
- **Why it's safe mid-game:** swaps happen inside `Update`, which `AssetSystem` runs before every other system; the render queue stores handles, not images; `Draw` resolves handles after the swap.
- **Derived data** follows `EventModified` or `Store.Version`: the input system's bindings, shader recompiles, re-cut atlases, tilesets and clip sets.

## Asset library

The opt-in folder convention, package loading and packs ([guide](../guide/engine/asset-library.md)).
The code is in `asset/library`, stdlib plus `asset`.

- **Folders are packages; code owns lifetime.**
  Every top folder of the root is a package: the unit of loading, mounting and packing.
  A package declares no lifetime; it loads into the `asset.Scope` of whoever asks (a level, `LoadPackage`, the global scope at boot).
  An earlier layout had scope-kind folders (`common/`, `levels/<id>/`, `screens/<id>/`, `shared/<id>/`); the kinds turned out to be checker rules only (`shared/` had no scope and `levels` and `screens` ran the same code), and they forced every structure that was not "a level" into `shared/`.
  `asset` knows nothing about the library, so a game that does not use the convention pays nothing.
- **Package = first path segment.**
  `PackageOf` is a string cut, so the checker, the packer, the editor and the MCP server agree without a lookup.
  No nesting: deeper folders belong to their package; nesting, if ever needed, would come with per-package manifests and needs no key change.
  `packs/` and folders starting with `.` or `_` are reserved.
- **Dependencies are inferred.**
  The scan records which packages a package's files reference (`uses`); loading a package also mounts their packs into the same scope, and ref counting keeps shared assets alive across a documented level switch.
  Only references that break are errors: `global/` using a package other than `boot/` (it would stay mounted for good), `boot/` using any package (nothing else is mounted when it is), and references outside an optional fence (`library.json` `packages.<name>.uses`).
  Cycles mount fine and are only warned about.
- **`global/` is required, `boot/` optional.**
  `global/` loads into the global scope at boot and marks the library root for the tools.
  `boot/` is mounted first so a startup screen can draw while `global/` downloads.
- **Levels by base key.**
  `LoadLevelBase("forest/cave")` spawns `cave.level.json` and `cave.tmj` or `cave.world`: the old group rule keyed by base path, so one package can hold any number of levels.
  It loads only what the level references; preloading a package's entries is always an explicit `LoadPackage`, because the entries of a package with several levels include every other level's map.
- **Auto-mount.**
  `LoadLevel`, `LoadMap` and `LoadLevelBase` call `HoldPackage` on the key's package (packs only, no entries) and start once the packs are mounted, at once for loose files.
- **Kinds by file name, loaders by registration.**
  The library maps names to kinds (`KindOf`) but cannot import the Ebiten packages that own the types, so the engine maps kinds to load functions (`SetLoader`), like `asset` and `render`.
- **Entries from references.**
  One generic JSON walk finds what files name: the keys the loaders read as dependencies (`source`, `image`, `fileName`, `sheet`, relative to the file), and in GOD documents every string with a known kind (root-relative keys).
  The same walk powers the entries (loadable files nothing in the package references), the packages a package uses, and `wpix assets check`.
  Running the loaders' `Decode` instead would need the Ebiten packages in the tool.
- **Catalog when listing is impossible.**
  `Open` reads `assets.catalog.json` or lists the FS (`fs.WalkDir`).
  HTTP cannot list, and a packed build has its files inside zips, so both ship a catalog written by `wpix assets`.
  Its `version` stays 0 until a release.
- **Packs are zips mounted into a union FS with ordered layers.**
  `archive/zip.Reader` is an `fs.FS` with random access and per-file compression (stored for PNG and Ogg, deflate for JSON).
  The server's `Options.FS` is the library's mount table over the base FS, so the asset system is unchanged.
  Decode workers read it while the game goroutine mounts, so the table is copy-on-write behind an `atomic.Pointer`: reads never lock.
  The table has two layers: packs, and above them overlays (`Mount`).
  A newer mount wins within its layer and a missing file falls through; keeping overlays above packs means a pack mounted later never covers a patch, and leaves room for a mod layer.
  A key of a packed package without a mounted pack fails with `library: package <p> is packed and not loaded` (wrapping `fs.ErrNotExist`) rather than reading the base, which has no such file in a packed build.
- **A pack is an asset.**
  `library.Pack` decodes the zip directory on a worker and mounts it in `Finish`; `Unload` unmounts that exact reader.
  So downloading, progress, errors and lifetime come from the pipeline: the pack is held by the scope that loaded the package, and releasing the scope unmounts it.
  A reload mounts the new version before the old one's `Unload` removes the old.
- **Two phases per package load.**
  Entries can only be read once the packs are mounted, and `Finish` cannot load into a scope, so `LoadPackage` loads the packs and `Library.Update` (after `Server.Update`) loads the entries when every pack is Ready.
  With loose files the entries start at once.
  The engine starts a level's parts in the same assets stage.
- **The first package is mounted inside `game.New`.**
  A pack mounts in `Finish`, which runs in `Update`, so anything loaded from a packed package before the first `Update` (the bindings file, a font loaded with `LoadSync` in a system's `Init`) would read an unmounted folder and fail for good.
  `New` therefore mounts `boot/`, or `global/` without one, with `MountPackage` (the pipeline's `LoadSync`), before the input system loads its bindings; with `boot/`, `global/` mounts asynchronously and synchronous boot loads belong in `boot/`.
- **Generated keys are a separate, optional step.**
  `wpix assets keys` writes constants from the same scan; clip tags come from `animfile`.
  Constants rather than nested structs: they are compile-time, greppable, and cost nothing to games that ignore them.
- **`.assetignore` lives in the listing.**
  The matcher (`Ignore`, git's rules, checked against `git check-ignore` in a test) runs inside the one walk that `Scan`, `Open`, `WritePacks`, the tools and the embed generator share, so they cannot disagree about which files exist.
  Ignored folders are pruned while walking, which is exact because git cannot re-include below an ignored folder.
  The asset module never lists folders and knows nothing about it.
- **Generated embed patterns.**
  `go:embed` patterns are fixed at compile time, and filtering an `embed.FS` at run time would still ship the files, so `wpix assets embed` writes the patterns.
  It names a folder wherever go:embed's own walk of it gives exactly the library's files (new files there need no regeneration) and single files elsewhere; staging a filtered copy under `build/` was rejected because a fresh checkout would not compile before a build step.
- **Ignored means absent at run time too.**
  `Open` of a listed folder keeps the rules and the mount table refuses ignored keys of the base, so a game that loads an ignored file fails in development as it would packed or embedded.
  Overlays are not filtered (a mod is not part of the root), and a catalog is already the file list.
- **Deterministic packs.**
  Members carry a fixed time and sorted order, so the same files give the same bytes; the name carries the SHA-256 prefix for HTTP caching.

## Package placement

The asset system is its own module, standard library only, and imports neither `ecs` nor Ebiten, so it is testable with `fstest.MapFS` and fake loaders and tools never compile a renderer.
Asset types that need Ebiten (`Texture`, `Sprite`, `Atlas`, `FontSource`, `Font`, `Shader`) live in `render`; clips in `anim`; `Sound` and `Music` in `sound`; the `AssetSystem` that calls `Server.Update` as the first stage of the tick lives in `extract`.
The library (`asset/library`) stays in the engine because its folder convention, packages and packs are engine policy.

## Not built yet

- **Asset library beyond v1.**
  Packs are downloaded whole (no byte-range reads), localized variants (`title.de.png`) are not built, and there is no mod layer yet: mods that add packages to the catalog, a load order and a report of the keys each mod overrides.
  Overlays (`Mount`) cover patches today.
- **Watching beyond files.**
  Nothing reloads Go code or GOD schemas.
  The asset system's own limits are in [its design page](https://github.com/weakpixel/asset/blob/main/doc/design.md#not-built-yet).
- **Audio beyond v1.**
  The `sound` package plays effects, positional emitters and crossfading music on buses ([audio.md](../guide/engine/audio.md)).
  Not built: music loop points and intros, pitch, ducking, pausing on focus loss.
- **Tilemaps beyond v1.**
  Tiled tilesets, maps and worlds load ([tilemaps.md](../guide/engine/tilemaps.md)).
  Not built: embedded single-image tilesets (no key of their own) are rejected, because the asset system gives a child type exactly one parent type (multi-parent children are not built there), so only `render.Atlas` can publish `render.Sprite` children and a `tilemap.Tileset` borrows them from the atlas of the same `*.tsj` key.
- **Per-entity asset lifetime.**
  By design every resolved asset belongs to one scope; deleting an entity releases nothing.
