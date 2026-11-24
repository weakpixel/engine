# Input design

How `input` and its two companion packages are built, and why.
The API is in the guide: [input.md](../guide/engine/input.md).

## Requirements

- Game code reads **actions**, never devices; which control drives an action is data, per player, rebindable.
- Several players on one machine, with pads plugged in and out while the game runs.
- Keyboard and mouse, gamepads (standard layout and raw), touch.
- Bad bindings files report every problem with a path, like GOD documents.
  Programmer errors panic.
- `Router.Update` and every state query are **0 allocs**: they run every tick.
- Testable and replayable without Ebiten or a display.
- Input is read once per tick, before game systems, so every system in a tick sees the same state.
  `Draw` never reads input.

## Packages

| Package | Imports | Why |
| --- | --- | --- |
| `input` | stdlib only | Everything that decides: actions, files, compile, router, evaluation, profiles, rebinding, names, recording. No Ebiten, so the whole logic is tested with `ManualSource` and replayed with `ReplaySource`. The bindings asset (loader, handle, store) is in `input/inputsys`, so this package needs no asset system |
| `input/ebiteninput` | Ebiten, `input` | The `Source`: polls Ebiten into an `input.Frame`. The only Ebiten import of the input system |
| `input/inputsys` | `ecs`, `ecs/system`, `asset`, `gfx`, `render`, `input` | ECS and asset glue: `InputSystem` (resource, update), the bindings asset (`RegisterLoaders`, `LoadBindings`, `BindingsStore`) and `TouchControlsExtract` (draw items) |

The proposal put the glue in `gfx`; it moved to `inputsys` because `gfx` is plain-data components and must not import `render`.
`gfx` only holds the `PlayerSlot` component.

## Frame, then evaluate

A `Source` fills one `Frame` per tick: plain arrays (key bitset, mouse, up to 8 pads, up to 10 touches) and the device changes.
Everything after that reads only frames, so:

- **Edges are computed, not polled.**
  `JustPressed` compares this tick's accumulated value with last tick's. Replays, fake sources and blocked contexts share one code path, and a context that becomes active cannot see a stale "just pressed".
- **Recording is copying frames.**
  A replay feeds the same frames to a router with the same bindings and gets the same states.
  Idle ticks are stored as a repeat count.
- Unfocused frames are blurred (every control up) by the router, not the source.

## Compile once, evaluate flat

`ParseBindings` first walks the JSON against the Go file model (`checkJSON`, reflection over the `json` tags) so unknown properties and wrong types are reported with paths, all at once; `encoding/json` alone stops at the first problem.
Then every binding compiles to a fixed-size struct: up to four 4-byte `Control`s, a modifier bitmask, a small processor slice, one interaction, a threshold.
Bindings of a mapping are contiguous and sorted by modifier count, so `Ctrl+S` is evaluated before `S` and consumes the key.

Per tick and player, evaluation is a loop over the active mappings' bindings: read controls from the frame, apply processors, run the interaction, accumulate per action, then `commit` computes edges.
All per-player buffers (accumulators, binding states, consumed list, mapping order) are sized when bindings are built and reused, which is what makes `Update` 0 allocs.
The mapping order is an insertion sort over the stack (a handful of names) for the same reason.

Decisions inside evaluation:

| Decision | Why |
| --- | --- |
| Buffered presses are a tick number per action, set in `commit` | `PressedWithin` is one subtraction and replays stay exact; a counter starting at 1 lets 0 mean "no press" (or consumed) without a flag. Kept across mapping changes, since it is history read by game code, not binding state |
| Fresh-press rule for **Button** bindings only | A key held when a context appears must not fire it (the Escape that opened the menu, the Space that closed it). Axis and Vec2 pass through: walking must continue when a menu closes |
| Reached state kept **by mapping name** across rebuilds | Rebinding or a reloaded file must not count as "the mapping became active" and release held actions |
| Consumption by control, modifier keys collapsed to their group | Higher bindings shadow lower ones on the same key, across mappings too, but `Ctrl+S` does not block `Ctrl+A` |
| `repeat` gates Axis and Vec2 values to the pulse ticks | Menu navigation from a held stick or arrow key without a separate Button action |
| Unknown mapping names stay on the stack | Games push context names before the bindings asset is loaded |
| Processors and interactions are objects with one key, not strings or unions | A `TypeSchema` can describe them, so the schema is generated from the Go types |

## Profiles store the diff

A profile records overrides keyed by `(mapping, action, class, slot, part)`, not a copy of the bindings.
Applying it to new defaults (`applyProfile`) rebuilds the effective bindings; an override that no longer fits is skipped and reported, never fatal.
So a game that changes a default reaches every player who did not touch that action, and a removed action costs a log line.

Slots are numbered per class (`kbm`, `gamepad`, `touch`) because an options screen shows "keyboard 1, keyboard 2, gamepad 1" columns; the proposal's `keyboard` became `kbm` since mouse buttons share it.
Composite parts are a separate `part` field rather than `move.up`, because action names may contain dots (`ui.move`).

## Devices and players

Devices are IDs: keyboard, mouse and touch are fixed, pads start at 16.
Each player owns a device set; a per-tick `devSet` view of the frame masks out other players' devices, so the same compiled bindings serve every player.
With `AssignJoin`, unowned devices drive a lobby player that has its own stack.
A pad lost while assigned keeps its slot and is matched back by GUID and name.

## Schemas

`input/schema/*.schema.json` are `god.TypeSchema` documents generated from the Go file model by a test (`go test ./input -run TestSchemas -update`), which also validates the test data against them.
The `input` package itself never imports `god`.

## Not built yet

- Hot reload of anything but the bindings file: `InputSystem.Watch` follows `Store.Version`, which [asset hot reload](asset-system.md#hot-reload) bumps when the file changes.
- Recording on by default in development builds.
- A prompt-drawing extract and glyph atlas (names and glyph keys exist).
- Lua access to the router; text entry.
