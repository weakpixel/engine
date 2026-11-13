package speech

import (
	"github.com/weakpixel/ecs"
	"github.com/weakpixel/engine/gfx"
)

// Speaker gives an entity a voice. [Speech.Say] speaks a line with it; if
// the entity has a gfx.Label, the system sets Label.Text to the part of the
// line said so far while it speaks. The state of the running line lives in
// the [Speech], not here, so the component is plain authored data.
//
// +game:component
type Speaker struct {
	// Voice is a *.voice.json file or a starter voice by name ("chirpy").
	// The zero handle, a missing or a failed voice speaks silently, at the
	// default speed. A voice that is still loading delays the line until it
	// is ready.
	//
	// +game:asset:kind=voice
	Voice gfx.VoiceHandle `json:"-"`
	// Speed scales how fast the voice speaks: 2 is twice as fast. Zero
	// means 1; the editor starts a new speaker at 1.
	//
	// +game:default=1
	Speed float32
	// Pan moves the voice in the stereo field, -1 (left) to 1 (right). It is
	// added to the pan of every note.
	Pan float32
}

// Talker is a speaking text: a Label placed by a Transform, with a voice.
// The Label is drawn in screen space, so Transform.Pos is in logical pixels.
//
// +game:object
type Talker struct {
	// Where the text is.
	// +game:component
	Transform *gfx.Transform
	// The text the line types into, and how it is drawn.
	// +game:component
	Label *gfx.Label
	// The voice.
	// +game:component
	Speaker *Speaker
}

// DialogueBox plays a *.dialogue.json file on its entity: the current
// line types out into the entity's gfx.Label in its speaker's voice, and
// the speaker's display name goes into the Label of Name. [Speech.Play]
// starts it, [Speech.Next] is the game's "advance" input. The state of the
// running dialogue lives in the [Speech], not here.
//
// +game:component
type DialogueBox struct {
	// Dialogue is the *.dialogue.json file to play. The zero, a missing or
	// a failed dialogue plays nothing (Play sends DialogueFinished at once).
	//
	// +game:asset:kind=dialogue
	Dialogue gfx.DialogueHandle `json:"-"`
	// Name, when set, is an entity with a gfx.Label that shows the name of
	// the current line's speaker. In a GOD level it is the id of an object
	// of the same document.
	Name ecs.Entity
	// AutoStart plays the dialogue from its first line once the asset is
	// ready, without a call of Play: a box placed in a level speaks when
	// the level runs.
	AutoStart bool
	// AutoAdvance goes on after every line by itself after the line's hold
	// time, whatever the dialogue file says (hands-free play).
	AutoAdvance bool
}

// Box is a dialogue box: a Label placed by a Transform that plays a
// dialogue file. The Label is drawn in screen space, so Transform.Pos is in
// logical pixels. Put a gfx:Panel behind it for a frame, and point
// DialogueBox.Name at a gfx:Text for the speaker's name.
//
// +game:object
type Box struct {
	// Where the text is.
	// +game:component
	Transform *gfx.Transform
	// The line types into it.
	// +game:component
	Label *gfx.Label
	// The dialogue.
	// +game:component
	DialogueBox *DialogueBox
}
