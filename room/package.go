// +game:group:name=room
//
// Package room moves the game between rooms: levels named by their base key
// ("cave/cave": cave.level.json plus cave.tmj or cave.world, see
// game.Engine.LoadLevelBase) entered through exits placed in the level
// editor, with a
// screen transition that covers the swap.
//
// A [room.Exit] is an area in a GOD level with a target room and a spawn
// name; a [room.Spawn] names a point; a [room.Traveler] (the hero) triggers
// exits, survives the swap and lands on the spawn. [Install] adds the
// director to an engine and returns its [Rooms]; [Rooms.GoTo] does the same
// from code. Transitions are a fade by default, or any registered
// [Transition]: a post shader ([Shader]), a function ([Func]) or a type of
// your own. See doc/guide/engine/rooms.md.
//
// Package game never imports room: rooms are opt-in.
package room

//go:generate go run github.com/weakpixel/engine/tools/weakpixel gen
