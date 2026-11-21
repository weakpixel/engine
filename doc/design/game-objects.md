# Game Object generator and GOD design

How `+game:*` markers become typed Go views and a JSON definition, and how `god` uses that definition at run time. The user-facing behavior is in the guide: [Game Object API](../guide/ecs/api-game-object.md) and [GOD](../guide/ecs/game-object-definition.md).

## Goals

- **No new storage model.** A Game Object is an ordinary entity holding ordinary components. The generated view is sugar over `ViewN` and the low-level `World`, so objects mix freely with hand-written systems.
- **One declaration, three consumers.** The same Go struct drives the typed Go API, JSON loading and saving (levels, prefabs, saves), and tools that never compile the game (`wpix doc`, Lua, editors).
- **Generated code works from any module.** It uses exported `ecs` and `god` API only.

## Pipeline (`wpix gen`)

```
load.go      packages.Load; an existing zz__gen.go is blanked through an overlay so stale output can't break loading
markers.go   marker definitions on a marker.Registry (internal/marker collects and parses them, see its package doc)
parse.go     walk the package, flatten embeds into each object's component list
validate.go  numbered rules; every problem becomes a diagnostic; all are collected and sorted
schema.go    group/class and component group/name resolution; Go type → TypeSchema following encoding/json rules
doc.go       doc comments → descriptions (without +game:* lines)
render.go    template data; generator.go.tpl → zz__gen.go; JSON → the package's GOD file
main.go      settings (.weakpixel.json via project); writes <specs>/<group>.god.json (specs: god.specs, default build/god)
```

`generate(dir, patterns)` is pure (returns outputs, writes nothing), which is what the golden and diagnostic tests call. `write` skips unchanged files and removes stale files the generator wrote, recognized by their header and the `"generator": "wpix gen"` field. In the specs folder a file also carries `"package"`: `write` refuses to overwrite another package's file (two packages with one group) and removes only its own package's files under an old name. One folder of `<group>.god.json` instead of a file per package directory gives tools one place to look; naming by group keeps the names readable and stable across package moves.

## What the template generates

