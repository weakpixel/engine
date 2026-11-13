# Input

Package `input` turns keyboard, mouse, gamepad and touch input into named **actions**. Game code reads `jump` and `move`, never keys; which control drives an action is data in a bindings file, stacked in **mapping contexts** per player and rebindable with saved per-player **profiles**.

```go
acts := &input.Actions{}
Move := acts.Vec2("move", "Walk.")
Jump := acts.Button("jump", "Jump.")

e := game.New(game.Config{FS: assets, Input: game.InputConfig{
	Actions: acts, Bindings: "input/bindings.json",
}})
e.Start(func(e *game.Engine) { e.Input().Player(0).Push("gameplay") })
e.AddSystem(game.StageInput, func(ctx *system.Context) {
	st := ecs.GetResource[input.Router](ctx.World).Player(0).State()
	if st.JustPressed(Jump) { /* ... */ }
	x, y := st.Vec2(Move)
})
```

`game.Engine` wires all of it. Wiring by hand is under [Without game.Engine](#without-gameengine).

## Packages

| Package | Job |
| --- | --- |
| `input` | Actions, bindings files, the `Router`, players, profiles, rebinding, names, recording. Standard library and the asset module only: it runs headless. |
| `input/ebiteninput` | The Ebiten `Source`: the only input code that imports Ebiten. |
| `input/inputsys` | ECS glue: `InputSystem` (updates the router, publishes resources, follows the bindings asset) and `TouchControlsExtract` (draws touch regions). |
| `gfx.PlayerSlot` | Game Object component: which player controls an entity. |

## Actions

Actions are declared in Go, once, before anything uses them: `Button` (digital), `Axis` (one float) and `Vec2` (two floats). `Freeze` ends declaration; `game.New`, `NewRouter` and `RegisterLoaders` need a frozen table (`game.New` freezes it for you).

| Option | Meaning |
| --- | --- |
| `input.Delta()` | `Vec2` only. The value is a per-tick delta (mouse movement), not a position. |
| `input.AllowMods()` | Rebinding may capture modifier groups with the control (`Ctrl+S`). |

Names are free strings; dotted names (`ui.move`) are fine. `Lookup(name)` finds an action by name.

### Reading state

`Player.State()` returns a `*State`, stable for the life of the router. Every query is 0 allocs and panics if the action's kind does not match (`input: action "jump" is Button, not Vec2`).

| Query | Kinds | Meaning |
| --- | --- | --- |
| `Down`, `JustPressed`, `JustReleased`, `HeldTicks` | Button | Level, edges and how long it is down. Edges come from comparing ticks, so they work the same for replays and blocked contexts. |
| `PressedWithin(a, n)` | Button | `a` went down in the last `n` ticks (this one included) and the press was not consumed. `PressedWithin(a, 1)` equals `JustPressed(a)`. |
| `Strength` | Button | Analog strength of the press (a trigger), 0..1. |
| `Progress` | Button | `hold` interaction progress, 0..1, for charge bars. |
| `Axis` | Axis | -1..1 (or 0..1 for a trigger). |
| `Vec2` | Vec2 | x right, y **down** (screen convention). |
| `Pointer` | - | Mouse or primary touch position in logical pixels, and whether it is valid. |
| `Touch` | - | The live geometry of the touch regions (for drawing). |

When several bindings of one action are active, a Button is down if any is, and an Axis or Vec2 takes the value with the largest magnitude.

**Input buffering.** A press made a few ticks too early still counts with `PressedWithin`; `Player.ConsumePress(a)` uses it up, so it fires once. Consuming also clears `JustPressed(a)` for the rest of the tick (later readers don't act on the same press); `Down`, `HeldTicks` and `Router.Any()` are unchanged. The record survives mapping changes and rebinding; `Listen` forgets it. At 60 TPS, 6 ticks are 100 ms.

```go
if body.OnGround && st.PressedWithin(Jump, 6) {
	vel.Y = -300
	p.ConsumePress(Jump)
}
```

`Router.Any()` merges every player: a "press any button" title screen reads it.

### The Controller pattern

Gameplay systems should not import `input` either. Give the game a component with named fields and let one system copy the action state into it:

```go
type Controller struct {
	Move       gfx.Vec2
	Jump, Dash bool // pressed this tick
}

func controllers(ctx *system.Context) { // StageInput, after the engine's InputSystem
	r := ecs.GetResource[input.Router](ctx.World)
	for _, c := range view.Each() { // View2[gfx.PlayerSlot, Controller]
		slot, ctl := c.Unpack()
		st := r.Player(int(slot.Player)).State()
		x, y := st.Vec2(Move)
		*ctl = Controller{Move: gfx.Vec2{X: x, Y: y}, Jump: st.JustPressed(Jump), Dash: st.JustPressed(Dash)}
	}
}
```

AI-driven entities fill the same component, and replays or network input can too.

## Bindings files

A bindings file holds the default bindings: mappings (contexts), each with bindings per action and optional touch regions. JSON schema: [`input/schema/bindings.schema.json`](../../../input/schema/bindings.schema.json) (GOD `TypeSchema` dialect; a test keeps it in sync with the Go types and checks the shipped files against it).

```json
{
  "version": 1,
  "groups": { "play": ["gameplay", "vehicle"] },
  "mappings": {
    "gameplay": {
      "bindings": {
        "move": [
          { "composite": "dpad", "up": "key:W", "down": "key:S", "left": "key:A", "right": "key:D" },
          { "control": "pad:leftstick", "processors": [{ "deadzone": 0.2 }] },
          { "control": "touch:stick" }
        ],
        "jump": [{ "control": "key:Space" }, { "control": "pad:south" }, { "control": "touch:jump" }],
        "save": [{ "control": "key:S", "mods": ["mod:ctrl"] }],
        "charge": [{ "control": "key:C", "interaction": { "hold": "400ms" } }],
        "pause": [{ "control": "key:Escape", "locked": true }, { "control": "pad:start", "locked": true }]
      },
      "touch": {
        "stick": { "kind": "stick", "area": [0, 0.4, 0.5, 1], "radius": 0.1, "floating": true },
        "jump": { "kind": "button", "circle": [0.88, 0.8, 0.07] }
      }
    },
    "menu": { "priority": 20, "block": true, "bindings": { "ui.back": [{ "control": "key:Escape" }] } }
  }
}
```

`input.ParseBindings(data, acts)` validates and compiles a file. It reports **every** problem at once, each with a GOD-style path: `$.mappings.gameplay.bindings.move[1].control: control "pad:south" has 1 dimension(s), Vec2 action "move" needs 2`. Compiled `Bindings` are immutable and shared between players.

### Controls

| Form | Values | Dimensions |
| --- | --- | --- |
| `key:<Key>` | Ebiten key names: `key:A`, `key:Space`, `key:ArrowUp`, `key:ShiftLeft`, `key:Digit1`, `key:F1`, ... | 1 |
| `mouse:<button>` | `left`, `middle`, `right`, `back`, `forward` | 1 |
| `mouse:wheel`, `mouse:delta` | wheel steps; movement in logical pixels per tick | 2 |
| `pad:<button>` | Positional names on the standard layout: `south`, `east`, `west`, `north`, `lb`, `rb`, `lt`, `rt`, `select`, `start`, `ls`, `rs`, `up`, `down`, `left`, `right`, `home` | 1 (`lt`/`rt` analog) |
| `pad:leftstick`, `pad:rightstick` | sticks | 2 |
| `padraw:b<n>`, `padraw:a<n>` | raw buttons and axes of pads without a standard mapping | 1 |
| `touch:<region>`, `touch:any` | a region of the mapping's `touch` table; any touch | 2 for stick and drag regions, else 1 |
| `mod:<group>` | `shift`, `ctrl`, `alt`, `meta` (either side). Only inside `mods`. | - |

Two-dimensional controls take a part suffix to become one-dimensional: `X`, `Y`, `X+`, `X-`, `Y+`, `Y-` (`pad:leftstickY-` is "stick up", `mouse:wheelY` is vertical scrolling). Positional pad names mean `pad:south` is A on Xbox, Cross on PlayStation and B on Nintendo; prompts use the device's family (see [Names and glyphs](#names-and-glyphs)).

### Binding fields

| Field | Meaning |
| --- | --- |
| `control` | One control, dimensioned like the action (Vec2 needs a 2D control). |
| `composite` | `axis` (`neg`, `pos` to an Axis), `dpad` (`up`, `down`, `left`, `right` to a Vec2) or `chord` (2..4 `controls` that must all be down, to a Button). Exclusive with `control`. |
| `whichWins` | Composites: `none` (default; opposite parts cancel) or `last` (the part pressed last wins). |
| `mods` | Modifier groups that must be down. Bindings with more modifiers are evaluated first and **consume** their controls: `Ctrl+S` = save does not also fire `S` = walk back. |
| `processors` | Applied in order. Each object has exactly one of: `deadzone` (with optional `max`; radial for Vec2; the profile's `Deadzone` setting, when set, replaces it), `invert` (`x`, `y`, `xy`), `scale` (`[f]` or `[fx, fy]`), `normalize` (Vec2: clamp length to 1), `sensitivity` (multiply by the profile's `Sensitivity`, apply `InvertY`), `curve` (`linear`, `quadratic`, `cubic`). |
| `interaction` | One of `press` (default), `release`, `hold` (duration), `tap` (released within), `doubleTap` (second press within), `repeat` (`delay`, `every`). Axis and Vec2 actions take only `press` and `repeat`. |
| `threshold` | Analog magnitude that counts as pressed. 0 means 0.5. |
| `locked` | Rebinding can't change it or take its control (Escape on pause). |

Durations are Go durations (`"300ms"`) converted to ticks at the router's TPS, rounded up, at least one tick. `repeat` on a Button pulses `JustPressed`; on an Axis or Vec2 it gates the value, so it is non-zero only on the pulse ticks: menu navigation from a held stick.

A mapping has `priority` (higher evaluates first), `block` (stop evaluating lower mappings: a menu over gameplay), `strictMods` (a binding does not fire while a modifier group it does not require is held, so `Ctrl+W` is not also `W`), `bindings` and `touch`. `groups` name mappings that are active together, so rebinding reports conflicts across them.

### Touch regions

Regions are in screen fractions (0..1 of the logical size; `circle` radius and stick `radius` are fractions of the height). `kind` is `button`, `stick` (Vec2 from the base to the touch, clamped to `radius`; `floating` puts the base where the touch starts) or `drag` (Vec2 delta in pixels per tick). Use `area` `[x0, y0, x1, y1]` or `circle` `[cx, cy, r]`. A touch belongs to the region it started in until it ends, so a thumb sliding off the stick keeps steering and never presses a button.

`inputsys.TouchControlsExtract` draws the regions of the player that owns the touch screen as screen-space outlines, but only while that player's active device is the touch screen (`AlwaysShow` overrides). The engine adds it; `Config.Input.HideTouchControls` turns it off and `e.TouchControls()` sets its colors.

## Mapping contexts

Each player has a stack of active mapping names: `Push(name)`, `Pop(name)`, `SetMappings(names...)`, `IsActive(name)`. Changes take effect at the next `Router.Update`, so every system in a tick sees the same state. A name the bindings don't define stays on the stack, inactive, until bindings that define it are set.

Each tick the active mappings are evaluated by priority (ties: pushed later first), stopping after the first blocking one. A control used by a binding that fired is consumed for the rest of the tick.

**Fresh-press rule.** When a mapping becomes active, its Button bindings wait until their control has been seen up. Closing a menu with Space does not also jump; the Escape that opened the menu does not also close it. Axis and Vec2 values pass through: holding the stick after closing a menu keeps walking. Rebuilding the bindings (a rebind, a reloaded file) does not count as becoming active.

## Players and devices

`Config.Input.Players` (1..8) sets the slots. `Assign` picks the policy:

- `input.AssignSingle` (default): player 0 gets every device.
- `input.AssignJoin`: devices start unassigned and drive the **lobby** player (`Router.Lobby()`; push its menu mapping). A device whose `JoinAction` binding (in the lobby's bindings) goes down claims the lowest free slot; keyboard and mouse join together. `Events.PlayersJoined` reports it. `Player.Leave()` frees the slot; `Assign`/`Unassign` move devices by hand.

`Router.Events()` (also the `*input.Events` resource) lists, per tick: pads connected and disconnected, players joined and left, devices lost (unplugged while assigned; the player keeps the slot) and restored (the same pad, matched by GUID and name, came back), players whose active device changed (switch prompt glyphs), and rebinding captures.

`Player.ActiveDevice()` is the device that last produced input; prompts follow its `Family` (`xbox`, `playstation`, `nintendo`, `generic`, `kbm`, `touch`). `Player.Rumble(strong, weak, d)` vibrates the player's pads, scaled by the profile's `Vibration`.

Without focus (`Frame.Unfocused`), every control reads up, so alt-tabbing releases held actions.

## Rebinding and profiles

A `Profile` holds a player's settings (`Deadzone`, `InvertY`, `Sensitivity`, `Vibration`) and **only the overrides**: bindings the player changed. A default the game changes in a later version reaches every action the player did not touch. Save with `Profile().Marshal()`, load with `ParseProfile` and `SetProfile` (schema: [`profile.schema.json`](../../../input/schema/profile.schema.json)). Overrides that no longer fit (an action that was removed) are skipped and reported to `OnError`.

Bindings are addressed as **slots**: the n-th binding of a class (`kbm`, `gamepad`, `touch`) of an action in a mapping, plus a `Part` (`up`, `neg`, ...) for composites. `Player.Slots(mapping, action)` lists them for an options screen.

```go
target := input.Slot{Mapping: "gameplay", Action: Jump, Class: input.SlotClassOf(p.ActiveDevice().Class)}
p.Listen(input.ListenOptions{Target: target}) // every action of p reads zero meanwhile

// a later tick:
for _, c := range r.Events().Captured {
	if !c.Canceled {
		err := r.Player(c.Player).Bind(c.Target, c.Control, c.Mods, input.Swap)
	}
}
```

`Listen` waits until every capturable control is up (so the press on "Rebind" is not captured), then takes the first control that goes down: keys and mouse buttons for `kbm`, pad buttons, sticks and triggers for `gamepad`. A modifier alone is captured on release; with `AllowMods`, modifiers held with a key are captured as `Mods`. Escape and Start cancel (`ListenOptions.Cancel` changes that). Touch slots can't be captured.

`Conflicts(target, control, mods)` lists bindings that already use the control in the mapping and its groups. `Bind` resolves them by policy: `Keep` (both bound), `Replace` (unbind the others) or `Swap` (the others get the target's old control). It returns an error and changes nothing for locked bindings, a control of the wrong class, or a composite without a `Part`. `Reset(mapping, action)` and `ResetAll()` drop overrides.

The profile JSON stores each override as `{mapping, action, class, slot}` plus exactly one of `binding` (a whole binding), `control` (with `part`, one part of a composite) or `unbind: true`.

## Names and glyphs

| Call | Example |
| --- | --- |
| `Router.ControlName(c, fam)` | `"A"` (Xbox south), `"Cross"` (PlayStation south), `"Left Stick Up"`, `"Right Click"`, and the key's label on the current keyboard layout where the platform reports it (`"A"` for the Q key on AZERTY). |
| `Router.BindingName(c, mods, fam)` | `"Ctrl+L"` |
| `Router.GlyphKey(c, fam)` | `"xbox/pad_south"`: a key for the game's own prompt atlas. |
| `Bindings.ControlString(c)` | `"touch:jump"`: the file form. |

## Recording and replay

`Router.Record(&input.Recorder{MaxTicks: n})` records every polled frame and device change (idle ticks are folded into a repeat count). `Recorder.WriteTo` saves it (gob), `ReadRecording` loads it and `NewReplaySource` plays it into a new router: the same bindings produce the same action states tick by tick. After the end the replay source reports an unfocused, empty frame.

## Without game.Engine

```go
acts.Freeze()
inputsys.RegisterLoaders(srv, acts) // bindings files load as input.Bindings, compiled on a worker
r := input.NewRouter(ebiteninput.New(func() (int, int) { return w, h }), input.Options{Actions: acts})
sys := inputsys.New(r)
sys.Watch(srv, inputsys.LoadBindings(srv.Global(), "input/bindings.json"))
s.AddStage("input")
s.AddRunner(sys) // first in the input stage; Init publishes *input.Router and *input.Events
```

Tests drive a router with `input.ManualSource` (set `Frame` and `Changes` before each `Update`, read `Vibrations`). `game.Config.Input.Source` takes one too, so a whole game runs headless.

`ebiteninput.LoadGamepadMappings(db)` adds SDL mappings (`gamecontrollerdb.txt`) so more pads get the standard layout instead of `padraw:` controls.

## Costs

`Router.Update` is 0 allocs in steady state (keyboard, mouse, a pad, two touches, three mappings: about 2 µs on a 2.8 GHz Xeon, `BenchmarkRouterUpdate`), and so is every `State` query and `ConsumePress` (`TestUpdateAllocs`, `TestStateQueryAllocs`). Rebinding, `SetProfile`, `SetBindings` and device changes rebuild per-player tables and allocate; they are not per-tick work.

## Panics

Programmer errors panic with `input: `. Bad files are errors with paths.

| Message | Cause |
| --- | --- |
| `input: action "x" declared after Freeze` | Declaring on a frozen table |
| `input: empty action name`, `input: duplicate action "x"`, `input: too many actions` | Bad declaration |
| `input: action "x": Delta is only valid for Vec2 actions` | `Delta()` on a Button or Axis |
| `input: unknown action N` | An `Action` value from another table, or 0 |
| `input: action "x" is Button, not Vec2` | State query of the wrong kind |
| `input: <who>: nil Actions`, `input: <who>: Actions not frozen` | `NewRouter`, `inputsys.RegisterLoaders` before `Freeze` |
| `input: NewRouter: nil Source` | |
| `input: NewRouter: Players must be 1..8, got N` | |
| `input: NewRouter: AssignJoin needs a Button JoinAction` | |
| `input: NewRouter: Bindings were compiled for another Actions table`, `input: SetBindings: ...` | Mixing tables |
| `input: player index out of range: N` | `Router.Player(i)` |
| `input: Listen: player N is already listening`, `input: Listen: cannot capture touch slots` | |
| `input: inputsys.New: nil Router` | |
