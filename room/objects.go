package room

import "github.com/weakpixel/engine/gfx"

// ExitArea is the Game Object of an exit: where it is and where it leads.
// +game:object
type ExitArea struct {
	// The top-left corner of the area.
	// +game:component
	Transform *gfx.Transform
	// The target room and spawn.
	// +game:component
	Exit *Exit
}

// SpawnPoint is the Game Object of a named arrival point.
// +game:object
type SpawnPoint struct {
	// Where travelers arrive.
	// +game:component
	Transform *gfx.Transform
	// The point's name.
	// +game:component
	Spawn *Spawn
}
