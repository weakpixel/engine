package editorapp

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/weakpixel/engine/editor/animedit"
	"github.com/weakpixel/engine/editor/filekind"
	"github.com/weakpixel/engine/editor/fxedit"
	"github.com/weakpixel/engine/editor/sheetedit"
	"github.com/weakpixel/engine/editor/toolshell"
	"github.com/weakpixel/engine/editor/viewer"
)

// Kind is a file kind of [filekind.All] with the tool that opens it.
type Kind struct {
	filekind.Info
	// Tool is the name of the tool: the editor of an edited kind ("sheet",
	// "anim", "fx"), "view" for every previewed kind (they share the
	// preview router of package viewer). It is the shell's name, which the
	// ready message carries.
	Tool string
	New  func() toolshell.Tool
	// Summary is the kind's line in help.
	Summary string
	// Starter makes a file of the kind that does not exist yet; nil for a
	// kind that must exist. It gets the value of Flag (empty without a
	// Flag), the new file's path and its asset key.
	Starter func(flag, file, key string) (Start, error)
	// Flag is the command-line flag the starter needs ("image", "sheet"),
	// empty for a starter without one or no starter.
	Flag string
	// FlagValue names the flag's value in help and errors.
	FlagValue string
	// Noun names a new file of the kind in the error of a missing one.
	Noun string
	// Untitled makes the text of an untitled document of the kind (a page
	// with weakpixelHost.untitled: a file without a name on disk yet); nil
	// for a kind that cannot be untitled. Unlike Starter it knows no asset
	// key, so it names no files of its own.
	Untitled func() []byte
}

// Kinds are the kinds of [filekind.All] that open in a tool of the tool
// shell, in that order: the stage kinds (a prefab) open in the level
// editor's stage instead and have no tool. There is no global registry: a
// new kind is a line in filekind and one here.
var Kinds = []Kind{
	{Info: info(filekind.Sheet), Tool: "sheet", New: sheetedit.New,
		Summary: "edit a grid sprite sheet: frame size, margin and spacing over the texture, frame count and the index under the mouse",
		Starter: textStarter(sheetedit.NewSheet), Flag: "image", FlagValue: "texture", Noun: "sheet"},
	{Info: info(filekind.Anim), Tool: "anim", New: animedit.New,
		Summary: "edit an animation definition: pivot, body, hit and hurt boxes, clips",
		Starter: textStarter(animedit.NewDefinition), Flag: "sheet", FlagValue: "export", Noun: "definition"},
	{Info: info(filekind.Fx), Tool: "fx", New: fxedit.New,
		Summary: "make a particle effect: presets, pickers, live preview; a new file starts from the default effect",
		Starter: func(_, _, key string) (Start, error) {
			text, files := fxedit.NewEffect(key)
			return Start{Text: text, Files: files}, nil
		},
		Untitled: fxedit.NewUntitled},
	preview(filekind.Texture, "preview a texture"),
	preview(filekind.Atlas, "preview an atlas: its frames, names and indices over the texture"),
	preview(filekind.Font, "preview a font: sample text per size"),
	preview(filekind.Shader, "preview a shader drawn on a texture, with a control per +shader:param marker"),
	preview(filekind.Map, "preview a Tiled map: layers and objects"),
	preview(filekind.Tileset, "preview a Tiled tileset: tile ids, collision and animations"),
	preview(filekind.World, "preview a Tiled world: its maps and the levels that use them"),
	preview(filekind.Aseprite, "play the clips of an Aseprite export, read-only"),
	preview(filekind.Voice, "preview a voice with sound: says a line from the panel, with the piano roll of its notes; edit it in the Voice Editor"),
	preview(filekind.Dialogue, "preview a dialogue with sound: plays it in a box, Next goes on; edit it in the Voice Editor"),
}

// Start is a new file's starter: its text and the files it names that the
// first save writes with it (asset keys), such as the sprite a new effect
// copies from the built-in one.
type Start struct {
	Text  []byte
	Files []toolshell.CreateFile
}

// textStarter is a starter that makes only the text, from the flag's file.
func textStarter(f func(flag, file string) ([]byte, error)) func(flag, file, key string) (Start, error) {
	return func(flag, file, _ string) (Start, error) {
		text, err := f(flag, file)
		return Start{Text: text}, err
	}
}

func info(k filekind.Kind) filekind.Info {
	i, ok := filekind.Lookup(k)
	if !ok {
		panic("editorapp: no file kind " + string(k))
	}
	return i
}

func preview(k filekind.Kind, summary string) Kind {
	return Kind{Info: info(k), Tool: "view", New: viewer.New, Summary: summary}
}

// KindOf returns the kind of the file name or asset key name. A name of no
// kind is an error that names the supported suffixes.
func KindOf(name string) (Kind, error) {
	i, err := filekind.Of(name)
	if err != nil {
		return Kind{}, err
	}
	if i.Stage {
		return Kind{}, fmt.Errorf("%s: a %s opens in VSCode or Weakpixel Studio", filepath.Base(name), i.Kind)
	}
	for _, k := range Kinds {
		if k.Kind == i.Kind {
			return k, nil
		}
	}
	panic("editorapp: file kind " + string(i.Kind) + " has no tool")
}

// MissingError is the error of a file that does not exist and cannot start
// from the kind's starter without its flag (or has no starter).
func (k Kind) MissingError(file string) error {
	name := filepath.Base(file)
	if k.Flag == "" {
		return fmt.Errorf("%s does not exist", name)
	}
	return fmt.Errorf("%s does not exist: give --%s <%s> to start a new %s", name, k.Flag, k.FlagValue, k.Noun)
}

// Table prints every kind of [filekind.All] one per line as "<suffixes>
// <edit|preview> [stage] [sound]", the form the VSCode extension's lists
// are compared with.
func Table() string {
	var b strings.Builder
	for _, k := range filekind.All {
		mode := "preview"
		if k.Edits {
			mode = "edit"
		}
		fmt.Fprintf(&b, "%s %s", strings.Join(k.Suffixes, " "), mode)
		if k.Stage {
			b.WriteString(" stage")
		}
		if k.Sound {
			b.WriteString(" sound")
		}
		b.WriteByte('\n')
	}
	return b.String()
}

// FillWindow is the logical size the engine is configured with. With
// IntegerScale the engine picks the largest whole scale that fits that size
// in the window, which is 1 for any window smaller than this: the logical
// screen is then the window itself, and the viewport, the mouse and the
// overlays all work in the same pixels.
const FillWindow = 1 << 14
