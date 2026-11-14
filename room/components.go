package room

import "github.com/weakpixel/engine/gfx"

// ExitTrigger says what fires an [Exit].
//
// +game:enum
type ExitTrigger uint8

// Exit triggers.
const (
	// OnEnter fires when a Traveler walks into the area: it is inside this
	// tick and was outside the tick before, so a traveler that arrives on a
	// spawn inside an exit does not leave at once.
	OnEnter ExitTrigger = iota
	// OnAction fires when a Traveler inside the area presses the input
	// action named Exit.Action (a door that needs a button).
	OnAction
)

// Exit is an area that sends the travelers to another room. The area is the
// rectangle from the entity's Transform.Pos (its top-left corner) with Size,
// as a gfx.DebugShape rect draws it; rotation and scale are ignored. The
// director gives every exit such a debug shape, so the debug overlay and the
// editor show the area.
//
// +game:component
type Exit struct {
	// Target is the room to go to: a level's base key ("cave/cave", see
	// game.Engine.LoadLevelBase). Empty means the current room (a teleport).
	Target string
	// Spawn names the arrival point in the target room: a room:Spawn with
	// that Name, else a Tiled object with that name.
	Spawn string
	// Size is the width and height of the area, in world units.
	Size gfx.Vec2
	// Trigger says what fires the exit: 0 walking in (OnEnter), 1 an input
	// action (OnAction).
	Trigger ExitTrigger
	// Action is the input action an OnAction exit waits for ("interact"),
	// pressed by the traveler's PlayerSlot player (player 0 without one).
	Action string
	// Transition is the name of a registered transition; empty uses the
	// default ("fade" unless Config.Default says otherwise).
	Transition string
}

// Spawn names an arrival point at the entity's Transform.Pos.
//
// +game:component
type Spawn struct {
	// Name is what Exit.Spawn and Rooms.GoTo look for.
	Name string
}

// Traveler marks an entity that triggers exits, is moved to the spawn
// point and survives the room swap (the director takes it out of the old
// room's levels before unloading them). The hero is a traveler.
//
// +game:component
type Traveler struct{}
