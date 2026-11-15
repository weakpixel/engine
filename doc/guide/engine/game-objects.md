# Game Objects and levels (`gfx` markers + `resolver`)

`Transform`, `Sprite`, `Animator` and `Label` are Game Object components, so levels and [prefabs](#prefabs) can be authored as GOD (JSON) and loaded with `types.Deserialize`. The general format is in [Game Object Definition](../ecs/game-object-definition.md); this page covers what the engine adds.

## Objects

| Object (group `gfx`) | Components |
| --- | --- |
| `Visual` | `Transform`, `Sprite` |
| `AnimatedVisual` | embeds `Visual` + `Animator` |
| `Text` | `Transform`, `Label` |
| `Panel` | `Transform`, `NineSlice` |

Embed `Visual` in your own object to get a positioned, drawn thing:

```go
// +game:object
type Enemy struct {
	// +game:embed
	gfx.AnimatedVisual
	// +game:component
	Health *Health
}
```

`ScreenSprite`, `ScreenNineSlice` and `DebugShape` are runtime-only (not Game Object components).

## Asset and runtime fields

Components hold handles (`json:"-"`) and runtime state that has no place in a data file. Two markers on **direct fields of a component struct** describe them (full rules: [GOD asset fields](../ecs/game-object-definition.md#asset-fields)):

| Marker | Field holds | In JSON / schema |
| --- | --- | --- |
| `+game:asset:kind=<k>` | an asset handle | a key string: `"format": "asset"`, `"x-asset": "<k>"` |
| `+game:runtime` | runtime state | in the schema with `"x-runtime": true`; **never serialized**, rejected in input |

```go
type Sprite struct {
	// +game:asset:kind=sprite
	ID SpriteHandle `json:"-"`
	...
}

type Animator struct {
	// +game:asset:kind=clip
	Clip ClipHandle `json:"-"`
	// +game:runtime
	Time time.Duration `json:"-"`
	...
}
```

Both fields need `json:"-"`, cannot be combined, and `asset` needs a `kind`. Nothing else in generated Go changes.

## Asset kinds

The engine has eight kinds: `sprite` (`Sprite.ID`), `clip` (`Animator.Clip`), `font` (`Label.Font`), `shader` (`Sprite.Shader`), `sound` (`SoundEmitter.Sound`), `effect` (`Particles.Effect`) and `music` and `prefab` (no `gfx` field; for your own components). Package `speech` adds `voice` (`speech.Speaker.Voice`, a `gfx.VoiceHandle`; a `*.voice.json` key or a starter name such as `"chirpy"`) and `dialogue` (a `gfx.DialogueHandle`). Handles, Go types and key examples: [kinds.md](kinds.md#asset-kinds).

The resolver itself knows no kind. The package that owns an asset type registers its kinds in `assetkind` from an `init` function, so a kind exists in every program that links its owner. Add your own the same way: [Custom kinds](#custom-kinds).

## Wiring

```go
srv := asset.NewServer(asset.Options{FS: levelFS, Workers: 2})
render.RegisterLoaders(srv, render.LoaderOptions{})
anim.RegisterLoaders(srv)

res := resolver.New(srv)
resolver.Install(world, res) // types.SetAssetResolver

level := srv.NewScope("level1")
res.SetScope(level) // what Deserialize loads is held by this scope; nil = global

objs, err := types.Deserialize(world, f) // keys -> handles
```

Level data:

```json
{"objects": [{"group": "gfx", "class": "AnimatedVisual", "components": {
  "gfx:Transform": {"Pos": {"X": 1, "Y": 2}, "PrevPos": {"X": 1, "Y": 2}, "Scale": {"X": 1, "Y": 1}},
  "gfx:Sprite":    {"ID": "chars/player.aseprite.json#idle_0", "Layer": 2},
  "gfx:Animator":  {"Clip": "chars/player.aseprite.json#idle", "Speed": 1}
}}]}
```

Leaving out an asset key (or `""`) keeps the zero handle (placeholder). Do not include `Time`, `Frame` or `Done`: `Deserialize` rejects them with `component gfx:Animator: $.Time: runtime field, not authored data`.

`Deserialize` does not block: assets load in the background and draw the placeholder until ready. To wait: `for !level.Done() { srv.Update() }`, or show `level.Progress()`.

Write back:

```go
err := types.Serialize(world, w, objs...) // handles -> keys; runtime fields omitted
```

A stale handle (its asset was unloaded) is a serialize error, so a key that no longer means anything is never written.

## Prefabs

A prefab (`*.prefab.json`) is a pre-defined Game Object: one object (or a root with attached [parts](#prefabs-with-several-objects)) with its component values and asset keys filled in, placed in levels and spawned from code many times. The format, the override rules and variants are in [Game Object Definition](../ecs/game-object-definition.md#prefabs). Prefabs can live in any folder; with an [asset library](asset-library.md) the package decides how long they stay loaded (`global/prefabs/` for the whole game, `<package>/prefabs/` while something of that package is loaded).

```json
{"objects": [{"group": "gfx", "class": "AnimatedVisual", "components": {
  "gfx:Transform": {"Pos": {"X": 0, "Y": 0}, "PrevPos": {"X": 0, "Y": 0}, "Rot": 0, "Scale": {"X": 1, "Y": 1}},
  "gfx:Sprite":    {"Layer": 4, "Origin": {"X": 0, "Y": 0}, "FlipX": false, "FlipY": false, "Tint": 4294967295, "Blend": 0, "Hidden": false},
  "gfx:Animator":  {"Clip": "global/player/player.anim.json#Idle", "Speed": 1}
}}]}
```

A prefab file may carry an optional top-level `"name"`, a label for editors (`Prefab.Name()`); the VS Code extension lists prefabs by it, with the file name as fallback.

**In a level**, an entry names the prefab and overrides what differs:

```json
{"id": "villager-left",  "prefab": "global/prefabs/villager.prefab.json",
 "components": {"gfx:Transform": {"Pos": {"X": 440, "Y": 250}, "PrevPos": {"X": 440, "Y": 250}}}}
```

`LoadLevel`, `LoadLevelData` and `LoadLevelBase` load the document's prefabs into the level's scope first and spawn the document in the tick in which they are all Ready (`Level.Spawned()`); a document without prefab instances still spawns at once. A prefab that fails, or an override that does not validate, goes to `Level.Err()` and `Config.OnAssetError`, and nothing spawns. The [level editor](editor.md#prefab-instances) edits documents with instances, and opens `*.prefab.json` files themselves.

**From code**, package `prefab` makes prefabs assets. `game.Engine` registers the loader and installs the `types.PrefabSource` levels use.

```go
goblin := prefab.Load(level.Scope(), "global/prefabs/goblin.prefab.json")
prefabs := e.Stores().Prefabs()

// later, in a system:
if prefabs.Ready(goblin) {
	at := gfx.Vec2{X: 100, Y: 40}
	e := prefabs.Spawn(goblin, gfx.Transform{Pos: at, PrevPos: at, Scale: gfx.Vec2{X: 1, Y: 1}})
	_ = e
}
```

| API | Does |
| --- | --- |
| `prefab.Load(sc, key)` / `LoadSync` / `Hold` | Handle of a prefab asset, held by `sc`, like other assets. |
| `prefab.StoreOf(srv)` | `Get`, `Lookup`, `Ready`, `Err`, `Key` of prefab handles. |
| `Store.Spawn(h, overrides...)` | One entity from the prefab (the root, for a prefab with several objects), overrides replacing (or adding) components of the root by type. Panics with `prefab: Spawn: <key> is not ready (<state>)` before it is Ready. |
| `Store.SpawnAll(h, dst, overrides...)` | Like `Spawn`, appending every spawned entity to `dst`, root first, in `Types().Parts()` order. Same panics. |
| `(*prefab.Prefab).Types()` | The parsed `types.Prefab` (key, object type, component JSON, base). |
| `prefab.Source(srv, scope)` | The `types.PrefabSource` the engine installs: loads a key into `scope()` and returns it when Ready. |
| `prefab.RegisterLoader(srv, opts)` | For an `asset.Server` without `game.Engine`: `Options.World`, and `Options.Assets` returning the `AssetResolver` for a prefab's own scope. |

A prefab is parsed and validated on a decode worker, then bound to the World once: spawning reads no JSON (see `BenchmarkPrefabSpawn`). The base of a variant and every prefab a prefab nests are dependencies, so such a prefab is Ready after them, reloads when one of them changes, and a cycle fails the prefab that closes it. The art a prefab references loads into a scope the prefab owns: it draws placeholders until loaded and is released with the prefab, when the last scope holding the prefab is released. With `Config.WatchInterval` on, a changed prefab file reloads, its variants are rebuilt, and the next spawn uses the new values; entities already spawned keep theirs.

### Prefabs with several objects

A prefab file may hold a root plus parts, like a knight with a sword and a shadow; the format (`children` overrides, qualified ids such as `k1/sword`, nesting, variants) is in [Game Object Definition](../ecs/game-object-definition.md#multi-object-prefabs). The engine adds one rule: **every part reaches the root through `hierarchy:Parent`**, directly or through other parts. A part that does not fails the prefab with `prefab "<key>": object "<id>" is not attached to the root (hierarchy:Parent)`. Parts name their parent by the prefab's ids (`"Entity": "knight"`); the instance's ids replace them on expansion. A part with `PlaceMode: PlaceAbsolute` starts at its `Local` relative to where the instance is placed and then stays in world space ([absolute children](hierarchy.md#absolute-children)).

- **In a level**, one entry places the group. The parts are objects of the level (`Level.Objects()` lists them after their root, with `Object.Part` set), so another level object can attach to one with `"Entity": "k1/sword"`. The level's prefabs, nested ones included, load before the document spawns, and the hierarchy stage places the whole group in the spawn tick.
- **From code**, `Spawn` creates every part and returns the root; `SpawnAll` returns all entities. Spawning reads no JSON for the `Parent` links.
- **Deleting** the root deletes the parts: the hierarchy stage removes children of dead parents. Deleting a part deletes it and its own children. `UnloadLevel` skips entities that are gone already.
- **Limit:** a group spawned by a system in `simulate` or `post` is placed by the `hierarchy` stage in the same tick. Spawned later in the tick (anim, extract), the parts draw one frame at their authored `Transform`.


## Lifetime

Releasing the scope set with `SetScope` frees the art that level loaded. Deleting an entity releases nothing.

## Errors

`ResolveAsset` returns errors, never panics: malformed path, unknown kind, or a kind whose loader is not registered. `types.Deserialize` reports them together, prefixed `$.ID: resolve sprite asset "x": ...`. Without `resolver.Install`, a non-empty key gives `no AssetResolver set`.

## Custom kinds

Register your own kind (for example `quest`) from an `init` function of the package that owns the asset type. `assetkind.Of` builds the kind from your typed `Load` helper and the `Key` method of your store:

```go
const AssetKindQuest = "quest"

func init() {
	assetkind.Register(AssetKindQuest, assetkind.Of(LoadQuest,
		func(srv *asset.Server, h QuestHandle) asset.Key { return QuestStoreOf(srv).Key(h) }))
}
```

Every `resolver.Resolver` then resolves fields marked `+game:asset:kind=quest` to a `QuestHandle` and writes them back as keys. The asset type's loader must be registered on the server as for any asset. `assetkind.Kind{Load, Key}` is the untyped form that `Of` returns.

| Panic | When |
| --- | --- |
| `assetkind: kind "<name>" is already registered` | `Register` with a name that is taken. A kind has one owner; the engine's names are in [kinds.md](kinds.md#asset-kinds) |
| `assetkind: kind "<name>" needs Load and Key` | `Register` with a `Kind` whose `Load` or `Key` is nil |

To interpret kinds in a different way altogether, implement `types.AssetResolver` yourself (`ResolveAsset(kind, path)` and `AssetKey(kind, h)`) and install it with `types.SetAssetResolver`.
