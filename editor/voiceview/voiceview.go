// Package voiceview is the "voice" tool of weakpixel-editor: a read-only
// preview with sound of the files of package speech. A voice
// (*.voice.json) speaks a line typed in the panel and draws the line's notes
// as a piano roll; a dialogue (*.dialogue.json) plays in a box, line by line,
// with a Next button. It is the only tool that runs with audio on. Editing
// voices and dialogues stays in the library's Voice Editor.
package voiceview

import (
	"encoding/json"
	"fmt"
	"path"
	"strings"
	"time"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/weakpixel/asset"
	"github.com/weakpixel/ecs"
	"github.com/weakpixel/ecs/system"
	"github.com/weakpixel/engine/editor/toolshell"
	"github.com/weakpixel/engine/gfx"
	"github.com/weakpixel/engine/speech"
)

// The widget ids of the panel.
const (
	IDText    = "voice.text"
	IDSpeed   = "voice.speed"
	IDSay     = "voice.say"
	IDSkip    = "voice.skip"
	IDStop    = "voice.stop"
	IDNext    = "dialogue.next"
	IDRestart = "dialogue.restart"
	IDEnd     = "dialogue.stop"
	IDStatus  = "status"
)

// DefaultLine is the line a voice says when the tool opens.
const DefaultLine = "Hello there, traveler! Are you lost? The village is just over the hill."

// editorHint names where voices are edited; the panel shows it.
const editorHint = "Edit voices and dialogues in the Voice Editor of github.com/weakpixel/voice"

// soundHint is the panel's note on browser audio: a webview starts sound
// only after a click inside the game's canvas (a click on the panel does
// not unlock it).
const soundHint = "In VSCode, click the view once to start the sound"

type kind int

const (
	kindVoice kind = iota
	kindDialogue
)

// kindOf picks what the tool shows for the file at key.
func kindOf(key string) (kind, error) {
	lower := strings.ToLower(key)
	switch {
	case strings.HasSuffix(lower, ".voice.json"):
		return kindVoice, nil
	case strings.HasSuffix(lower, ".dialogue.json"):
		return kindDialogue, nil
	}
	return 0, fmt.Errorf("voice: %s is not a voice or a dialogue (supported: .voice.json, .dialogue.json)", path.Base(key))
}

// Tool is the "voice" tool. It is a [toolshell.Tool] without a document, an
// [toolshell.Initializer] (it installs package speech before the engine's
// schedule is built) and a [toolshell.Closer]: the host can switch it to
// another file.
type Tool struct {
	shell  *toolshell.Shell
	sp     *speech.Speech
	srv    *asset.Server
	events *system.EventReader[speech.Event]
	scope  *asset.Scope
	key    string
	kind   kind

	// The voice: an entity with a Speaker and a Label (no Transform, so
	// the renderer does not draw the label; the tool draws the text).
	talker ecs.Entity
	voice  gfx.VoiceHandle
	voiceV uint32 // the voice's version at the last Say, to notice a reload
	text   string
	speed  float64
	notes  []speech.Note

	// The dialogue: a box entity with a DialogueBox and a Label, and the
	// name entity with a Label.
	box, name ecs.Entity
	dialogue  gfx.DialogueHandle
	dialogueV uint32
	line      int // index of the box's current line, from LineStarted

	view textView
}

// New returns the tool.
func New() toolshell.Tool { return &Tool{text: DefaultLine, speed: 1} }

// Check reports whether the tool can open the file at key. The command
// calls it before opening a window.
func (*Tool) Check(key string) error {
	_, err := kindOf(key)
	return err
}

// Init implements [toolshell.Initializer]: it installs package speech. Its
// system does not run while the shell pauses the game, so Sync steps it.
// A second Init does nothing.
func (t *Tool) Init(s *toolshell.Shell) {
	if t.sp != nil {
		return // the engine has speech already
	}
	e := s.Engine()
	t.sp = speech.Install(e, speech.Config{OnError: s.ReportError})
	t.srv = e.Assets()
	t.events = system.NewEventReader[speech.Event](e.World())
}

