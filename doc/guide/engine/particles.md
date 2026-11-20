# Particles

`particles` simulates and draws particle effects: sparks, dust, embers, smoke. An effect is an asset, a `*.fx.json` file ([the format](#the-effect-file)), and an entity plays it with a `gfx.Particles` component that holds the effect's handle, as `gfx.Animator` holds a clip. An entity with a `Transform` and a `Particles` spawns, moves, fades and removes particles every tick, and they are drawn as ordinary batched sprites ([batching](rendering.md#batching)). `game.Engine` runs it; there is nothing to wire.

```go
sparks := particles.LoadEffect(sc, "global/fx/sparks.fx.json")
ecs.NewView2[gfx.Transform, gfx.Particles](w).NewEntity(
	gfx.Transform{Pos: pos, PrevPos: pos},
	gfx.Particles{Effect: sparks, Layer: 5},
)
```

`LoadEffect` never blocks: an entity whose effect is still loading (or failed, or is the zero handle) has no particles and starts when the effect is ready. `particles.LoadEffectSync` waits, for boot code and tests; `particles.EffectStoreOf(srv)` resolves handles. See [asset kinds](asset-kinds.md#effect).

## What a particle is

Particles are **not entities**. They live in pools owned by `particles.System`, one pool per `Particles` entity, so thousands of short-lived particles cost no archetype moves and no deferred deletes. The component stores only a runtime pool id (`Pool`) and counters. A pool is released in the tick after its entity is deleted or loses the component, and its memory is reused by the next effect. A cloned entity (copied `Pool` id) gets a pool of its own.

Each tick, in the `anim` stage after the `AnimationSystem`, the system moves the live particles (explicit Euler on the fixed tick delta), removes the dead ones, and spawns new ones. The `anim` stage runs while a [Tool](editor.md) pauses the game, so effects play live in the editor. `particles.Extract` runs in `render-prepare` right after the `SpriteExtract` and queues one `KindSprite` item per live particle. Particles keep their previous position, so drawing is interpolated like sprites. Those outside the camera bounds are culled and counted in `Stats.Culled`.

## The component

`gfx:Particles` is a [Game Object component](game-objects.md); `gfx:ParticleSource` is the object class that holds a `Transform` and a `Particles`, so a level can place a bare effect. Dropping a `*.fx.json` file on a level creates one. The component holds what differs per placed instance, and nothing else:

| Field | Meaning | Zero means |
| --- | --- | --- |
| `Effect` | The effect (asset key in GOD, kind `effect`) | nothing plays |
| `Layer` | Draw order, as `Sprite.Layer`; give additive effects a layer of their own | layer 0 |
| `Stop` | Stop spawning; live particles finish | keep spawning |
| `Seed` | Random seed | derived from the entity |
| `Pool`, `Time`, `Live`, `Done` | Runtime state, never serialized: pool id, seconds into the cycle, live particle count, finished | |

Two torches that differ in more than `Layer`, `Stop` and `Seed` use two effect files. `Layer` is not part of the effect because the same effect sits on different layers in different levels.

### Overrides

A placed effect can override single values of its file. With several emitters in the file an override applies to every one of them (per-emitter overrides are not built). `gfx:Particles` has an optional field for every emitter value, named like the `Emitter` field in Go (`Rate`, `Color`, `LifeMin`, `Angle`, `Shape`, ...), plus `Sprite` and `Shader` for the image and the shader. A level lists only what it overrides:

```json
"gfx:Particles": { "Effect": "fx/torch.fx.json", "Layer": 5, "Rate": 30 }
```

This torch spawns 30 per second. Every value it does not override still follows the file, hot reloads included: recoloring the file recolors it, a new `rate` in the file does not change it. Removing the key returns to the file's value at once.

- Values are in the component's units, like every component in a level: radians, packed `0xRRGGBBAA` colors, enum numbers (`Blend`, `Shape`). The file keeps its readable units.
- In Go the fields are pointers, nil meaning "from the effect": `c.Rate = gfx.Override(float32(30))`. The pointee is immutable because clones share it: replace the pointer, never write through it.
- `Sprite` and `Shader` are asset handles; the zero handle means "from the effect", so an instance cannot override back to the placeholder or to no shader.
- `particles.Resolve(e, c)` returns the emitter an entity plays for the effect emitter `e`. The system resolves it every tick (a copy, no allocation), so overriding one of the cycle keys listed under [Hot reload](#hot-reload) restarts the pool like a file edit does.

`Done` is set when a non-looping effect (or one with `Stop` set) has no live particle left. A one-shot effect deletes its entity then. Replay a finished effect by zeroing `Time` and `Done`.

The random numbers come from a PCG per pool, seeded from `Seed` or the entity, so the same level and inputs give the same particles. That keeps frame-log comparisons meaningful.

## The effect file

`*.fx.json` is JSON with readable units. Unknown keys and bad values are load errors, all reported together with the path of each value (`emitters[0].blend: unknown name "glow", use one of ...`). The fx tool writes the whole file in this canonical form.

```json
{
  "version": 0,
  "emitters": [
    {
      "sprite": "global/fx/spark.png",
      "blend": "additive",
      "color": "#fff0a0",
      "endColor": "#ff5014",
      "scale": 1.2,
      "endScale": 0.3,
      "fadeOut": 0.4,
      "burst": 12,
      "duration": 0.05,
      "max": 16,
      "life": [0.2, 0.45],
      "spread": 91.673,
      "speed": [40, 120],
      "drag": 3
    }
  ]
}
```

`emitters` holds one to eight emitters. They play together on the entity that places the effect, each with its own cycle, burst and pool: a fire can be embers, smoke and a glow in one file. `Layer`, `Stop`, `Seed` and the [overrides](#overrides) of the component apply to every emitter; `Live` counts the particles of all of them and `Done` is set when all are done. `version` is optional and must be 0. Units are world units and seconds; angles are degrees; a pair is `[min, max]`; a vector is `[x, y]`. Every key is optional, and every zero value is sensible, so an emitter with only `sprite` and `rate` shows white, one-second particles that spawn at the entity and stay there.

| Key | Meaning | Absent means |
| --- | --- | --- |
| `name` | Label for tools; the engine ignores it | none |
| `layer` | Added to the component's `Layer` when this emitter's particles draw, so smoke can sit behind sparks. Equal layers draw in file order, the first below | 0 |
| `sprite` | Particle image (asset key from the asset root) | the placeholder |
| `shader`, `params` | Kage shader for every particle and its `Params` vec4 (an array of 4 numbers): see [shaders](shaders.md#particles) | none |
| `blend` | `normal`, `additive`, `multiply`, `screen`, `subtract`, `lighten` or `erase` (see [kinds](kinds.md#blend-modes-renderblend)) | `normal` |
| `color`, `endColor` | Tint at birth and at death, `#rrggbb` or `#rrggbbaa`, blended linearly. `#00000000` is refused: the zero color means white | white; no `endColor` keeps `color` |
| `scale`, `endScale` | Size at birth and at death | 1; no `endScale` keeps `scale` |
| `fadeIn`, `fadeOut` | Fractions (0 to 1) of the life over which alpha ramps up and down | no fade |
| `rate` | Particles per second (a fractional rest carries to the next tick) | none |
| `burst` | Particles spawned at the start of each cycle | none |
| `duration` | Length of a cycle in seconds | forever |
| `loop` | Restart after `duration` (and burst again) | the effect ends |
| `max` | Pool capacity (up to 65535); spawns beyond it are dropped | 256 |
| `shape`, `shapeSize` | `point`, `circle` (radius is `shapeSize[0]`) or `rect` (width, height), centered on the entity | a point |
| `life` | Random lifetime `[min, max]`; a max below the min means the min | both zero: 1 s |
| `angle`, `spread` | Direction in degrees (clockwise on screen from +X, added to `Transform.Rot`) and full cone width (0 to 360) | along the entity's +X, no spread |
| `speed` | Random initial speed `[min, max]` | at rest |
| `gravity` | Constant acceleration `[x, y]`, units/s² | none |
| `drag` | Slows particles by this factor per second | none |
| `spin`, `randomRotation` | Rotation speed `[min, max]` in degrees per second and random start rotation | no rotation |
| `local` | Particles move with the entity | they stay where they spawned |

In Go, `fxfile.Decode` and `fxfile.Encode` (package `particles/fxfile`, no Ebiten) read and write the file, `fxfile.Emitter` holds one emitter in the units the simulation uses (radians, `gfx.Color`), and a loaded `particles.Effect` lists its emitters with their sprite and shader handles. The effect holds its image and shader in a scope of its own, so they live as long as the effect does. A shader that is loading or failed draws the particles normally.

### Hot reload

Editing an `*.fx.json` file (in the fx tool or a text editor) reloads the effect, and the entities that play it follow at once: a changed color, rate, speed or gravity applies to the live particles. A change that live particles cannot follow (`duration`, `loop`, `burst`, `max`, `shape`, `shapeSize`) restarts the pools that play the effect, and so does an [override](#overrides) of one of them.

## Rotation and scale

The emitter's `Transform` places each particle when it spawns:

- `Rot` turns the emission direction (`Angle` is relative to it), the spawn shape and the particle's start rotation (`RandomRotation` adds a random angle on top). A thruster or torch entity can be rotated without editing `Angle`.
- `Scale` stretches the spawn shape per axis and multiplies the particle size (`Scale`, `EndScale`). Zero means (1, 1), as for sprites. Speed, gravity and drag are not scaled.

Both apply at spawn only. A live particle keeps its direction and size when the emitter turns or scales later; with `Local`, it keeps following the emitter's position, not its rotation. `Gravity` is always in world space, so smoke rises from a tilted emitter.

## Authoring

`wpix-editor <file>.fx.json` makes and edits effect files (a file that does not exist yet starts from the default effect) with presets, pickers and a live preview ([the fx tool](editor.md#the-fx-tool)); in VSCode a `*.fx.json` opens it on a double click. A file is also easy to write by hand. Place an effect in a level by dropping the file on it or by adding a `gfx:ParticleSource` with a `gfx:Particles`; the example places `ambient-embers.fx.json` in `assets/world-1/level-1.level.json`. Effects of a game live in `global/fx/` (or the package of the level that uses them): an effect nothing references is an entry of its [asset library](asset-library.md) package, and its image and shader are loaded with it.

## Performance

- Put additive particles on a layer of their own (`Particles.Layer`): the renderer batches consecutive quads with the same texture and blend mode, and a normal sprite in between breaks the run.
- A shaded emitter draws every particle with its own call (they do not batch): keep `max` small, see [shaders](shaders.md#particles).
- Set `max` to what the effect needs. It sizes the pool and caps the work.
- A simulation tick and the extract allocate nothing once the pools exist; the system looks the effect up once per entity per tick.
- Measured numbers: [engine README](README.md#performance).
- Not built: flipbook particles, curves and gradients, collisions, sub-emitters, trails. The design decisions are in [engine-renderer.md](../../design/engine-renderer.md#particles).
