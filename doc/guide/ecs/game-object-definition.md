# GOD - Game Object Definition

A **GOD** describes a *Game Object*: a named set of components that together form one
kind of entity (a `Hero`, a `Boss`, ...). Game Objects are declared as plain Go structs,
and the generator `wpix gen` turns them into

- typed Go helpers (`NewHeroView` with `NewEntity`, `Get`, `Has`, `Set`, `Remove`, `Each`) in `zz__gen.go`,
- a machine-readable definition per package, the package's **GOD file** (the **GOD JSON schema**):
  `<specs>/<group>.god.json`, where `<specs>` is `god.specs` of the project's [`.weakpixel.json`](../project-file.md)
  or, by default, `build/god` below the module root (build output, not committed),
- a registration of each component type and object schema at program start, which enables
  loading and saving entities as JSON (the **serialized GOD**).

This document covers:

1. [How it works](#1-how-it-works)
2. [GOD JSON schema](#2-god-json-schema)
3. [Serialized GOD](#3-serialized-god)

## 1. How it works

### Declaring objects

Objects and components are declared with `+game:*` comment markers on Go structs. The list of markers and the generated Go API are in [Game Object API](api-game-object.md#declaring-objects). This page covers what they mean for the JSON forms:

- `*T` components are read/write; `T` (value) components are **readonly**: copied out on read, always required, `"readonly": true` in the schema.
- `+game:optional` components may be absent from a serialized object (`"optional": true`). They are suggestions for editors, not a limit: an object may also hold any other [registered component](#extra-components).
- `+game:object:group=g,class=c`, `+game:component:group=g,name=n` (on a component type) and the package's `+game:group:name=g` set the [identity](#identity) of objects and components.
- `+game:asset:kind=k` and `+game:runtime` describe [asset fields](#asset-fields) and [runtime fields](#runtime-fields).

`+game:asset` and `+game:runtime` go on **direct fields of a component struct** (not on objects,
and not on nested, inlined or embedded structs). Both fields must be exported and tagged
`json:"-"`, and a field cannot carry both markers. `+game:asset` also needs a non-empty
`kind` (a free string, interpreted by the resolver only) and a field of non-pointer struct type
(the generator cannot import the engine, so it does not recognise handle types by import path;
the resolver checks the concrete type at runtime). Violations are reported with
`file:line:col`, all at once.

### Identity

**Objects** are registered under the key `group:class`. The group defaults to the
`+game:group:name` of the package, else the package name; the class to the Go type name.
`Hero` in the group `game` has the key `game:Hero`.

**Components** are global types with the key `group:name`, the same everywhere: in GOD
files, levels, prefabs, Lua and the editor (`"gfx:Transform": {...}`).

- `name` defaults to the Go type name; `+game:component:name=n` on the type sets it.
- `group` comes from `+game:component:group=g` on the type, else the `+game:group:name`
  of the package that **declares** the type, else (only for a type no object uses) its
  Go import path. A foreign component keeps the identity its own package gives it:
  `gfx.Transform` is `gfx:Transform` in every package that uses it.
- A component used by an object needs an explicit group (type or package marker), so no
  import path ends up in a data file. A third-party type without markers cannot be used
  directly; wrap it in a local type.
- Both values are Go identifiers (so never contain `:`). Two component types in one
  package's GOD (declared or used) may not share a key.

The Go field name of a component in its object struct is Go sugar only: it names the
field of the generated view and appears in the GOD as `field`, for docs. Instances are
keyed by the component key, so renaming a field changes no data; renaming or regrouping
the type does.

Generator errors (all reported at once, with `file:line:col`):

| Problem | Message |
|---|---|
| Component without a group used by an object | `<pkg>.<Object>.<Field>: component <Type> (<importpath>) is used by object <group:class> but has no group; add +game:component:group=... or +game:group:name to its package` |
| Two types with the same key | `<pkg>.<Type>: component key <group:name> is already used by <importpath.Type>` |
| Not an identifier | `<pkg>.<Type>: +game:component group "<value>" is not a valid identifier` (same for `name`) |
| Arguments on a field marker | `<pkg>.<Object>.<Field>: +game:component takes group and name on the component type, not on a field` |

### Generation

```
//go:generate go run github.com/weakpixel/engine/tools/wpix gen
```

```
+game:* markers --parse/validate--> zz__gen.go                   (typed API + init() registration)
                                \-> <specs>/<group>.god.json     (GOD JSON schema; <specs> is build/god
                                                                  without god.specs)
```

`zz__gen.go` calls `types.RegisterComponent` for every component type the package declares
or uses, then `types.RegisterSchema` for each object. A registered `ComponentSchema` holds
the identity (`Group`, `Name`), the Go type, a `TypeSchema` describing the JSON form, a
`Decode` function and the runtime component ID. A registered `Schema` holds, per
component: its identity, Go `Field` name, `Optional`/`Readonly` flags, schema, decoder and
runtime ID. Registering an identical definition again is a no-op (packages using the same
foreign component all register it); a different one under the same key panics
(`god: component "<key>" registered twice with different definitions`).

In the specs folder, a package's file is named by its **package group** (the
`+game:group:name` marker, else the package name) and records the package's import path
(`"package"`). So two packages with the same group fail the second generation
(`<specs>/game.god.json is generated from package A; package B has the same group "game"; set +game:group:name`)
instead of overwriting each other, and a package whose group changed removes the file it
wrote under the old name, leaving other packages' files alone. `wpix doc --format god`
merges the files into one project GOD (`god.out`, see [tools](../tools.md#doc)).

### Runtime API (`god`)

| Function | Purpose |
|---|---|
| `RegisterComponent(ComponentSchema)` | Register a component type under `group:name` (generated code calls it). |
| `LookupComponent(group, name)` | Find a registered component type. |
| `Components() iter.Seq[ComponentSchema]` | Every registered component type, sorted by key, used by an object or not. |
| `ComponentKey(group, name)` | The key `group:name`; `Component.Key()` and `ComponentSchema.Key()` return it. |
| `LookupSchema(group, class)` | Find a registered object schema. |
| `Schemas() iter.Seq[Schema]` | Every registered schema, sorted by key (iterates a snapshot; registering meanwhile is safe). |
| `ClassTag(group, class) (ObjectClass, bool)` | The [class tag](#object-identity) value of a registered schema, to add to entities you spawn yourself. |
| `ClassOf(world, e) (SchemaMeta, bool)` | Which object type `e` is: its tag, else [inferred](#object-identity). Panics on a stale entity like `SerializeComponents`. |
| `Objects(world) iter.Seq[Object]` | Every entity `ClassOf` resolves, in query order. |
| `Deserialize(world, r) ([]Object, error)` | Read a [serialized GOD](#3-serialized-god) document and create one entity per object. |
| `Serialize(world, w, objs ...Object) error` | The inverse: write a serialized GOD document for the given objects. |
| `SerializeComponents(world, group, class, e)` | Return the JSON `components` object of one entity. |
| `CheckComponents(group, class, raw)` | Check a raw components map (keyed by component key): every required component present, every other key a registered component type. |
| `ComponentOf(s, key) (Component, bool)` | How an object of class `s` holds the component `key`: the class's entry, or an optional `Component` built from the registered type. |
| `ObjectComponents(s, keys) []Component` | The components of an object of class `s` with the given keys: the class's in declaration order, then the other registered keys, sorted. |
| `DecodeComponent(world, c, raw, refs) (any, error)` | One component's JSON to a value for `SetComp`: validate, resolve [entity references](#entity-references) through `refs`, decode, resolve asset keys. `Deserialize` and Lua use it. |
| `EncodeComponent(world, c, v, refs) (json.RawMessage, error)` | The inverse: marshal, asset handles to keys, entities to ids through `refs`. |
| `Refs` / `ObjectRefs(objs)` | Maps entities to document ids (`RefID`) and back (`RefEntity`). `ObjectRefs` uses `Object.ID`. A nil `Refs` makes every non-null reference an error (`no Refs for entity reference`). |
| `SetAssetResolver(world, r)` | Install the [`AssetResolver`](#asset-fields) used for asset properties (a world resource of type `types.AssetResolver`). |
| `ParsePrefab(key, data, src) (*Prefab, error)` | Parse and validate a [prefab](#prefabs) file; `src` finds the base of a variant. |
| `(*Prefab).Bind(world, assets) (*BoundPrefab, error)` / `(*BoundPrefab).Spawn(overrides ...any) ecs.Entity` | Decode a prefab once for a world, then spawn copies without JSON. `Spawn` returns the root of a [multi-object prefab](#multi-object-prefabs). |
| `(*BoundPrefab).SpawnAll(dst, overrides ...any) []ecs.Entity` / `(*Prefab).Parts()` / `(*Prefab).PartIndex(id)` | Spawn a multi-object prefab and get every entity, root first, in `Parts()` order. |
| `PrefabSource` / `PrefabMap` / `SetPrefabSource(world, src)` | Where `Deserialize` and `Serialize` find prefabs by key (a world resource of type `types.PrefabSource`). |
| `PrefabOf(world, e) (string, bool)` / `PrefabPartOf(world, e) (key, part string, ok bool)` | The key of the prefab `e` was made from (its `PrefabInstance` tag), and which of the prefab's objects it is (`part` is `""` for the root). |
| `(*Prefab).Expand(group, class, overrides)` / `(*Prefab).Overrides(components)` | The merged components of an instance with these overrides, and back: the minimal overrides for complete components. For tools such as the level editor; `Deserialize` and `Serialize` use the same functions. |
| `(*Prefab).ExpandInstance(PrefabEntry) ([]ExpandedObject, error)` | The plain objects an instance entry (`id`, `components`, `children`) stands for, root first, with qualified ids. For tools such as the level editor; `Deserialize` and `Serialize` use it. |
| `PrefabTag(key) PrefabInstance` / `PrefabPartTag(key, part)` | The `PrefabInstance` tag of a prefab key (and of one of its parts), for code that creates instances itself (the level editor). |
| `DocumentName(data) string` | The document's top-level `name`, or `""` (none, not a string, invalid JSON); shape-only like `PrefabKeys`. |
| `PrefabKeys(data) ([]string, error)` | The prefab keys a document names, to load them before `Deserialize`. |

All functions identify an object by `group, class`. An `Object` pairs that identity with the entity:

```go
type Object struct {
    SchemaMeta            // Group, Class
    ID         string     // document id, "" when the object has none
    Prefab     string     // key of the prefab it is an instance of, or ""
    Part       string     // id within the prefab of a part of a multi-object instance, or ""
    Entity     ecs.Entity
}
```

`Deserialize` returns `[]Object` in document order, so its result can be passed straight back to `Serialize`:

```go
objs, err := types.Deserialize(world, in)
// ... the game runs ...
err = types.Serialize(world, out, objs...) // writes the same document shape
```

`Deserialize` works in two phases, so references can point forward (after expanding [prefab instances](#prefabs) into plain objects, several for an instance of a [multi-object prefab](#multi-object-prefabs)):

1. For every object: look up `group:class`, check required and [unregistered](#extra-components) components, validate each component's JSON against its `TypeSchema` (a runtime property in the input is an error), and collect the `"id"`s (duplicates are errors). Problems of all objects are reported together.
2. Create one entity per object (with its `ObjectClass` tag), then for every object decode its components with `DecodeComponent` (references resolved to the new entities, [asset properties](#asset-fields) resolved through the world's `AssetResolver`) and set them.

`Deserialize` is **all-or-nothing**: if any object fails, entities created earlier in
the same call are deleted again.

### Object identity

`types.ObjectClass` is a component that says which object type an entity is. It is 4 bytes (an index into the process's registered schemas, never persisted) and belongs to no schema, so `Serialize`, Lua field access and `wpix doc` never see it. `Deserialize` and Lua's `world.spawn` add it; the generated `New<Object>View(...).NewEntity` does not, to keep that path allocation-free and unchanged.

`ClassOf` uses the tag when there is one. For an untagged entity it infers the type: the candidates are the schemas whose required components the entity all has, and the one with the most of its components (required and optional) present wins. So an entity with every component of `Boss` is a `Boss`, even though it also has everything a `Hero` needs. A tie (two schemas with the same component set) or no candidate returns `false`; a schema without components never matches. Inference costs a component lookup per schema component, so it is for tools, not per-tick code.

`Objects` gives "save everything" in one line:

```go
err := types.Serialize(world, out, slices.Collect(types.Objects(world))...)
```

## 2. GOD JSON schema

One file per generated package (`<specs>/<group>.god.json`). It
lists the package's component types, each with its JSON shape once, and its objects,
which refer to components by key.

```json
{
  "version": 0,
  "generator": "wpix gen",
  "package": "example.com/game/gametypes",
  "components": [
    {
      "group": "game",
      "name": "Weapon",
      "doc": "Weapon is an optional offensive component.",
      "goType": "example.com/game/gametypes.Weapon",
      "schema": {
        "type": "object",
        "properties": {
          "Ammo":   { "type": "integer", "format": "int64" },
          "Damage": { "type": "number",  "format": "double" }
        },
        "required": ["Damage", "Ammo"]
      }
    }
  ],
  "objects": [
    {
      "group": "game",
      "class": "Hero",
      "doc": "Hero is the player character.",
      "goType": "example.com/game/gametypes.Hero",
      "components": [
        {
          "component": "game:Weapon",
          "field": "Weapon",
          "optional": true,
          "doc": "The weapon the hero carries, if any."
        }
      ]
    }
  ]
}
```

### File fields

| Field | Meaning |
|---|---|
| `version` | Format version, `0` until the first release; readers accept exactly this version. |
| `generator` | `"wpix gen"` for a package's file (the generator uses it to recognize its own stale files), `"wpix doc"` for a merged project GOD. |
| `package` | Import path of the package the file is generated from. Not in a merged GOD. |
| `components[]` | Every component type the package declares (`+game:component` on the type, used or not) or its objects use (foreign ones included, so the file is self-contained), sorted by key. Merging files dedupes them. |
| `objects[]` | One entry per Game Object. |
| `objects[].group`, `.class` | Identity (`group:class`). |
| `objects[].doc` | Go doc comment of the object type, without `+game:*` lines. Omitted when empty. |
| `objects[].goType` | The object's Go struct, `importpath.Type` (the class name need not be the struct name). Documentation, used by editors to jump to the source; `RegisterSchema` does not compare it, `wpix doc` does. |
| `objects[].components[]` | The object's components in declaration order (embedded objects are flattened in). |

### Component type fields (`components[]`)

| Field | Meaning |
|---|---|
| `group`, `name` | [Identity](#identity); the key `group:name` names the component in instances. |
| `doc` | Go doc comment of the component type, without `+game:*` lines. Omitted when empty. |
| `goType` | The Go type, `importpath.Type`. Documentation. |
| `schema` | `TypeSchema` of the component's JSON form. |

### Object component fields (`objects[].components[]`)

| Field | Meaning |
|---|---|
| `component` | Key `group:name` of the component type; its schema is in `components[]`. |
| `field` | Go field name in the object struct. Documentation; instances do not use it. |
| `optional` | `true` if the component may be absent. Omitted when `false`. |
| `readonly` | `true` for value (`T`) components. Always required. Omitted when `false`. |
| `doc` | Doc comment of the object field that declares the component (or its trailing line comment when there is no doc comment), without `+game:*` lines. Omitted when empty. |
| `embeddedVia` | Names of the embedded objects the component is promoted through, outermost first (`["Common"]`, `["Common","Base"]`). Omitted when the object declares the component directly. |

`optional` and `readonly` never occur together. The same shape (with `schema` filled in)
is what the editor protocol's `ready` message carries per object component.

### TypeSchema

A subset of OpenAPI v3 / JSON Schema, same keywords and meaning, plus the `x-` extensions below.

| Keyword | Applies to | Meaning |
|---|---|---|
| `type` | all | `object`, `array`, `string`, `integer`, `number`, `boolean`. |
| `format` | numbers, strings | `int32`, `int64`, `float`, `double`, `byte` (base64 string, validated). |
| `nullable` | all | `null` is accepted (nil pointers, slices, maps). Otherwise `null` is an error. |
| `properties` | object | Known properties, keyed by JSON name. Other keys are errors. |
| `required` | object | Property names that must be present. |
| `additionalProperties` | object | Schema for every other key (maps). Lifts the unknown-property error. |
| `items` | array | Schema of each element. |
| `minItems`, `maxItems` | array | Length bounds (fixed-size Go arrays set both). |
| `minimum`, `maximum` | integer, number | Value bounds (set for sized Go integer types). |
| `description` | any | Documentation: the Go doc comment of a struct field on its property (doc comment, else trailing line comment). The component type's own doc is the component's `doc`. No effect on validation. |
| `x-go-type` | any | Go type with a custom JSON encoding (e.g. `time.Time`). Shape is not described and **not validated**. |
| `x-preserve-unknown-fields` | any | Value of unknown shape (interfaces, recursive types, funcs). **Not validated**. |
| `format: "asset"` | string | The string is an asset key, see [Asset fields](#asset-fields). |
| `format: "entity"` | string | The string is the id of another object of the same document, see [Entity references](#entity-references). An empty string is a validation error. |
| `x-asset` | string with `format: "asset"` | The asset kind (`sprite`, `clip`, `font`, ...). A free string; only the resolver interprets it. On a non-string schema it is a validation error. |
| `default` | any | The value an editor writes for a new component, declared with `+game:default`, see [Defaults](#defaults). Never applied when decoding; `Validate` ignores it. |
| `x-editor` | any property | Hints for editors, declared with `+game:editor`, see [Editor hints](#editor-hints). Never used when decoding; `Validate` ignores them. |
| `x-runtime` | any property | Runtime state, see [Runtime fields](#runtime-fields). `Validate` rejects the property in input. |
| `x-enum` | integer | The named values of a `+game:enum` type, `[{"value": 0, "name": "EmitPoint", "doc": "..."}, ...]` sorted by value, see [Enums](#enums). `Validate` accepts only these values. |

### Entity references

A component field of type `ecs.Entity` (direct, pointer, slice or array element, map value, or nested in a struct) gets the schema `{"type": "string", "format": "entity", "nullable": true}`, with no marker. In a [serialized document](#3-serialized-god) its value is the `"id"` of another object of the same document, or `null` for the zero `Entity`:

```json
{"id": "scout", "group": "game", "class": "Hero", "components": {"game:Follow": {"Target": "leader", "Distance": 2}, ...}},
{"id": "leader", "group": "game", "class": "Hero", "components": {...}}
```

Ids are local to the document, so the same document (a prefab) can be loaded many times and each copy links to its own objects. References to objects in other documents, and to entities that are not objects, are not supported.

### Asset fields

Asset handles (the engine's `gfx.SpriteHandle`, `gfx.ClipHandle`, ...) are runtime values: their slot numbers mean
nothing in a file, so components tag them `json:"-"`. Authored data (prefabs, levels, an
editor) needs to say which sprite or clip an entity uses, so a field marked
`+game:asset:kind=sprite` is described in the schema by its **key** instead:

```json
"ID": { "type": "string", "format": "asset", "x-asset": "sprite" }
```

The key is a file path relative to the asset root with an optional `#sub` (an atlas frame or
an Aseprite tag): `chars/player.aseprite.json#run`. The asset property is never `required`.
`god` converts key and handle through a resolver, which the engine implements on top
of its asset server. `ecs` and `god` do not import the engine.

```go
type AssetResolver interface {
    ResolveAsset(kind, path string) (any, error)       // key -> handle (assignable to the field type)
    AssetKey(kind string, h any) (string, error)       // handle -> key
}
types.SetAssetResolver(world, resolver)                 // stored as the world resource types.AssetResolver
```

Behavior:

- **Deserialize:** a present, non-empty asset property is resolved and set on the field.
  Absent or `""` leaves the zero handle (the renderer draws a placeholder) and needs no
  resolver. Errors, collected with `errors.Join` and prefixed like other component errors:
  a non-empty key but no resolver (`$.ID: cannot resolve sprite asset "x": no AssetResolver set`),
  an error from `ResolveAsset` (`$.ID: resolve sprite asset "x": ...`) and a result that is not
  assignable to the field (`$.ID: resolver returned T for sprite asset "x", want U`).
- **Serialize / SerializeComponents:** a zero handle omits the property. Otherwise `AssetKey`
  writes the key; a missing resolver or an `AssetKey` error is an error
  (`component gfx:Sprite: $.ID: cannot write sprite asset: no AssetResolver set`).
  Components that have asset properties are re-marshalled, so their keys come out in sorted order.
- The property is found by reflection from the schema: the Go field with the property's name
  (fields are `json:"-"`, so the name is the Go field name). Components without asset
  properties skip this entirely (0 allocations).
- `Component.Decode(null)` stays resolver-free; it is how a scripting binding learns the Go type.
- **Lifetime:** the interface has no scope parameter. The engine's resolver
  (`resolver`) loads into the scope the game sets with `SetScope(levelScope)` before
  deserializing a level, so releasing that scope frees the art. Deleting an entity releases
  nothing.
- Asset fields are direct fields of a component only; slices, maps and nested structs of
  handles are not supported.

### Runtime fields

A field marked `+game:runtime` (for example `Animator.Time`) keeps its normal schema plus
`"x-runtime": true` and is never `required`. It is in the schema so that scripts and docs
see it, but it is not authored data: `Serialize` omits it (the field is `json:"-"`), and a
runtime property in the **input** of `Deserialize` is an error
(`component gfx:Animator: $.Time: runtime field, not authored data`), so an editor that wrongly
writes one is told. `wpix doc` marks such fields; a scripting binding can read and write them.

### Enums

A named integer type marked `+game:enum` lists its members in the schema of every field
that uses it (directly, through a pointer, as slice items or map values):

```go
// +game:enum
type EmitShape uint8

const (
    // EmitPoint spawns every particle at the emitter's position.
    EmitPoint EmitShape = iota
    EmitCircle // EmitCircle spawns inside a disc.
)
```

```json
"Shape": { "type": "integer", "format": "int32", "minimum": 0, "maximum": 255,
           "x-enum": [ { "value": 0, "name": "EmitPoint", "doc": "EmitPoint spawns every particle at the emitter's position." },
                       { "value": 1, "name": "EmitCircle", "doc": "EmitCircle spawns inside a disc." } ] }
```

The members are the exported package-level constants of exactly that type in the declaring
package, sorted by value; `name` is the Go constant name and `doc` its doc comment (else its
trailing line comment). Untyped constants, constants of other types and unexported ones are
not members. The JSON value stays the integer, so levels, prefabs, Lua and the Go types are
unchanged; `name` is runtime data (the editor shows it), `doc` is documentation (see
[Documentation fields](#documentation-fields)). A `[]T` of a `uint8` enum is a byte string
in `encoding/json` and carries no `x-enum`.

The marker is opt-in, so an open set (for example `sound.Bus`, where games add their own
values) stays a plain number. Generator errors:

| Problem | Message |
|---|---|
| Not an integer type | `<pkg>.<Type>: +game:enum requires a named integer type, got <type>` |
| Alias or generic type | `<pkg>.<Type>: +game:enum requires a named, non-generic integer type` |
| No exported constants | `<pkg>.<Type>: +game:enum has no exported constants of type <Type>` |
| No constant with value 0 | `<pkg>.<Type>: +game:enum has no constant with value 0; the zero value would not validate` |
| Two constants with one value | `<pkg>.<Type>: +game:enum constants <A> and <B> have the same value <n>` (make one unexported if it must stay) |
| Custom JSON (`MarshalJSON`, `MarshalText`) | `<pkg>.<Type>: +game:enum on a type with MarshalJSON or MarshalText; its wire value is not the integer` |
| On a field | `marker +game:enum is not valid on a field` |

Zero must be a member because a zero-valued component (a new entity, an added component)
must serialize and validate.

### Defaults

A field marked `+game:default` carries the value an editor writes when it creates the
component, as the JSON Schema keyword `default` (the wire value):

```go
type Label struct {
    // +game:default="Hello"
    Text string
    // +game:default=BlendAdditive
    Blend gfx.Blend // an enum: the constant's name; the schema holds the integer
    // +game:default:X=1,Y=1
    Scale gfx.Vec2 // leaves by path; several marker lines merge
    // +game:default:walk=2,run=4
    Speeds map[string]float32 // a map key is a path segment
    // +game:default={1, 2, 3}
    Steps []int // a slice or array takes a whole list
}
```

```json
"Scale": { "type": "object", "properties": { "X": { "type": "number", "format": "double", "default": 1 },
                                              "Y": { "type": "number", "format": "double", "default": 1 } } }
"Speeds": { "type": "object", "additionalProperties": { "type": "number", "format": "float" },
            "default": { "run": 4, "walk": 2 } }
```

`+game:default=v` sets the field itself (a scalar, or a list for a slice or array whose
length must match); `+game:default:a.b=v,c=w` sets leaves by dotted Go field names, which
the generator maps to the JSON names. A map key is a segment (`walk`), only at the end of a
path. Every value is a Go constant expression of the leaf's type, evaluated in the field's
file (so `2 * Tile`, `paint.Red` and `ModeBold` work; an untyped number also works for an
enum if it is a member) and checked against the leaf's schema (overflow, enum membership).
Values in a path list end at the next comma unless quoted. A path default lands on the leaf
in that use only; a default on a field of a nested struct applies wherever the struct is
used, and a path from the outer field overrides it for that leaf.

Defaults are **editor seed values, never applied at runtime**: decoding a level or prefab
that omits the field still gives the Go zero value, and `NewEntity`, `SetComp` and `w.Add`
are unchanged. The schema's `InitialValue()` is what an editor writes for a new component:
the `default` if set, else for an object the initial value of each property, else the zero
value (arrays `minItems` long, numbers 0, `false`, `""`, entity references and values of
unknown shape `null`; runtime properties are left out, and so are pointer properties
without a `default`: nil means "not set", so a new object does not override what another
source provides, such as the particle effect of a `gfx.Particles`). `default` is data, not
documentation: it stays in the compiled schemas and `RegisterComponent` compares it.

Generator errors (all collected, at the marker's line):

| Problem | Message |
|---|---|
| Value does not evaluate or fit | `<pkg>.<Type>.<Field>: +game:default: "x" cannot be used as float64`, `300 overflows uint8`, `undefined: Missing`, `f() is not a constant`, `7 is not a value of the enum (...)` |
| Field form on a struct or map, or a list of structs | `...: a struct needs its fields set by path (+game:default:name=value)` |
| Path names no field, or goes through a scalar, slice or map of structs | `...: +game:default:Z: DefVec has no field Z`, `cannot select X in a scalar` |
| Same path set twice | `<pkg>.<Type>.<Field>: +game:default sets path X more than once` |
| Field form and path form on one field | `...: +game:default mixes the field form (=value) with the path form (:path=value)` |
| Entity reference, asset field, runtime field, `json:"-"` or unexported field, `,string` or an undescribed type | `...: +game:default is not allowed on an entity reference (ids belong to one document)` and similar |
| On an object field (`+game:component`, `+game:embed` or plain) | `...: +game:default is only valid on a field of a component struct (or a struct nested in one), not on an object field` |

### Editor hints

`+game:editor` on a field tells an editor how to show and edit it. It is for the property list of an editor such as the VSCode inspector; nothing at run time reads it. All arguments are optional and the marker takes any of them in one line:

```go
type Light struct {
    // +game:editor:name="Brightness",min=0,max=10,step=0.5,order=0
    Power float32
    // +game:editor:unit=deg,min=0,max=360,step=5
    Angle float32
    // +game:editor:color=true
    Tint gfx.Color
    // +game:editor:showIf=Shape=EmitRect,min=1
    Size Vec2
    // +game:editor:hidden=true
    Cache int32
}
```

| Argument | Meaning |
| --- | --- |
| `name` | Label shown instead of the Go field name. |
| `min`, `max`, `step` | Bounds and step of a number, or of every element of an array or object of numbers (`Vec2`). With both `min` and `max` the editor shows a slider. They guide the widget; they do not validate. With `unit=deg` they are in degrees. |
| `unit` | A label such as `s` or `u/s`. `unit=deg` is special: the field is stored in radians and the editor shows degrees. |
| `order` | Position among the siblings, lowest first. Fields without `order` follow, in declaration order. |
| `color` | `true` asks for a color picker. The field is an integer packed as `0xRRGGBBAA` (like `gfx.Color`), an object with `R`, `G`, `B` (and `A`), or an array of 3 or 4 numbers. |
| `hidden` | `true` keeps the field out of the editor's property list. It stays in the file. |
| `showIf` | The field is shown only while a condition on a sibling of the same struct holds, named by its JSON name: `Flag` (truthy), `!Flag`, `Shape=EmitRect` (an enum constant name, an integer, `true`/`false`, or a number or string) or `Shape!=EmitPoint`. |

The hints are one `x-editor` object on the property, `{"name": "Brightness", "min": 0, "max": 10, "step": 0.5, "unit": "deg", "order": 0, "color": true, "hidden": true, "showIf": {"field": "Shape", "equals": 2, "not": false}}` with only the keys that were given; `showIf` has the enum already resolved to its integer. Like `default` they stay in the compiled schema, so an editor built into the game sees them. `wpix schema` leaves them out.

Wrong hints fail `wpix gen` with the field's file and line: a bound or step that is not a number (or a step of 0, or `min` above `max`), `order` that is not an integer, bounds or `unit=deg` on a field that is not numeric, `color` on a field of another shape, a `showIf` that names a missing sibling, the field itself or a value its type cannot take, an unknown argument, and the marker on a field that is not in the schema (`json:"-"` without `+game:asset` or `+game:runtime`, an unexported or an embedded field).

### Documentation fields

`doc` (of components, objects and `x-enum` members), `goType`, `field`, `embeddedVia` and `description` exist for `wpix doc` (see
[api-game-object.md](api-game-object.md#documentation)). They are **not part of the
runtime**: the generated `zz__gen.go` does not contain them (the schemas compiled into it have
every `description` and `x-enum` `doc` removed), and `RegisterComponent` and `RegisterSchema` ignore them
when they compare two definitions. Exceptions: `goType` and `field` are in the generated code (the editor protocol sends them); `RegisterComponent` compares a component's `goType` and `RegisterSchema` compares `field`, while an object's `goType` is not compared. The text is `ast.CommentGroup.Text()` with the `+game:*` marker lines removed
and the result trimmed; a leading `Name ` is kept as written.

Go-to-schema mapping follows the `encoding/json` rules, including `json:"name"` tags,
`omitempty` and `json:"-"`; unexported fields are skipped and embedded structs are
flattened.

Examples:

```json
{ "type": "string" }
{ "type": "array", "items": { "type": "string" }, "nullable": true }
{ "type": "array", "items": { "type": "integer" }, "minItems": 3, "maxItems": 3 }
{ "type": "object", "additionalProperties": { "type": "integer" }, "nullable": true }
{ "type": "string", "format": "byte", "nullable": true }
{ "x-go-type": "time.Time" }
{ "x-preserve-unknown-fields": true }
```

### Validation

`TypeSchema.Validate` reports **all** violations at once, each prefixed with the path of
the value (`$` is the component root):

```
$.Damage: expected number, got string
$.Ammo: expected integer, got 1.5
$: missing required property "Ammo"
$.Extra: unknown property "Extra"
$.Pos: null is not allowed
$.Shape: 7 is not a value of the enum (0 EmitPoint, 1 EmitCircle, 2 EmitRect)
```

## 3. Serialized GOD

The serialized GOD is the instance data: the entities of a world as JSON. It is what
`types.Deserialize` reads. It is *not* the same as the GOD JSON schema above - the schema
describes object *types*, the serialized GOD holds object *instances*.

### Document

```json
{
  "objects": [
    {
      "group": "game",
      "class": "Hero",
      "components": {
        "game:Transform":    { "X": 10, "Y": 20 },
        "game:Acceleration": { "X": 0,  "Y": 0 },
        "game:State":        { "Active": true, "Kind": "idle" },
        "game:Health":       { "Current": 80, "Max": 100 },
        "game:Vel":          { "X": 1, "Y": 0 },
        "game:Hitbox":       { "Width": 2, "Height": 3, "OffsetX": 0, "OffsetY": 0 },
        "game:Companion":    { "Name": "Rex", "Attachment": null },
        "game:Weapon":       { "Damage": 5, "Ammo": 12 }
      }
    }
  ]
}
```

| Field | Meaning |
|---|---|
| `name` | Optional string, top level only: a human-readable label for editors (the VS Code extension lists prefabs by it). A level or a prefab may carry it. `DocumentName(data)` reads it from either, `(*Level).Name()` returns a loaded level's and `(*Prefab).Name()` a prefab's (`""` if none). It is not unique and never used for lookups; `Deserialize` ignores it and `Serialize` never writes it (editors edit the file text, so the name survives). |
| `objects` | Required array. Each entry creates one entity; the returned entities keep this order. |
| `id` | Optional string, unique within the document: the target of [entity references](#entity-references). Returned as `Object.ID`. |
| `prefab` | Optional key of a [prefab](#prefabs): the object is an instance of it, `group` and `class` may be left out, and `components` holds overrides. Returned as `Object.Prefab`. |
| `children` | Optional object, only with `prefab`: overrides of the other objects of a [multi-object prefab](#multi-object-prefabs), by part id (`null` removes one). |
| `group`, `class` | Select the registered schema (`group:class`). |
| `components` | Object mapping the component key (`group:name`) to its JSON value, shaped by the component's `TypeSchema`. |

Rules:

- Only these six keys per object are allowed; unknown keys are errors. At the top level only `name` and `objects` are allowed.
- Every required component must be present; optional ones (`game:Weapon` above) may be left out.
- Any other [registered component](#extra-components) may be present too; a key that names no registered component type is an error.
- Ids must not contain `/`: it is reserved for the [parts of prefab instances](#multi-object-prefabs).
- Nothing may follow the JSON document.
- Optional components that are absent are simply missing - they are not written as `null`.

#### Extra components

The object type sets the **minimum** of an object, not its limit. An object, in a level or a prefab, may hold any component type in the component registry (`RegisterComponent`, called by generated code for every component a package declares or uses), not only those of its class. Such an extra component is optional: it is validated against its registered `TypeSchema`, decoded, encoded and merged like a component of the class. `Serialize` writes every registered component the entity has: first the class's, then the others. The class still decides the `group:class` of the object, its required components and the components its typed view queries.

```json
{"id": "chest", "group": "gfx", "class": "Visual", "components": {
  "gfx:Transform": {...}, "gfx:Sprite": {...},
  "game:Armor": {"Value": 3}
}}
```

An entity without its optional component:

```json
{
  "objects": [
    {
      "group": "game", "class": "Common",
      "components": {
        "game:Transform":    { "X": 0, "Y": 0 },
        "game:Acceleration": { "X": 0, "Y": 0 },
        "game:State":        { "Active": false, "Kind": "" }
      }
    }
  ]
}
```

### Prefabs

A **prefab** is a pre-defined Game Object: a file (`*.prefab.json`) that fixes the component *values* of one object, so levels and code can place or spawn it many times. The object type (`+game:object`) says which components an object has at least; the prefab says what they start with, and may add any other [registered component](#extra-components); an **instance** says where it is and what differs.

A prefab file is a serialized GOD document with **one or more objects**, each valid like any other (every required component and property present). The first object is the **root**; see [multi-object prefabs](#multi-object-prefabs) for the others:

```json
{"objects": [{"id": "self", "group": "game", "class": "Hero", "components": {
  "game:Transform": {"X": 0, "Y": 0}, "game:Health": {"Current": 10, "Max": 10}, ...
}}]}
```

The optional top-level `"name"` labels the file for editors (`Prefab.Name()`); a variant does not inherit its base's name. Its `"id"` is optional with one object; entity references inside the prefab may point at it (a reference to `"self"` is a reference to each spawned copy). Every entity reference in a prefab must name an object of the same prefab (`unknown object id "x"`).

**Instances.** A document entry with `"prefab"` is an instance. Its `components` are **overrides**, merged into the prefab's components:

```json
{"id": "k1", "prefab": "prefabs/knight.prefab.json", "components": {"game:Transform": {"X": 5}}}
```

- JSON objects merge key by key, recursively: `{"game:Transform": {"X": 5}}` changes `X` and keeps the prefab's `Y`, so a later change to the prefab reaches every instance that did not override that field.
- Arrays, strings, numbers and booleans replace the prefab's value. `null` inside a component sets the field to null (unlike RFC 7386, it never deletes).
- A component the prefab lacks is added (any registered component type); `"game:Weapon": null` removes an optional component the prefab has. Removing a required component is the usual `missing required component` error.
- `group` and `class` may be left out; if given, they must be the prefab's.
- The merged object is validated like any other, with the usual error texts; errors carry the `objects[i]:` prefix.

**Variants.** A prefab's object may itself be an instance of another prefab (its base), with overrides: `{"objects": [{"prefab": "prefabs/knight.prefab.json", "components": {"game:Weapon": {...}}}]}`. Chains are limited to 16 prefabs; a cycle is an error naming it (`prefab cycle: a -> b -> a`).

#### Multi-object prefabs

A prefab file may hold several objects: a root plus objects that belong to it, like a hero with a sword and a shadow. Levels place the group with one entry, and code spawns it with one call. `god` does not know what ties the objects together (the engine requires every non-root object to be attached to the root, see [engine game objects](../engine/game-objects.md#prefabs)); to it, a prefab is a set of objects that are created together, with the references between them remapped for each copy.

```json
{"name": "Knight", "objects": [
  {"id": "knight", "group": "gfx", "class": "Visual", "components": {...}},
  {"id": "sword", "group": "gfx", "class": "Visual", "components": {..., "game:Follow": {"Target": "knight", "Speed": 1}}},
  {"id": "shadow", "group": "gfx", "class": "Visual", "components": {..., "game:Follow": {"Target": "knight", "Speed": 2}}}
]}
```

- With more than one object, every object needs an `id`. The other objects are the root's **parts**, named by their id.
- An instance of a multi-object prefab needs an `id` too. Expanding it gives the root the instance's id and each part `<id>/<part>` (`k1`, `k1/sword`, `k1/shadow`). The prefab's own references are rewritten into that id space first, then the overrides are merged, so after expansion an instance is plain objects of the document.
- `components` overrides the root, as before. **`children`** maps a part id to that part's component overrides, merged like `components`:

  ```json
  {"id": "k1", "prefab": "prefabs/knight.prefab.json",
   "components": {"game:Transform": {"X": 440, "Y": 250}},
   "children": {"sword": {"gfx:Sprite": {"ID": "items/axe.png"}}, "shadow": null}}
  ```

  `"<part>": null` removes that part from this instance. A remaining object that still references a removed part is an error that names both (`children: removing "sword" leaves "glint" with a reference to it`), so removing a subtree means listing every part of it. A key that is no part is an error (`children: prefab "k" has no object "x"`); the root is overridden with `components`.
- Override values are written in the **document's** ids, so a part may be pointed at a level object (`"children": {"sword": {"game:Follow": {"Target": "hero"}}}`), and other level objects may reference a part (`"Target": "k1/sword"`). To add an object to one instance, put a normal object in the level and attach it to `k1` or `k1/sword`.
- `Deserialize` returns the parts right after their root, in prefab order, with `Object.ID` `<id>/<part>`, `Object.Part` the part id and `Object.Prefab` the prefab key. `Serialize` folds objects with a `Part` back into their root's `children`, diffed like `components`; a part of the prefab that is not among the objects is written as `null`. A part whose root is not among the objects, or whose ID is not `<root ID>/<Part>`, is an error. So `Deserialize` followed by `Serialize` gives back the authored entry.
- `PrefabPartOf(world, e)` returns the prefab key and part id of an entity; `PrefabOf` still returns the key.
- The same expansion is a method: `(*Prefab).ExpandInstance(PrefabEntry)` returns the plain objects of an entry (`ID`, `Group`, `Class`, `Components`, `Part`), root first. The level editor uses it. `Prefab.Expand` and `Prefab.Overrides` stay what they were and see only the root.

**Nesting.** An object of a prefab file after the first may itself be an instance of another prefab (`{"id": "horse", "prefab": "prefabs/horse.prefab.json", "children": {...}}`). Parsing flattens it the same way: the knight's parts are `knight`, `sword`, `horse`, `horse/saddle`, and an instance overrides `"children": {"horse/saddle": {...}}`. The nested instance needs an `id`, like every object of a multi-object prefab.

**Variants of multi-object prefabs.** A variant's first object is an instance of its base and may use `children`. It inherits the base's part ids unqualified, so instances of the variant address the same names (`sword`, not `<root id>/sword`). It keeps the base's root id unless it names its own, and may add objects after its first; their ids must not clash with the base's (`duplicate id "sword" (also in base "k")`) and they reference base parts by their plain ids.

Chains of variants and nested prefabs are limited to 16 prefabs deep.

**Finding prefabs.** `Deserialize` and `Serialize` look prefabs up through the world's `PrefabSource` (`SetPrefabSource`); without one, an instance is an error (`prefab "<key>": no PrefabSource set`). `PrefabMap` is a source over parsed prefabs (`m.Add(key, data)`, bases first). The engine's source loads prefab assets (see [engine game objects](../engine/game-objects.md#prefabs)).

**Spawning from code.** `Bind` decodes a prefab's components once (asset keys resolved through the given `AssetResolver`, or the world's); `Spawn` creates an entity from those values without reading JSON. Each override is a component value that replaces the prefab's value of its type, or adds a component the prefab lacks (any registered component type):

```go
knight, _ := prefabs["prefabs/knight.prefab.json"].Bind(world, nil)
e := knight.Spawn(gametypes.Transform{X: 3, Y: 4})
```

`Spawn` allocates nothing beyond what the caller boxes (`TestBoundPrefabSpawnAllocs`, `BenchmarkPrefabSpawn`) for a prefab without entity references. For a [multi-object prefab](#multi-object-prefabs) `Spawn` creates every object and returns the root; the overrides apply to the root only (typically its transform, which moves the group). `SpawnAll(dst, overrides...)` appends every spawned entity to `dst` in `Prefab.Parts()` order, root first, and returns it; `Prefab.PartIndex(id)` finds a part in that order:

```go
ents := knight.SpawnAll(buf[:0], gametypes.Transform{X: 3, Y: 4})
sword := ents[prefab.PartIndex("sword")]
```

A component with entity references is decoded once by `Bind`, with placeholder entities, and `Spawn` copies the value and patches the entity fields, so it reads no JSON and costs one allocation per such component (`TestBoundPrefabMultiSpawnAllocs`, `BenchmarkPrefabSpawnMulti`). This works when every reference sits at a fixed position of the Go value (struct fields and arrays); a reference under a slice, a map or a pointer makes `Spawn` decode that component from JSON per spawn. Per-part overrides from code are not an API: set components on the returned entities. An override of a type that is neither a component of the class nor a registered component panics: `god: Spawn: <T> is not a registered component`. The first override of a registered type outside the prefab's components looks the type up once; later spawns with it allocate nothing.

**Identity.** Instances from `Deserialize` and `Spawn` carry a `PrefabInstance` tag (like `ObjectClass`: 8 bytes, process-local, in no schema); `PrefabOf` reads it and `Objects` fills `Object.Prefab` and `Object.Part`. Instances are copies: a prefab changed later affects new spawns and documents loaded again, not live entities.

### Serializing

`types.Serialize(world, w, objs...)` writes one document with one entry per `Object`,
in argument order. Each object gets every component of its class the entity has, then every other registered component it has. Optional components an entity lacks are left out; a missing required
component or an unregistered `group:class` is an error. The whole document is built
before anything is written, so on error nothing is written to `w`. A stale entity is a
programmer error and panics (see [ecs panics](https://github.com/weakpixel/ecs/blob/main/doc/api-low-level.md#panics)).

References are written as ids: an object's id is its `Object.ID`; a referenced object
without one gets `"_<i>"`, its index in `objs`. `"id"` is written only for objects that
have one after that, so a document without references (or ids) has none, exactly as
before. A reference to an entity that is not among `objs` (stale or simply not passed)
and two objects with the same final id are errors. Editors that want stable diffs give
every object an `ID` before saving.

An `Object` with `Prefab` set is written as an instance: `"prefab"` instead of `group` and
`class`, and as `components` only what differs from the prefab (sub-objects reduced to
their changed keys, a removed optional component as `null`, a field encoding/json left
out as its zero value); `components` is omitted when nothing differs. The objects of an instance of a [multi-object prefab](#multi-object-prefabs) (`Object.Part` set) are folded into the root's `children`. So `Deserialize`
followed by `Serialize` gives back the authored instances. A map entry of the prefab that
the entity no longer has cannot be expressed as an override and is an error.

`types.SerializeComponents(world, "game", "Hero", e)` returns only the `components`
object (`{"game:Transform": {...}, ...}`) of a single entity, with the same rules. Use it to
embed object data in your own file format. It has no document, so a non-zero entity
reference is an error; use `EncodeComponent` with your own `Refs` instead.

### Errors

Errors of an object are prefixed with its index: `objects[1]: ...`. This applies to both
`Deserialize` (index in the document) and `Serialize` (index in `objs`). An error in one of the
parts an instance expands to names the entry and the part: `objects[1] part "sword": ...`.

| Input | Error |
|---|---|
| Missing `objects` | `deserialize: missing "objects"` |
| Unknown top-level key | `deserialize: json: unknown field "..."` |
| Data after the document | `deserialize: unexpected data after the JSON document` |
| Unregistered `group:class` | `unknown object game:Nope` |
| Required component absent | `object game:Hero: missing required component game:Health` |
| Component type not registered | `object game:Hero: component Nope is not registered` |
| Wrong value shape | `component game:Health: $.Max: expected number, got string` |
| `x-runtime` property in input | `component gfx:Animator: $.Time: runtime field, not authored data` |
| Integer that is not an `x-enum` value | `component gfx:Sprite: $.Blend: 7 is not a value of the enum (0 BlendNormal, 1 BlendAdditive, 2 BlendMultiply, 3 BlendScreen, 4 BlendSubtract, 5 BlendLighten)` |
| Asset key without resolver | `component gfx:Sprite: $.ID: cannot resolve sprite asset "a.png": no AssetResolver set` |
| Resolver failure | `component gfx:Sprite: $.ID: resolve sprite asset "a.png": <resolver error>` |
| Duplicate `id` | `objects[1]: duplicate id "a" (first used by objects[0])` (also from `Serialize`) |
| Reference to an unknown id | `component game:Follow: $.Target: unknown object id "zz"` |
| Empty reference | `component game:Follow: $.Target: empty entity reference` |
| Reference without `Refs` (`SerializeComponents`, `DecodeComponent` with nil) | `component game:Follow: $.Target: no Refs for entity reference` |
| `Serialize`: reference outside `objs` | `component game:Follow: $.Target: reference to entity {7 1}, which is not one of the serialized objects` |
| Unknown or unavailable prefab | `prefab "p.prefab.json": unknown prefab` (the source's error) |
| Instance without a `PrefabSource` | `prefab "p.prefab.json": no PrefabSource set (see SetPrefabSource)` |
| Instance `group:class` differs from the prefab's | `object game:Boss does not match prefab "p.prefab.json", which is game:Hero` |
| Prefab file with no object | `prefab "p.prefab.json": no object` |
| Prefab with several objects, one without `id` | `prefab "p.prefab.json": objects[1]: a prefab with several objects needs an "id" on each` |
| Instance of a multi-object prefab without `id` | `objects[0]: instance of multi-object prefab "p.prefab.json" needs an "id"` |
| `/` in an authored `id` | `objects[0]: id "a/b" contains "/", which is reserved for the parts of prefab instances` |
| Prefab reference to an unknown object | `prefab "p.prefab.json": object "sword": component game:Follow: $.Target: unknown object id "nobody"` |
| Variant adds an id the base has | `prefab "v.prefab.json": objects[1]: duplicate id "sword" (also in base "p.prefab.json")` |
| `children` key that is no part | `objects[0]: children: prefab "p.prefab.json" has no object "x"` |
| `children` key that is the root | `objects[0]: children: "knight" is the root of prefab "p.prefab.json"; override it with "components"` |
| `children` value that is not an object or null | `objects[0]: children.sword: expected an object of components or null` |
| Removed part still referenced | `objects[0]: children: removing "sword" leaves "glint" with a reference to it` |
| Error in a part's components | `objects[0] part "sword": component gfx:Sprite: $.ID: ...` |
| `children` without `prefab` | `objects[0]: "children" needs a "prefab"` |
| `Serialize`: part without its root | `objects[3]: part "sword" of prefab "p.prefab.json" has no root object "k1"` |
| `Serialize`: part with the wrong ID | `objects[3]: part "sword" of prefab "p.prefab.json" needs the ID "<root ID>/sword", has "k1/axe"` |
| Variant or nesting cycle | `prefab "a": prefab cycle: a -> b -> a` |

A `Deserialize` error leaves the world unchanged: entities created earlier in the same
call are deleted. Errors of different objects are reported together. `Serialize` never changes the world.
