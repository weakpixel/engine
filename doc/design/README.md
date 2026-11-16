# Design

How the Weakpixel engine is built and why. These pages are for people changing the engine; to *use* it, read the [guide](../README.md#guide). Design pages don't repeat API signatures: they link to the guide page that owns them.

| Page | Covers | Status |
| --- | --- | --- |
| [architecture.md](architecture.md) | Modules, packages (ECS and Engine Runtime), dependency rules, cross-cutting conventions (panics, goroutines, allocations, generated code, tests) | Built |
| [game-objects.md](game-objects.md) | The `+game:*` generator pipeline, what it generates, GOD versioning and the `god` runtime | Built |
| [asset-system.md](asset-system.md) | How the engine uses the asset module: concrete handles, authored asset keys, hot reload sources, the asset library | Built; asset library, audio and tilemap extras not yet |
| [input.md](input.md) | Actions, compiled bindings, frames and edges, contexts, the profile diff, devices and players | Built; prompt drawing not yet |
| [editor.md](editor.md) | The level editor: one core (`editor.Session`) with a JSON bridge, the document owning undo, applying by id, the protocol; the tool shell of `wpix-editor` | Core, bridge and VSCode extension (viewport, inspector, outline) built; tool shell, `view`, `anim`, `fx`, `sheet` and `voice` tools built (the extension side of `anim`, `fx` and `sheet` is not) |
| [engine-renderer.md](engine-renderer.md) | Components, extract systems and an ecs-free renderer; the frame contract; animation; particles, tweens, the startup screen, rooms, scenes, collision detection and physics | First version built; render targets, tilemaps not yet |

## Where else design lives

- **The ECS core** (registry, entities, archetype storage, structural changes, the revision counter, views, resources) is designed in its own repository: [weakpixel/ecs design](https://github.com/weakpixel/ecs/blob/main/doc/design.md).
- **Doc comments** in the code record the contract of each identifier and explain non-obvious performance tricks next to them.
- **`plan/`** (not committed) holds the working plans of individual changes. Decisions that outlive a plan are copied into these pages.
- **Contributor rules** for working in the repository (workflow, hard rules, conventions) are in [CLAUDE.md](../../CLAUDE.md).

## Keeping this section honest

A design page describes what the code does, plus a clearly marked "Not built yet" list. When a change makes a page wrong, fix the page in the same change. Proposals that are not built at all stay out of the repository until they are planned.
