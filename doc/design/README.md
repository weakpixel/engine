# Design

How the Weakpixel engine is built and why.
These pages are for people changing the engine; to *use* it, read the [guide](../README.md#guide).
Design pages don't repeat API signatures: they link to the guide page that owns them.

Each page holds one topic, so read this table and then only the page you need.
Pages stay under 16 KB and hold one sentence per line (`TestDesignPageBudget` in `internal/deps`); a topic that outgrows its page gets a page of its own.

| Page | Read it when you change | Status |
| --- | --- | --- |
| [architecture.md](architecture.md) | Modules, root packages, dependency rules, cross-cutting conventions (panics, goroutines, allocations, generated code, tests) | Built |
| [packages.md](packages.md) | What one Engine Runtime package does and may import | Built |
| [game-objects.md](game-objects.md) | The `+game:*` generator pipeline, what it generates, GOD versioning, the `god` runtime | Built |
| [asset-system.md](asset-system.md) | How the engine uses the asset module: concrete handles, authored keys, hot reload sources, the asset library | Built; mods, localized variants, music loop points not yet |
| [input.md](input.md) | Actions, compiled bindings, frames and edges, contexts, the profile diff, devices and players | Built; prompt drawing not yet |
| [engine-renderer.md](engine-renderer.md) | Components, extract systems and the ecs-free renderer: frame contract, sorting, batching, stats, canvases, nine-slice panels | Built; several cameras, variable timestep not yet |
| [render-effects.md](render-effects.md) | Sprite shaders, layer groups, post-processing | Built |
| [simulation.md](simulation.md) | Particles, tweens, the attachment hierarchy | Built |
| [physics.md](physics.md) | Collision detection and physics | Built |
| [scenes.md](scenes.md) | The scene stack, room transitions, the startup screen | Built |
| [editor.md](editor.md) | The level editor core (`editor.Session`): host and stages, attached objects in the viewport, applying documents, modes | Built |
| [editor-protocol.md](editor-protocol.md) | A message between the level editor and its host (`editor/bridge`, the extension's `controller.ts`) | Built, version 0 |
| [editor-extension.md](editor-extension.md) | How the VSCode extension hosts the editor and the tools | Built |
| [tool-shell.md](tool-shell.md) | The `wpix-editor` shell: kind table, tool contract, panels, documents, the tool protocol | Built |
| [editor-tools.md](editor-tools.md) | The `fx`, `anim` and `sheet` tools | Built |
| [previewers.md](previewers.md) | The `view` tool: textures, atlases, shaders, Tiled files, voices and dialogues | Built |

## Where else design lives

- **The ECS core** (registry, entities, archetype storage, structural changes, the revision counter, views, resources) is designed in its own repository: [weakpixel/ecs design](https://github.com/weakpixel/ecs/blob/main/doc/design.md).
- **Doc comments** in the code record the contract of each identifier and explain non-obvious performance tricks next to them.
- **Plans** (not committed) hold the working plans of individual changes.
  Decisions that outlive a plan are copied into these pages.
- **Contributor rules** for working in the repository (workflow, hard rules, conventions) are in [CLAUDE.md](../../CLAUDE.md).

## Keeping this section honest

A design page describes what the code does, plus a clearly marked "Not built yet" list.
When a change makes a page wrong, fix the page in the same change.
Status lives in the table above, not in dated remarks on the pages.
Proposals that are not built at all stay out of the repository until they are planned.
