// Package input turns keyboard, mouse, gamepad and touch input into named
// actions. Game code reads actions ("jump", "move"), never keys; which
// control drives an action is data.
//
// The pieces:
//
//   - [Actions]: the action table, declared in Go (Button, Axis, Vec2).
//   - [Bindings]: a bindings file ([BindingsFile], JSON) compiled against the
//     actions: mapping contexts with bindings, composites, modifiers,
//     processors, interactions and touch regions. [ParseBindings] reports
//     every problem at once with GOD-style paths.
//   - [Router]: polls a [Source] once per tick and evaluates every [Player]:
//     its devices, its stack of active mappings, its [Profile] overrides.
//     The result is one [State] per player.
//   - [Profile]: per-player settings and rebinding overrides, saved as JSON.
//
// The package imports only the standard library, so it runs headless: tests
// and replays drive it with [ManualSource] and [ReplaySource]. input/ebiteninput reads Ebiten;
// input/inputsys connects the Router to an ecs World and loads
// bindings files through the asset system. The guide is
// doc/guide/engine/input.md.
package input
