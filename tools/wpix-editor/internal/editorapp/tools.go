package editorapp

import (
	"github.com/weakpixel/engine/editor/animedit"
	"github.com/weakpixel/engine/editor/fxedit"
	"github.com/weakpixel/engine/editor/sheetedit"
	"github.com/weakpixel/engine/editor/toolshell"
	"github.com/weakpixel/engine/editor/viewer"
	"github.com/weakpixel/engine/editor/voiceview"
)

// Tool is a tool's constructor and its line in help.
type Tool struct {
	New     func() toolshell.Tool
	Summary string
	// FileOptional is true for a tool that starts an untitled document when
	// it gets no file.
	FileOptional bool
	// Sheet is true for a tool whose command takes --sheet, the Aseprite
	// export a definition that does not exist yet starts from.
	Sheet bool
	// Image is true for a tool whose command takes --image, the texture a
	// sheet that does not exist yet starts from.
	Image bool
	// Audio is true for a tool that plays sound: its engine opens the audio
	// context (every other tool runs with game.Config.NoAudio).
	Audio bool
}

// Tools are the tools by name. There is no global registry: adding a tool
// is one line here, and the command line gets its subcommand from it.
var Tools = map[string]Tool{
	"anim":  {animedit.New, "edit an animation definition (.anim.json): pivot, body, hit and hurt boxes, clips; an Aseprite export (.aseprite.json, .ase.json) opens read-only", false, true, false, false},
	"fx":    {fxedit.New, "make a particle effect (.fx.json): presets, pickers, live preview; no file starts an untitled effect", true, false, false, false},
	"sheet": {sheetedit.New, "edit a grid sprite sheet (.sheet.json): frame size, margin and spacing over the texture, frame count and the index under the mouse", false, false, true, false},
	"voice": {voiceview.New, "preview a voice (.voice.json: says a line from the panel, with the piano roll of its notes) or a dialogue (.dialogue.json: plays it in a box, Next goes on) with sound; edit them in the Voice Editor", false, false, false, true},
	"view":  {viewer.New, "preview a texture (.png, .jpg), an atlas (.sheet.json, .atlas.json: frames, names, indices) a font (.ttf, .otf: sample text per size) or a Tiled file (.tmj map: layers and objects, .tsj tileset: tile ids, collision and animations, .world: maps and their levels) or a shader (.kage: drawn on a texture, with a control per +shader:param marker)", false, false, false, false},
}

// FillWindow is the logical size the engine is configured with. With
// IntegerScale the engine picks the largest whole scale that fits that size
// in the window, which is 1 for any window smaller than this: the logical
// screen is then the window itself, and the viewport, the mouse and the
// overlays all work in the same pixels.
const FillWindow = 1 << 14
