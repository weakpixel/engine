# Rooms and transitions

Package `room` moves the game between rooms. A room is a library level named by its base key (`"cave-1/cave-1"`, loaded with `LoadLevelBase`, see [asset-library.md](asset-library.md#engine)). You place exits and spawn points in the level editor; when the hero walks into an exit, the director covers the screen with a transition, swaps the rooms, puts the hero on the spawn point and uncovers the screen. Code can do the same with `GoTo`.

Rooms are opt-in: `game` never imports `room`, and a game that does not call `room.Install` pays nothing.

- [Setup](#setup)
- [Exits, spawns and travelers](#exits-spawns-and-travelers)
- [Going to a room from code](#going-to-a-room-from-code)
- [What happens in a transition](#what-happens-in-a-transition)
- [Custom transitions](#custom-transitions)
- [Config](#config)
- [Not covered](#not-covered)

## Setup

Install the director once, before the first `Update`, and enter the first room:

```go
e := game.New(cfg)
rooms := room.Install(e, room.Config{
	// Spawn options for every room the director loads.
	Spawn: func(id string) tilemap.SpawnOptions {
		return tilemap.SpawnOptions{Chunked: true}
	},
	// Per-room setup after each swap, while the screen is still covered.
	OnEnter: func(ev room.Entered) {
		log.Printf("entered %s at %q", ev.To, ev.Spawn)
	},
})
rooms.Start("world-1/world-1", "start") // load the level world-1/world-1, no visual transition
```

The hero is a traveler:

```go
hero := ecs.NewEntity2(e.World(), gfx.Transform{}, room.Traveler{})
```

A game that loads its first room itself (a base level plus an extra GOD level, say) hands the levels to the director instead of calling `Start`. Leaving the room then unloads all of them:

```go
rooms.SetCurrent("world-1/world-1", worldLevel, objectsLevel)
```

## Exits, spawns and travelers

| Component | Fields | Meaning |
| --- | --- | --- |
| `room:Exit` | `Target`, `Spawn`, `Size`, `Trigger`, `Action`, `Transition` | An area that sends the travelers to the room `Target` (a base key; `""` is the current room), onto the spawn `Spawn` |
| `room:Spawn` | `Name` | An arrival point at the entity's `Transform.Pos` |
| `room:Traveler` | none | Triggers exits, is moved to the spawn and survives the swap. The hero is one |

The GOD objects are `room:ExitArea` (`gfx:Transform` + `room:Exit`) and `room:SpawnPoint` (`gfx:Transform` + `room:Spawn`). In a level document:

```json
{ "id": "cave-door", "group": "room", "class": "ExitArea", "components": {
    "gfx:Transform": { "Pos": { "X": 560, "Y": 292 }, "PrevPos": { "X": 560, "Y": 292 }, "Rot": 0, "Scale": { "X": 1, "Y": 1 } },
    "room:Exit": { "Target": "cave-1/cave-1", "Spawn": "cave-entry", "Size": { "X": 16, "Y": 16 },
                   "Trigger": 0, "Action": "", "Transition": "iris" } } },
{ "id": "from-cave", "group": "room", "class": "SpawnPoint", "components": {
    "gfx:Transform": { "Pos": { "X": 568, "Y": 330 }, "PrevPos": { "X": 568, "Y": 330 }, "Rot": 0, "Scale": { "X": 1, "Y": 1 } },
    "room:Spawn": { "Name": "from-cave" } } }
```

Like any object, an exit can carry more components: a `gfx:Sprite` for a visible door, a `gfx:Particles` for a glow.

- **The area** is the rectangle from `Transform.Pos` (its top-left corner) with `Size`. Rotation and scale are ignored.
- **`Trigger` 0 (`room.OnEnter`)** fires when a traveler walks in: it is inside this tick and was outside before (its `PrevPos`). A traveler that arrives on a spawn inside an exit does not leave again at once; it has to walk out and back in.
- **`Trigger` 1 (`room.OnAction`)** fires when a traveler inside the area presses the input action named `Action` (a door that needs a button). The press is read from the traveler's `gfx.PlayerSlot` player, or player 0. An action the game did not declare goes to `OnError` once.
- **`Transition`** names a registered transition; `""` uses the default (`"fade"` unless `Config.Default` says otherwise). An unknown name uses the default and goes to `OnError` once.
- **Spawn lookup:** first a `room:Spawn` with that name, then a Tiled object with that name in the room's maps, so a Tiled point such as `player` works too. A name that is found nowhere goes to `OnError` and leaves the travelers where they were.
- **Debug shapes:** the director gives every exit a `gfx.DebugShape` rectangle of its size and every spawn a small circle, so the debug overlay (`Engine.SetDebug`) shows them and the level editor can pick them. The shapes are runtime-only and never saved.

Exits and spawns are Game Objects in the level, not Tiled objects, because the editor validates and places them like everything else ([where to place objects](tilemaps.md#where-to-place-objects)). Exits drawn in Tiled still work with a few lines of game code:

```go
for l, o := range m.ObjectsOfClass("exit") {
	min := tilemap.ObjectPos(l, o, lm.Pos)
	area := gfx.Rect{Min: min, Max: min.Add(gfx.Vec2{X: float32(o.Width), Y: float32(o.Height)})}
	if area.Contains(hero.Pos) && !area.Contains(hero.PrevPos) && !rooms.Busy() {
		target, _ := o.Props.String("target")
		spawn, _ := o.Props.String("spawn")
		rooms.GoTo(target, spawn)
	}
}
```

## Going to a room from code

```go
rooms.GoTo("cave-1", "from-world")                                    // the default transition
rooms.GoTo("boss", "arena", room.With("iris"), room.Seconds(0.6, 0.4)) // cover in 0.6 s, uncover in 0.4 s
rooms.GoTo("", "checkpoint-2")                                        // same room: a teleport behind a fade
```

`GoTo` returns false and does nothing while a transition runs (`rooms.Busy()`). `rooms.Room()` is the current room's id, `rooms.Current()` its levels and `rooms.Phase()` the running phase. Problems found later (an unknown room, a room that fails to load, an unknown spawn) go to `Config.OnError` and leave the old room in place.

## What happens in a transition

| Phase | Progress | What the director does |
| --- | --- | --- |
| `Out` | 0 to 1 over the out time | Holds `StageSimulate` (`Engine.Hold`), so the hero cannot walk on |
| `Wait` | 1 | Loads the target level. Once it has spawned and its assets are done: takes the travelers out of the old room's levels, unloads those levels, calls `OnEnter`, and waits for every level of the new room to spawn (also the ones `OnEnter` added with `SetCurrent`). Then it places the travelers on the spawn (`Pos` and `PrevPos`), sets `CameraRig().Bounds` to the room's maps and snaps the camera |
| `In` | 1 to 0 over the in time | Releases the hold at the end |

- Loading starts only once the screen is covered, because the new room's map spawns at its own world position and could show through a half-covered old room. Both rooms are loaded only during `Wait`.
- Times are simulation time. The defaults are 0.25 s each way (`room.DefaultOut`, `room.DefaultIn`); `Start` uses the `"cut"` transition and no time.
- The input, camera and post stages keep running during a transition, so tweens advance and transitions can use them. `Config.KeepSimulating` keeps `StageSimulate` running too.
- The director runs in `StagePost`. It does nothing but give exits and spawns their debug shapes while a `game.Tool` (the level editor) pauses the game.

## Custom transitions

A transition must cover the whole screen at progress 1, because the swap happens then. There are three ways to write one.

### A shader

`room.Shader(e, key)` loads a post shader ([post-processing](shaders.md#post-processing)) into the global scope. While the transition runs, the director appends a pass to `e.Post().Passes` and removes it at the end (don't replace `Passes` during a transition). The shader's `Params`:

| Uniform | Value |
| --- | --- |
| `Params.x` | Progress: 0 open, 1 covered |
| `Params.y`, `Params.z` | The first traveler on screen, in logical pixels (the screen centre without travelers) |
| `Params.w` | 0 while covering, 1 while uncovering |

An iris that closes on the hero:

```kage
//kage:unit pixels

package main

var Params vec4

func Fragment(dst vec4, src vec2, color vec4) vec4 {
	c := imageSrc0At(src)
	r := (1 - Params.x) * length(imageSrc0Size())
	d := distance(src-imageSrc0Origin(), Params.yz)
	return mix(c, vec4(0, 0, 0, 1), clamp(d-r+0.5, 0, 1))
}
```

```go
rooms.Register("iris", room.Shader(e, "global/fx/iris.kage"))
```

Wipes, pixelation, dissolves and diamond tiles take the same uniforms and a different `Fragment`. A shader that is not loaded yet when a transition starts makes that one transition a black fade.

### A function

`room.Func(f)` calls `f` every tick with the `room.Frame`, and once more at the end with `Progress` 0 and `Phase` `room.Idle`, so `f` can undo what it changed:

```go
rooms.Register("zoom", room.Func(func(f *room.Frame) {
	f.Engine.Camera().Zoom = 2 + f.Progress*4
	f.Engine.Post().Fade = gfx.RGBA(0, 0, 0, uint8(255*f.Progress))
}))
```

### The interface

```go
type Transition interface {
	Begin(f *room.Frame)  // when the transition starts (in GoTo)
	Update(f *room.Frame) // every tick after that
	End(f *room.Frame)    // once the new room is uncovered
}
```

Use it when a transition needs setup and teardown, such as overlay entities:

```go
type curtains struct{ left, right ecs.Entity }

func (c *curtains) Begin(f *room.Frame)  { /* spawn two ScreenSprites */ }
func (c *curtains) Update(f *room.Frame) { /* move each across f.Progress of the screen */ }
func (c *curtains) End(f *room.Frame)    { /* delete them */ }

rooms.Register("curtains", &curtains{})
```

`room.Frame` holds `Engine`, `Phase`, `T` (the raw progress), `Progress` (`T` eased with `tween.InOutSine`), `Focus` (the first traveler on screen, in the old room during `Out` and in the new one during `In`), `From`, `To` and `Spawn`. One transition value runs one transition at a time.

The built-ins are `"fade"` (`room.Fade(gfx.Black)`, the default) and `"cut"` (`room.Cut()`, nothing drawn). `room.Fade(color)` fades through another color through `render.PostChain.Fade`, which it resets to 0 at the end.

## Config

| Field | Default | Meaning |
| --- | --- | --- |
| `Spawn` | zero `tilemap.SpawnOptions` | The spawn options of the room with that id |
| `OnEnter` | none | Runs after each swap (also after `Start` and for a teleport), before the travelers are placed. It may add levels to the room with `SetCurrent` |
| `OnError` | `log.Print` | Receives what goes wrong in a transition |
| `Default` | `"fade"` | The transition of exits and `GoTo` calls that name none |
| `Out`, `In` | 0.25 s | Cover and uncover times; negative means none |
| `KeepSimulating` | false | Leaves `StageSimulate` running during transitions |
| `Scene` | nil | A `game.Scene`, not pushed yet, that gets the director instead of the root and loads the rooms ([scenes](engine.md#scenes)): popping or replacing it unloads the current room. Its rooms load once it is pushed |
| `NoExits` | false | Exits never fire (they keep their debug shapes); `GoTo` still works. Editor builds set it, so playing a level in the editor never unloads it |


## Not covered

- Slide or scroll transitions where both rooms are visible at once. Maps placed side by side in one Tiled `.world` already scroll with the camera.
- Loading the next room's assets while the screen covers.
- Saving room state (picked-up items) between visits: rooms load fresh.
- An editor gizmo for exit areas beyond the debug shape, and pickers for `Target` and `Spawn`.
- Lua bindings.
