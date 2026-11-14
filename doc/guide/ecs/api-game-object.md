# Game Object API

This page covers the runtime API of generated Game Objects. A Game Object is a Go struct whose fields are components. `weakpixel gen` generates a typed view for it on top of the [generic](https://github.com/weakpixel/ecs/blob/main/doc/api-generic.md) and [low-level](https://github.com/weakpixel/ecs/blob/main/doc/api-low-level.md) APIs of the [ECS](https://github.com/weakpixel/ecs). Game Objects are not a separate storage model: every object is an ordinary entity holding ordinary components.

For the JSON schema and (de)serialization see [game-object-definition.md](game-object-definition.md).

Contents: [Declaring objects](#declaring-objects) · [Generating](#generating) · [Documentation](#documentation) · [View](#view) · [Optional and value components](#optional-and-value-components) · [Embedding](#embedding) · [Rules](#rules)

## Declaring objects

Markers are whole `//` comment lines.

| Marker | On | Meaning |
| --- | --- | --- |
| `+game:object` | struct type | Generate a view for this struct. Accepts `:group=g,class=c` to override its [identity](game-object-definition.md#identity). |
| `+game:component` | field | The field is a component. `*T` is read/write; `T` is a read-only value component. No arguments: the identity belongs to the type. |
| `+game:component` | struct type | Register the type as a component (`CID_<Type>`, in the GOD and in `types.Components()`) even if no object uses it. Accepts `:group=g,name=n` to set its [identity](game-object-definition.md#identity) `group:name` (defaults: the package's group and the Go type name). |
| `+game:optional` | pointer field | The component may be absent. |
| `+game:embed` | field | Flatten another `+game:object` into this one. |
| `+game:asset:kind=k` | `json:"-"` field of a component struct | The field is an asset handle; in the schema and serialized GOD it is a key string (`format: "asset"`, `x-asset: k`). |
| `+game:runtime` | `json:"-"` field of a component struct | Runtime state: in the schema (`x-runtime`), never serialized, rejected on input. |
| `+game:enum` | named integer type | The type's exported constants are its only values: the schema lists them as `x-enum` (value, name, doc), `Validate` rejects other values and the editor shows a dropdown. The JSON value stays the integer. See [Enums](game-object-definition.md#enums). |
| `+game:default=v`, `+game:default:path=v,...` | field of a component struct (or of a struct nested in one) | The value the editor writes when it creates the component. `v` is a Go constant expression of the field's type: `+game:default=1.5`, `+game:default=BlendAdditive`, `+game:default=gfx.White`, a slice as a list `+game:default={1, 2, 3}`. Struct leaves and map keys are set by dotted path: `+game:default:X=1,Y=1`, `+game:default:walk=2,run=4`; the marker may repeat. Editor only: decoding still gives the Go zero value. See [Defaults](game-object-definition.md#defaults). |
| `+game:editor:name="..",min=..,max=..,step=..,unit=..,order=..,color=true,hidden=true,showIf=..` | field of a component struct (or of a struct nested in one) | Hints for how an editor shows the field; every argument is optional. They land in the schema as `x-editor` and change nothing about decoding or validation. See [Editor hints](game-object-definition.md#editor-hints). |
| `+game:group:name=g` | package doc comment | Default group of the package's objects and component types, and the name of the package's GOD file in the specs folder (`<g>.god.json`). |

```go
// +game:object
type Common struct {
    // +game:component
    Transform *Transform
    // +game:component
    State *State
}

// +game:object
type Hero struct {
    // +game:embed
    Common

    // +game:component
    Health *Health

    // +game:component
    // +game:optional
    Weapon *Weapon

    NotAComponent string // unmarked fields are ignored
}
```

`+game:asset` and `+game:runtime` apply to direct, exported, `json:"-"` fields of a component struct only (a field cannot have both, and `+game:asset` needs a `kind` and a non-pointer struct type); they change the schema, not the generated Go. Rules and behavior are in [game-object-definition.md](game-object-definition.md#asset-fields).

Each component type may appear only once per object (after flattening). A component type used by an object needs a group (on the type or its package), so data files never hold an import path. The generator reports every violation at once, with `file:line:col`. Identities only matter for serialization, Lua and the editor; see [game-object-definition.md](game-object-definition.md#identity).

This table is the one list of markers; [game-object-definition.md](game-object-definition.md) covers what they mean for the JSON forms.

## Generating

```go
//go:generate go run github.com/weakpixel/engine/tools/weakpixel gen
```

`go generate` writes `zz__gen.go` into the package, and the package's GOD file into the project's specs folder (`god.specs` of [`.weakpixel.json`](../project-file.md), as `<group>.god.json`) or, without one, `zz__gen.god.json` next to the sources. Do not edit them by hand. Everything it uses from `ecs` is exported API, so it works from any module (run `go mod tidy` after adding the directive; the generator needs `golang.org/x/tools`).

It generates:

- `CID_<Type>` component IDs, registered at package init (`CID_ExtHitbox` for `ext.Hitbox` from another package);
- `New<Object>View(world)` and the view methods below, for every `+game:object`.

## Documentation

Go doc comments on objects, their component fields, component types, component fields and the constants of `+game:enum` types are copied into the package's GOD file (without the `+game:*` lines; see [Documentation fields](game-object-definition.md#documentation-fields)). `weakpixel doc` turns those files into one HTML or Markdown document: see [Tools](../tools.md#doc).

## View

```go
world := ecs.NewWorld()
heroes := gametypes.NewHeroView(world)
```

`New<Object>View` returns an `<Object>View` **value** bound to one `World`; it holds no entity state. Create it once and keep it — copies share the same underlying state, like [`ViewN`](https://github.com/weakpixel/ecs/blob/main/doc/api-generic.md#views). It has the same method set as a `ViewN`, with the object struct in place of `ComponentsN`:

| Method | Description |
| --- | --- |
| `NewEntity(obj T) ecs.Entity` | Creates an entity from `obj`. A nil required pointer component is stored as the zero value. An optional component is added only when non-nil. |
| `Get(e) T` | Returns a `T` whose component fields point to the live components. Optional fields are resolved (`nil` when absent). Panics if `e` lacks a required component. |
| `Has(e) bool` | `true` when `e` has every required component, i.e. `Get` would not panic. Panics if `e` is stale. |
| `Set(e, obj T)` | Writes `obj` to the entity (see [Set](#set)). |
| `Remove(e)` | Removes all of the object's components (required and optional) that `e` has, in one archetype move. Other components stay. |
| `Each() iter.Seq2[ecs.Entity, T]` | Yields every entity that has all required components (see [Each](#each)). |

```go
e := heroes.NewEntity(gametypes.Hero{
    Common: gametypes.Common{Transform: &gametypes.Transform{X: 1, Y: 2}},
    Health: &gametypes.Health{Current: 100, Max: 100},
})

h := heroes.Get(e)
h.Health.Current -= 10 // writes to the world: fields are live pointers

heroes.Set(e, gametypes.Hero{
    Common: gametypes.Common{Transform: &gametypes.Transform{X: 5}},
    Health: &gametypes.Health{Current: 50, Max: 100},
    Weapon: &gametypes.Weapon{Damage: 3}, // adds the optional component
})

if heroes.Has(other) { // "is other a Hero?"
    heroes.Get(other).Health.Current = 0
}
```

### Set

`Set` is a full write of the object:

- required components are added or replaced; a nil required pointer **overwrites the component with its zero value**;
- optional components are added or replaced when non-nil and **removed when nil**;
- components of the entity that are not part of the object are left untouched.

If the entity needs no structural change (it already has every component, no optional added or removed) the write happens in place.

### Each

```go
for e, hero := range heroes.Each() {
    hero.Transform.X += 1 // live pointer
    if hero.Weapon != nil {
        hero.Weapon.Ammo--
    }
    _ = e
}
```

- Yields every entity that has **all required components** of the object, regardless of how it was created. An entity of a richer object (e.g. a `Boss` that has all of `Hero`'s required components) also matches.
- Optional fields are always resolved, so `Each` yields the same `T` that `Get(e)` returns for the same entity. That costs one lookup per optional component per entity.
- The yielded `T` is a fresh value per entity; its component pointers point into the world.

## Optional and value components

- **Required `*T`**: always present on entities created through the view. `Get` panics if the entity lacks it.
- **Optional `*T`** (`+game:optional`): `nil` when absent. `Get` and `Each` always resolve optionals.
- **Value `T`** (no pointer): read-only. It is copied out by `Get` and `Each` and stored by `NewEntity` and `Set`; writing to the field of the returned object is not persisted. Cannot be optional.

## Links between objects

A component field of type `ecs.Entity` is a link to another object. It is a plain field in Go (an `ecs.Entity` handle, valid while that entity lives); in a level file it is the other object's `"id"`, so links survive save and load. No marker is needed. See [Entity references](game-object-definition.md#entity-references).

## Embedding

`+game:embed` flattens another object into the parent. Allowed forms: `Common`, `*Common`, `Common Common`, `Common *Common`, from the same or another package (everything must be exported). The embedded object's components become components of the parent entity; there is no nested entity. Pointer embeds are allocated automatically when nil, and on every read: `Get` and `Each` allocate one embedded struct per pointer embed. Prefer value embeds in objects used in hot loops.

## Rules

- The zero `<Object>View{}` panics with `ecs: not bound to a world` on every method (`NewEntity`, `Get`, `Has`, `Set`, `Remove`, `Each`), like a zero `ViewN`.
- The view methods use the underlying [ecs rules](https://github.com/weakpixel/ecs/blob/main/doc/api-low-level.md#component-access): returned pointers are invalidated by structural changes to the world (`Set` that adds or removes a component, `NewEntity`, `Remove`, `DeleteEntity`). Re-`Get` afterwards.
- During `Each`, writing through pointers and in-place `Set` are allowed; structural changes (creating or deleting entities, adding or removing components, including an optional change via `Set`) panic. Collect entities first and apply changes after the loop.
- `Get` and `Each` don't allocate unless the object has pointer embeds.
- Use the view for the whole object and `ecs.GetComp`/`SetComp` or a [`ViewN`](https://github.com/weakpixel/ecs/blob/main/doc/api-generic.md#views) for a few components; they operate on the same entity, so they mix freely. Delete entities with `world.DeleteEntity`.