- `CID_<Type>` for every component type the package uses, registered at package init. Foreign types get their import alias as prefix (`CID_ExtHitbox`).
- `<Object>View`/`New<Object>View` per object, a value type with the same method set as `ViewN`.
- **Objects with 1..16 required components** use one `ecs.ViewN` over the required components for `Each`, `Get`, `Has`, and for `NewEntity`/`Set` when there are no optionals. With 0 or more than 16 required components the view falls back to `world.Query(...)` with `[]any` assertions.
- **Optional components** always go through the low-level `World` methods (`GetCompOrNil`, `SetComp(e, values ...any)`, `RemoveComp`). `Set` writes all present values with one `SetComp` and removes absent optionals with one `RemoveComp`; that is why no-op structural calls must not bump the world revision ([ECS design](https://github.com/weakpixel/ecs/blob/main/doc/design.md#structural-changes)).
- `init()` calls `types.RegisterComponent` for every component type the package declares or uses (each schema parsed once into a local), then `types.RegisterSchema` for each object, whose components reuse those schemas (descriptions stripped, `TypeSchema.WithoutDescriptions`, so docs never bloat the binary). Several packages register the same foreign component; identical registrations are no-ops.

Required component order in the template follows the field names, sorted; components in the GOD JSON keep declaration order.

## Why the markers are shaped this way

- **Markers are comments**, so object and component structs stay plain Go structs with no tags or base types.
- **`+game:asset` and `+game:runtime` only on direct `json:"-"` fields of a component struct.** The schema is derived with `encoding/json` rules; marking a field that `encoding/json` would also serialize would give it two meanings. Handles are opaque at generation time: the generator can't import the engine, so it accepts any non-pointer struct and leaves the type check to the resolver.
- **Components are global types named `group:name`.** Earlier, a component was named by its Go field name, local to its object; the same type then had as many names as fields, the GOD repeated its schema in every object, and docs, Lua and the editor had to guess which `Transform` was meant. Now the identity belongs to the type, declared once by the package that owns it (`+game:component:group,name` or the package group), so a foreign component has the same key in every package and file. The cost: data breaks when a type is renamed or regrouped (set `name=`/`group=` to keep the old key), no longer when a field is renamed. The field name stays as Go sugar and as `field` in the GOD, for docs.
- **A used component needs an explicit group.** The import-path fallback keeps unused types documentable, but an import path in a level file would tie data to the module layout, so the generator refuses it for any component an object uses.
- **Identity is `group:class` for objects and `group:name` for components**, defaulting to package group and type name, overridable per type or per package, so moving a type between packages does not have to break saved data. Groups and names are Go identifiers, so keys split on the one `:` and are valid Lua and file-name parts.

## GOD versions

A GOD file carries `version`. Nothing is released yet, so the format, like the editor protocol, is at version `0` and changes without a version bump or compatibility code; `wpix doc` reads only version 0 and asks to regenerate anything else. The version starts counting at the first release.

## One GOD per project

`go generate` stays per package; `wpix doc` merges. Components merge by `group:name` and objects by `group:class`; identical definitions (docs ignored, as in `RegisterSchema`) collapse, so the self-contained package files (which repeat the foreign components they use) merge cleanly, and a different definition under one key is an error naming both files. `wpix doc --format god` writes the result (`god.out`), marked `"generator": "wpix doc"` so directory walks skip it instead of reading stale definitions back.

## `god` runtime

- **Registries** of `ComponentSchema`s keyed by `group:name` and `Schema`s keyed by `group:class`. Registering the identical definition twice is a no-op; a different definition under the same key panics. Every instance-key path (`CheckComponents`, the codec errors, `Deserialize`/`Serialize`, prefab `Expand`/`Overrides`) uses `Component.Key()`.
- **`TypeSchema.Validate` reports every violation** with a `$.path`, so an editor or a level loader can show all problems at once.
- **`Deserialize` is all-or-nothing**: it validates, decodes and resolves assets per object, and deletes the entities it already created if a later object fails. It returns `[]Object` in document order so the result goes straight back into `Serialize`.
- **`Serialize` builds the whole document before writing**, so a failure (for example a stale asset handle) never leaves half a file.
- **Object identity is a 4-byte tag component**, `ObjectClass`: an index into an append-only list of registered keys, so the column holds no strings and no GC pointers. `Deserialize` and Lua `world.spawn` add it; generated views do not (their `NewEntity` is a 0-alloc hot path and the template stays unchanged), so `ClassOf` falls back to inference by component set. Tools (catalog, outline, save) use `Schemas`, `ClassOf` and `Objects`; none of it touches `ecs`.
- **Entity references are rewritten on the JSON, driven by the schema.** `encoding/json` reads `{"ID":n,"Generation":g}` into an `ecs.Entity`, so `DecodeComponent` swaps document ids for that form before `Decode`, and `EncodeComponent` swaps it back after `Marshal`; no reflection over component types. A cached per-schema flag skips the walk for components without references, so their output is byte-identical to before. `Deserialize` is two-phase (check and collect ids, then create every entity before decoding any component) because references can point forward. One codec (`DecodeComponent`/`EncodeComponent`) serves `Deserialize`, `Serialize`, Lua and editors.
- **Prefabs are documents too.** A prefab file is a one-object GOD document, so everything that reads documents reads prefabs. An instance is expanded into a plain object *before* `Deserialize`'s two phases (a JSON merge patch per component, objects merged key by key, `null` setting null rather than deleting), so ids, references, validation, rollback and error texts need no prefab-specific code. A class is the minimum of an object, not its limit: any registered component type may be added (`ObjectComponents` builds the list, the class's first). The rule applies to every object, not only prefabs, because a prefab-only rule would need a second validator and would break the round trip of instances that add components. Encoding walks the component registry with one `HasComp` per registered type; `Serialize` is a tool path, not a per-tick one. Objects without any class are not supported. `Serialize` runs the inverse (`diffPatch`, with the schema supplying the zero value of a field `encoding/json` left out) so instances round-trip as overrides. Runtime spawning takes another path: `Bind` decodes the expanded components once, and `BoundPrefab.Spawn` copies the boxed values into `NewEntity` through a reused scratch slice (0 allocs), matching overrides by `reflect.Type` (an override of a registered type outside the prefab is looked up once and cached). `PrefabInstance` is an interned 4-byte tag like `ObjectClass`. Prefabs are found through a `PrefabSource` world resource; the engine's (`prefab`) serves prefab assets, whose base (for variants) is an asset dependency and whose art is held by a scope the prefab owns.
- **Assets** are resolved through a `types.AssetResolver` world resource. `god` defines only the interface; the engine's `resolver` package implements it ([asset-system.md](asset-system.md#authored-data)).
