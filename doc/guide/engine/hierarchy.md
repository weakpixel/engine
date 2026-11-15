# Hierarchy: attach entities to a parent

Package `hierarchy` lets an entity follow another one. A child holds one component, `hierarchy.Parent`, with the parent entity and a `Local` placement. Each tick the engine writes the child's world `gfx.Transform` and, by default, its draw layer from the parent chain. A weapon in a hand, a shadow under a sprite, a health bar over an enemy and a particle effect on a torch need nothing else.

- [Attach in code](#attach-in-code)
- [Attach in a level](#attach-in-a-level)
- [Placement](#placement)
- [Absolute children](#absolute-children)
- [Draw layer](#draw-layer)
- [Y-sorted layers](#y-sorted-layers)
- [Deleting](#deleting)
- [When it runs](#when-it-runs)
- [Panics and errors](#panics-and-errors)
- [Limits](#limits)

## Attach in code

```go
sword := ecs.NewEntity2(w, gfx.Transform{}, gfx.Sprite{ID: swordImg})
hierarchy.Attach(w, sword, hero, false) // jumps to the hero
ecs.GetComp[hierarchy.Parent](w, sword).Local.Pos = gfx.Vec2{X: 8, Y: -4}
```

`Attach(w, child, parent, keepWorld)` adds a `gfx.Transform` to the child if it has none, then adds or changes its `Parent`. With `keepWorld` the `Local` is computed so that the child stays where it is (it reads the parent's `Transform` as of the last tick); without it the child keeps its `Local` (zero for a new attachment) and moves to the parent. The first placement sets `PrevPos` too, so the jump is not interpolated.

| Function | Does |
| --- | --- |
| `Attach(w, child, parent, keepWorld)` | Makes `child` a child of `parent`. Not inside an `Each` over the child (it changes the archetype). |
| `Detach(w, child)` | Removes the `Parent`. The `Transform` already holds the world placement, so the child stays where it is. |
| `Delete(w, e)` | Deletes `e` and its descendants now. Not inside an `Each`. |
| `Children(w, e)` | The entities directly attached to `e`. It scans every `Parent`: for tools, not for ticks. |
| `WorldOf(w, e)` | The world `Transform` (`Pos`, `Rot`, `Scale`) the system would write for `e` now, composed from its chain; an entity without a `Parent` gets its own. For tools that read placement before the next tick, such as the level editor framing objects it just spawned. `ok` is false for a dead link or an entity with neither component. |
| `CheckCycles(w, ents, name)` | Joins an error per parent cycle (or chain deeper than `MaxDepth`) that starts at `ents`. The engine calls it for every document. |
| `Compose(parent, local)` / `LocalOf(parent, world)` | The world `Transform` (`Pos`, `Rot`, `Scale`) of a child placed at a `Local` under a parent's world `Transform`, and the inverse: the `Local` that puts a child at a world `Transform`. The system and `Attach` use them; tools such as the level editor use `LocalOf` to turn a dragged world placement into a `Local`. A parent scale axis of 0 cannot be inverted: that local axis is 0. |

## Attach in a level

`hierarchy:Parent` is a registered component, so any object of a GOD level or prefab can carry it ([extra components](../ecs/game-object-definition.md#extra-components)). `Entity` is an [entity reference](../ecs/game-object-definition.md): the `id` of an object of the same document. Children and parents may come in any order in the file.

```json
{"id":"sword","group":"gfx","class":"Visual","components":{
  "gfx:Transform":{"Pos":{"X":0,"Y":0},"PrevPos":{"X":0,"Y":0},"Rot":0,"Scale":{"X":1,"Y":1}},
  "gfx:Sprite":{"ID":"items/sword.png","Layer":0,"...":"..."},
  "hierarchy:Parent":{"Entity":"hero","Local":{"Pos":{"X":8,"Y":-4},"Rot":0,"Scale":{"X":0,"Y":0},
    "PlaceMode":0,"LayerOffset":1,"LayerMode":0,"YSortSide":0}}}}
```

The serialized `Transform` of a relative child is derived data (for an [absolute child](#absolute-children) it is too, until its first placement): the engine overwrites it, so what the file holds there does not matter. The children are in place in the tick in which the document spawns, before the first game system runs. A document with a parent cycle does not spawn: it fails like any invalid document, with an error that names the object ids, and spawns nothing.

## Placement

`gfx.Transform` stays in world space, so every system that reads it (extract, collision, physics, camera) works unchanged. The `Local` is the child's placement relative to its parent:

| `Local` field | Meaning |
| --- | --- |
| `Pos` | Offset in the parent's space: scaled by the parent's world `Scale`, rotated by its `Rot`. |
| `Rot` | Added to the parent's `Rot`, in radians. Under a mirrored parent (one negative scale axis) it is subtracted, so the child turns the way it looks. |
| `Scale` | Multiplies the parent's `Scale`. Zero means (1, 1), as for `Transform`. |
| `PlaceMode` | `PlaceRelative` (default) or `PlaceAbsolute`, see [Absolute children](#absolute-children). |
| `LayerOffset`, `LayerMode`, `YSortSide` | See below. |

The system walks from each child up to the root (the first ancestor without a `Parent`) and composes the `Local`s onto the root's `Transform`. It never reads an intermediate ancestor's `Transform`, so the order in which entities are visited does not matter, and a chain can be spawned in any order. The cost is children times depth, with no allocations (`TestSystemAllocs`). A root without a `Transform` counts as a zero `Transform`. A chain deeper than `hierarchy.MaxDepth` (32) panics.

## Absolute children

With `PlaceMode: PlaceAbsolute` a child is placed from its `Local` once, as a relative child would be, and then keeps its own `Transform`: the parent can move, turn and scale without taking it along. A dropped sword that still goes when its owner is deleted, a footprint, or a decal left by a moving actor need this. The first placement happens when the child spawns (a level or prefab instance starts its absolute parts relative to where the instance is placed), after `Attach`, and after `Reposition`; like any first placement it sets `PrevPos` too.

- **Its `Transform` is its own.** Move, tween or simulate it like a root. A body on an absolute child is not a tick late.
- **`Local` is not written.** It stays the authored placement relative to the parent, not a live offset. Code that needs the current offset calls `LocalOf(parent world, child Transform)`.
- **It is the root of its own children.** A relative child of an absolute one follows it, not the root above. The walk stops at the absolute link once it is placed and reads its `Transform`, so visiting order still does not matter.
- **Lifetime and layer still follow the parent.** It is deleted with its parent, and `LayerMode` applies as for any child.
- **Y-sort by itself.** It and its relative children sort by its own Y instead of the root's ([Y-sorted layers](#y-sorted-layers)).

Switch the mode with `SetPlace` rather than by writing the field, when the child should not jump:

```go
hierarchy.SetPlace(w, sword, hierarchy.PlaceAbsolute, false) // drop: stays where it is
hierarchy.SetPlace(w, sword, hierarchy.PlaceRelative, true)  // pick up: Local recomputed, no jump
hierarchy.SetPlace(w, sword, hierarchy.PlaceRelative, false) // back to the hand: moves to its Local
```

| Function | Does |
| --- | --- |
| `SetPlace(w, child, mode, keepWorld)` | Sets `Local.PlaceMode`. Switching to `PlaceAbsolute` keeps the child where it is. Switching to `PlaceRelative` with `keepWorld` computes `Local` (`Pos`, `Rot`, `Scale`) from the parent's `WorldOf` and the child's `Transform`, so the child stays put; without it the child moves to its `Local` in the next run. In place, no archetype move. |
| `Reposition(w, child)` | Places the child from its `Local` again in the next run, in either mode; for an absolute child it moves it back next to its parent. Sets `PrevPos` like a first placement. |

## Draw layer

With the default `LayerRelative`, the layers of the child's `Sprite`, `NineSlice`, `Label` and `Particles` (whichever it has) are set to the parent's layer plus `LayerOffset`, so a sword with offset 1 draws just over the hero wherever the hero's layer is moved to. The base is the layer of the nearest ancestor with a drawn component (its `Sprite`, else `NineSlice`, `Label`, `Particles`), plus the offsets of the relative ancestors between them. An ancestor that draws nothing (an empty group) passes the base on, and a chain without any drawn ancestor has base 0. With `LayerAbsolute` the child's layers are left alone. The write is in place: no archetype move.

## Y-sorted layers

In a [`YSort` layer](rendering.md#draw-order) a child sorts by the Y of its **root** (or of its nearest [absolute](#absolute-children) ancestor, or its own Y if it is absolute), nudged by 1/1024 world units to the side `YSortSide` says: `YSortInFront` (default) draws it just over the root, `YSortBehind` just under it. Another actor draws over or under the whole group, depending only on its Y against the root's, however far the child sits from the root. The key applies to sprites and nine-slice panels; particles and labels keep their own `SortY`. In layers without `YSort` it is ignored.

## Deleting

Delete the parent and its children go too: the system deletes every entity with a dead link in its chain, grandchildren included, in the same run. `Level` unloading and `hierarchy.Delete` therefore remove whole subtrees. A child whose parent is deleted by the `Commands` of a stage is deleted in the next hierarchy run.

## When it runs

The `hierarchy` stage sits between `post` and `anim` and is **not** gated by a Tool pause or `Hold`, so in the editor's edit mode, children move with their parent. Right after a document spawns objects the assets stage runs it once more, before the snapshot. See [engine.md](engine.md#tick-order).

## Panics and errors

| Message | When |
| --- | --- |
| `hierarchy: cycle or chain deeper than 32 at <entity>` | The system meets a cycle or too long a chain (a programmer error at runtime; documents are checked on load). |
| `hierarchy: attach would create a cycle: <child> under <parent>` | `Attach` of an entity under itself or under one of its descendants. |
| `hierarchy: SetPlace: <child> has no Parent` | `SetPlace` of an entity without a `Parent`. |
| `hierarchy: Reposition: <child> has no Parent` | `Reposition` of an entity without a `Parent`. |
| `hierarchy: parent cycle a -> b` (error) | A document loads a parent cycle. |

## Limits

- **Physics and collision run before the hierarchy.** A collider or body on a child sees the placement of the last tick, one tick late. Put bodies on roots or [absolute children](#absolute-children); give a body a child for its visuals, not the other way round.
- **The camera target** and anything else that reads a child's `Transform` before the `hierarchy` stage sees last tick's value, one tick late.
- **`Sprite.FlipX` and `FlipY` are not inherited.** A parent's mirror comes from a negative `Scale`.
- **Tween `Local`, not `Transform`.** A tween on a relative child's `Transform` is overwritten every tick; animate `Parent.Local`. An absolute child is the other way round: tween its `Transform`, since its `Local` is only read at placement.
- **Non-uniform scale with rotation is approximate**: there is no shear, as in most 2D engines. The offset is scaled and then rotated, and the child's own scale and rotation stay independent.
- **Prefabs:** a prefab file may hold a root with parts attached through `Parent` ([game objects](game-objects.md#prefabs-with-several-objects)).
- **Editor:** the level editor's gizmos edit the `Local` of an attached object ([editor](editor.md)). **Not built:** a nested object list and drag to re-parent in the editor, Lua bindings, inheriting `Hidden`, ordered children.