// Open implements [toolshell.Tool].
func (t *Tool) Open(s *toolshell.Shell, key string) error {
	k, err := kindOf(key)
	if err != nil {
		return err
	}
	if t.sp == nil {
		return fmt.Errorf("voice: the tool was not initialized (it must be the shell's Tool when the engine is created)")
	}
	if t.scope == nil {
		t.scope = s.Engine().Assets().NewScope("voice")
	}
	t.shell, t.key, t.kind, t.line = s, key, k, 0
	w := s.Engine().World()
	switch k {
	case kindVoice:
		h, err := speech.LoadVoiceSync(t.scope, key)
		t.voice = h
		t.talker = w.NewEntity(speech.Speaker{Voice: h, Speed: 1}, gfx.Label{})
		if err != nil {
			return fmt.Errorf("voice: %w", err)
		}
		t.say()
	case kindDialogue:
		h, err := speech.LoadDialogueSync(t.scope, key)
		t.dialogue = h
		t.name = w.NewEntity(gfx.Label{})
		t.box = w.NewEntity(gfx.Label{}, speech.DialogueBox{Dialogue: h, Name: t.name})
		t.dialogueV = t.dialogueVersion()
		if err != nil {
			return fmt.Errorf("voice: %w", err)
		}
		t.sp.Play(t.box, 0)
	}
	return nil
}

// Close implements [toolshell.Closer]: it ends the line or the dialogue,
// deletes the entities and releases the file's assets.
func (t *Tool) Close(s *toolshell.Shell) {
	w := s.Engine().World()
	for _, e := range []ecs.Entity{t.talker, t.box, t.name} {
		if e != (ecs.Entity{}) && w.IsAlive(e) {
			w.DeleteEntity(e)
		}
	}
	t.talker, t.box, t.name = ecs.Entity{}, ecs.Entity{}, ecs.Entity{}
	t.voice, t.dialogue = gfx.VoiceHandle{}, gfx.DialogueHandle{}
	t.notes = t.notes[:0]
	t.sp.Step(0) // ends the lines of the deleted entities: their notes are released
	if t.scope != nil {
		t.scope.Release()
	}
}

// say speaks the panel's text with the voice, replacing the running line.
func (t *Tool) say() {
	t.sp.Say(t.talker, t.text, speech.SayOptions{Speed: float32(t.speed)})
	t.voiceV = speech.VoiceStoreOf(t.srv).Version(t.voice)
	t.notes = t.sp.LineNotes(t.talker, t.notes[:0])
}

// dialogueVersion is the dialogue's asset version.
func (t *Tool) dialogueVersion() uint32 {
	st := speech.DialogueStoreOf(t.srv).Generic()
	return st.Version(asset.HandleOf[speech.Dialogue](t.dialogue.Ref))
}

// Update implements [toolshell.Tool]. The viewport takes no input: a click
// in it only starts the sound in a browser.
func (t *Tool) Update(*toolshell.Shell) {}

// Sync implements [toolshell.Syncer]: it steps speech by one tick (the
// shell pauses the game, so the speech system does not run) and replays
// the line or the dialogue when its file was reloaded.
func (t *Tool) Sync(s *toolshell.Shell) {
	if t.scope == nil {
		return
	}
	switch t.kind {
	case kindVoice:
		if !t.voice.IsZero() && speech.VoiceStoreOf(t.srv).Version(t.voice) != t.voiceV {
			t.say()
		}
	case kindDialogue:
		if !t.dialogue.IsZero() {
			if v := t.dialogueVersion(); v != t.dialogueV {
				t.dialogueV = v
				t.sp.Play(t.box, 0)
			}
		}
	}
	t.sp.Step(tick())
	for ev := range t.events.Read() {
		if ev.Entity == t.box && ev.Kind == speech.LineStarted {
			t.line = int(ev.Line)
		}
	}
	if t.kind == kindVoice && len(t.notes) == 0 {
		t.notes = t.sp.LineNotes(t.talker, t.notes) // a line that waited for its voice
	}
}

// tick is the length of one engine tick.
func tick() time.Duration {
	if tps := ebiten.TPS(); tps > 0 {
		return time.Second / time.Duration(tps)
	}
	return time.Second / 60
}

// Panel implements [toolshell.Tool].
func (t *Tool) Panel() toolshell.Panel {
	p := toolshell.Panel{Title: "voice " + path.Base(t.key)}
	switch t.kind {
	case kindVoice:
		p.Sections = append(p.Sections, toolshell.Section{ID: "voice", Label: "Voice", Fields: []toolshell.Field{
			toolshell.Text(IDText, "Line", t.text).WithHint("Press Enter to say it"),
			toolshell.Number(IDSpeed, "Speed", t.speed).Range(0.25, 4).WithStep(0.25).WithDigits(2),
			toolshell.Button(IDSay, "Say"),
			toolshell.Button(IDSkip, "Skip"),
			toolshell.Button(IDStop, "Stop"),
			toolshell.Label(IDStatus, "Status", t.voiceStatus()),
		}})
	case kindDialogue:
		p.Sections = append(p.Sections, toolshell.Section{ID: "dialogue", Label: "Dialogue", Fields: []toolshell.Field{
			toolshell.Button(IDNext, "Next"),
			toolshell.Button(IDRestart, "Restart"),
			toolshell.Button(IDEnd, "Stop"),
			toolshell.Label(IDStatus, "Status", t.dialogueStatus()),
		}})
	}
	if t.shell != nil {
		p.Sections = append(p.Sections, t.shell.ViewSection())
	}
	p.Sections = append(p.Sections, toolshell.Section{ID: "about", Label: "About", Collapsed: true, Fields: []toolshell.Field{
		toolshell.Label("about.sound", "Sound", soundHint),
		toolshell.Label("about.edit", "Edit", editorHint),
	}})
	return p
}

// voiceStatus is the status line of a voice: the line's position.
func (t *Tool) voiceStatus() string {
	if t.talker == (ecs.Entity{}) {
		return ""
	}
	at, length := t.sp.LinePosition(t.talker)
	state := "done"
	if t.sp.Speaking(t.talker) {
		state = "speaking"
	}
	return fmt.Sprintf("%s %.1f / %.1f s", state, at.Seconds(), length.Seconds())
}

// dialogueStatus is the status line of a dialogue: the line and its
// speaker.
func (t *Tool) dialogueStatus() string {
	if t.box == (ecs.Entity{}) {
		return ""
	}
	d := speech.DialogueStoreOf(t.srv).Get(t.dialogue)
	if !t.sp.Playing(t.box) || d.Script == nil {
		return "finished"
	}
	_, sp, ok := t.sp.Line(t.box)
	if !ok {
		return "loading"
	}
	name := sp.Name
	if name == "" {
		name = sp.ID
	}
	return fmt.Sprintf("line %d of %d, %s", t.line+1, len(d.Script.Lines), name)
}

// Set implements [toolshell.Tool].
func (t *Tool) Set(id string, v json.RawMessage) error {
	if strings.HasPrefix(id, "voice.") && t.talker == (ecs.Entity{}) || strings.HasPrefix(id, "dialogue.") && t.box == (ecs.Entity{}) {
		return fmt.Errorf("voice: %q does not apply to this file", id)
	}
	switch id {
	case IDText:
		if err := json.Unmarshal(v, &t.text); err != nil {
			return fmt.Errorf("voice: %s: %w", id, err)
		}
		t.say()
	case IDSpeed:
		var f float64
		if err := json.Unmarshal(v, &f); err != nil {
			return fmt.Errorf("voice: %s: %w", id, err)
		}
		t.speed = min(max(f, 0.25), 4)
	case IDSay:
		t.say()
	case IDSkip:
		t.sp.Skip(t.talker)
	case IDStop:
		t.sp.Stop(t.talker)
	case IDNext:
		if t.sp.Playing(t.box) {
			t.sp.Next(t.box)
		} else {
			t.sp.Play(t.box, 0)
		}
	case IDRestart:
		t.sp.Play(t.box, 0)
	case IDEnd:
		t.sp.StopDialogue(t.box)
	default:
		return fmt.Errorf("voice: unknown widget %q", id)
	}
	return nil
}
