package fxedit

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io/fs"
	"math"
	"path"
	"slices"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/weakpixel/ecs"
	"github.com/weakpixel/engine/editor/toolshell"
	"github.com/weakpixel/engine/gfx"
	"github.com/weakpixel/engine/input"
	"github.com/weakpixel/engine/internal/shaderparam"
	"github.com/weakpixel/engine/particles"
	"github.com/weakpixel/engine/particles/fxfile"
	"github.com/weakpixel/engine/render"
)

// Suffix is the file suffix of an effect.
const Suffix = fxfile.Suffix

// previewKey is the asset key the tool serves the effect it shows under, in
// the shell's overlay: the preview entity plays this file and a change of the
// model is a hot reload of it, so the preview runs the same loader and system
// as a game.
const previewKey = BuiltinPrefix + "preview" + Suffix

const (
	checkerKey  = BuiltinPrefix + "checker.png"
	checkerPx   = 1024 // side of the checker texture; a cell is 16 px
	checkerCell = 16
	grabPx      = 12 // distance from the emitter cross that grabs it
	// doubleClick is the longest gap between two clicks on the cross that
	// still counts as a double click, in ticks.
	doubleClick = 24
	// reListEvery is how often the asset folder is read again for the
	// pickers, and markerEvery how often the shader file is checked, in ticks.
	reListEvery  = 120
	markerEvery  = 30
	scrubSeconds = 5 // the scrub range of an endless effect
)

// speeds are the entries of the time scale dropdown.
var (
	speedNames  = []string{"0.1×", "0.25×", "0.5×", "1×", "2×"}
	speedValues = []float32{0.1, 0.25, 0.5, 1, 2}
)

const defaultSpeed = 3

// Preview ids.
const (
	idPlay     = "preview.play"
	idStep     = "preview.step"
	idSpeed    = "preview.speed"
	idScrub    = "preview.scrub"
	idTime     = "preview.time"
	idOverlays = "preview.overlays"
	idChecker  = "preview.checker"
	idBGImage  = "preview.bg"
)

// NewUntitled returns the text of the untitled effect the tool starts from:
// the smoke preset with a built-in sprite.
func NewUntitled() []byte {
	return text(Presets()[DefaultPreset].Emitters[0])
}

// NewEffect returns the starter of a new effect file at asset key key: the
// untitled effect with its built-in sprite exported next to it, as Save as
// would write it. The text names the copy; files holds it (asset keys).
// The caller writes the files with the text, so the saved effect never
// names a built-in file.
func NewEffect(key string) (doc []byte, files []SaveFile) {
	ems, files := ExportAll([]Emitter{Presets()[DefaultPreset].Emitters[0]}, key)
	return textOf(ems), files
}

// Tool is the fx tool. It implements [toolshell.DocumentTool],
// [toolshell.Syncer] and [toolshell.Checker].
type Tool struct {
	shell *toolshell.Shell
	key   string
	st    *State
	clock *particles.Clock

	effect, checker, backdrop ecs.Entity
	// preview is the handle of the file under previewKey, zero until the
	// first version of the model was published.
	preview gfx.EffectHandle
	sprites map[string]gfx.SpriteHandle
	lastRev int
	tick    int

	speed       int
	scrub       float64
	noOverlays  bool
	showChecker bool
	bgImage     string
	bgLoaded    string
	paramsKey   string
	paramsStamp string
	grab        bool
	grabbed     bool
	lastGrab    int
	prevBtn     uint8
	onBar       bool
	bar         *toolshell.Toolbar
	imagesSig   string
}

// The shell finds these interfaces with type assertions.
var (
	_ toolshell.DocumentTool = (*Tool)(nil)
	_ toolshell.Syncer       = (*Tool)(nil)
	_ toolshell.Checker      = (*Tool)(nil)
)

// New returns the fx tool.
func New() toolshell.Tool {
	return &Tool{speed: defaultSpeed, lastRev: -1, bar: &toolshell.Toolbar{Buttons: []toolshell.ToolbarButton{
		{Icon: "move", Hint: "Put the emitter back at the origin"},
		{Icon: "burst", Hint: "Burst (click in the viewport)"},
		{Icon: "pause", Hint: "Play or pause (Enter)"},
		{Icon: "step", Hint: "Step one tick (.)"},
		{Icon: "restart", Hint: "Restart (R)"},
	}}}
}

// Check reports whether the tool can open the file at asset key. An empty key
// is the untitled effect.
func (*Tool) Check(key string) error {
	if key != "" && !strings.HasSuffix(key, Suffix) {
		return fmt.Errorf("fx: %s is not a %s file", key, Suffix)
	}
	return nil
}

// Open implements [toolshell.Tool].
func (t *Tool) Open(s *toolshell.Shell, key string) error {
	if err := t.Check(key); err != nil {
		return err
	}
	t.shell, t.key = s, key
	e := s.Engine()
	if ov := s.Overlay(); ov != nil {
		for _, k := range BuiltinKeys() {
			data, _ := BuiltinPNG(k)
			ov.Set(k, data)
		}
		for _, k := range BuiltinShaderKeys() {
			data, _ := BuiltinShader(k)
			ov.Set(k, data)
		}
		ov.Set(checkerKey, checkerPNG())
	}
	t.clock = ecs.GetResourceOrNil[particles.Clock](e.World())
	if t.clock == nil {
		t.clock = &particles.Clock{}
		ecs.SetResource(e.World(), t.clock)
	}
	t.st = NewState(key, Hooks{
		Edit: s.Edit, Create: s.Create,
		Exists: func(k string) bool {
			if files := s.Files(); files != nil {
				return slices.Contains(files, k)
			}
			if s.FS() == nil {
				return false
			}
			_, err := fs.Stat(s.FS(), k)
			return err == nil
		},
	})
	t.sprites = map[string]gfx.SpriteHandle{}
	t.refreshAssets()
	w := e.World()
	t.effect = ecs.NewEntity2(w, gfx.Transform{Scale: gfx.Vec2{X: 1, Y: 1}}, gfx.Particles{Stop: true})
	t.checker = ecs.NewEntity2(w, gfx.Transform{}, gfx.Sprite{Layer: math.MinInt16, Hidden: true})
	t.backdrop = ecs.NewEntity2(w, gfx.Transform{}, gfx.Sprite{Layer: math.MinInt16 + 1, Hidden: true})
	if h, _ := render.LoadSpriteSync(e.Assets().Global(), checkerKey); h != (gfx.SpriteHandle{}) {
		ecs.GetComp[gfx.Sprite](w, t.checker).ID = h
	}
	s.Frame(gfx.Rect{Min: gfx.Vec2{X: -64, Y: -64}, Max: gfx.Vec2{X: 64, Y: 64}})
	return nil
}

// Load implements [toolshell.DocumentTool]: a new version of the document.
// One that does not parse leaves the shown effect as it is and marks it
// stale.
func (t *Tool) Load(text []byte) error {
	if err := t.st.Load(text); err != nil {
		return fmt.Errorf("fx: %w", err)
	}
	t.paramsKey = "\x00" // read the markers again
	return nil
}

// refreshAssets reads the asset folder for the pickers.
func (t *Tool) refreshAssets() {
	var images, shaders []string
	if files := t.shell.Files(); files != nil {
		images, shaders = ClassifyAssets(files)
	} else {
		images, shaders = ListAssets(t.shell.FS())
	}
	sig := strings.Join(images, "\n") + "\x00" + strings.Join(shaders, "\n")
	if sig == t.imagesSig {
		return
	}
	t.imagesSig = sig
	t.st.SetCandidates(images, shaders)
	t.lastRev = -1
}

// refreshMarkers reads the markers of the emitter's shader when the shader
// changed or its file did.
func (t *Tool) refreshMarkers() {
	key := t.st.Em.Shader
	stamp := ""
	if key != "" && t.shell.FS() != nil {
		if fi, err := fs.Stat(t.shell.FS(), key); err == nil {
			stamp = fmt.Sprintf("%d/%d", fi.Size(), fi.ModTime().UnixNano())
		}
	}
	if key == t.paramsKey && stamp == t.paramsStamp {
		return
	}
	t.paramsKey, t.paramsStamp = key, stamp
	t.st.Params, t.st.ParamsErr = nil, nil
	if key == "" || t.shell.FS() == nil {
		return
	}
	data, err := fs.ReadFile(t.shell.FS(), key)
	if err != nil {
		return // a missing shader is a Problems finding
	}
	t.st.Params, t.st.ParamsErr = shaderparam.Parse(data)
}

// Update implements [toolshell.Tool]: the keys and the mouse in the
// viewport.
func (t *Tool) Update(s *toolshell.Shell) {
	st := t.st
	if st == nil {
		return
	}
	if !s.Down(input.KeyControlLeft) && !s.Down(input.KeyControlRight) && !s.Down(input.KeyMetaLeft) && !s.Down(input.KeyMetaRight) {
		switch {
		case s.Pressed(input.KeyEnter):
			t.togglePlay()
		case s.Pressed(input.KeyPeriod):
			t.step()
		case s.Pressed(input.KeyR):
			st.Restart = true
		}
	}
	m := s.Mouse()
	down, wasDown := m.Buttons&1 != 0, t.prevBtn&1 != 0
	t.prevBtn = m.Buttons
	panning := s.PanHeld()
	switch {
	case down && !wasDown && t.bar.Over(s, m.X, m.Y):
		t.onBar = true
		if i, ok := t.bar.At(s, m.X, m.Y); ok {
			t.press(i)
		}
	case down && !wasDown && !panning:
		t.pointerDown(s, m)
	case down && t.grabbed && !t.onBar:
		t.moveEmitter(s.MouseWorld())
	}
	if !down {
		t.grabbed, t.onBar = false, false
	}
	if t.bar.Over(s, m.X, m.Y) {
		ebiten.SetCursorShape(ebiten.CursorShapePointer)
	}
}

// press runs the toolbar button i.
func (t *Tool) press(i int) {
	switch i {
	case 0:
		t.moveEmitter(gfx.Vec2{})
	case 1:
		t.st.Burst = true
	case 2:
		t.togglePlay()
	case 3:
		t.step()
	case 4:
		t.st.Restart = true
	}
}

func (t *Tool) togglePlay() { t.clock.Paused = !t.clock.Paused }

// step runs one tick and stays paused.
func (t *Tool) step() {
	t.clock.Paused = true
	t.clock.Step++
}

// pointerDown grabs the emitter's cross, or bursts when the click is
// elsewhere. A double click on the cross puts the emitter back at the
// origin.
func (t *Tool) pointerDown(s *toolshell.Shell, m input.MouseFrame) {
	if t.nearEmitter(s, m.X, m.Y) {
		if t.tick-t.lastGrab <= doubleClick {
			t.moveEmitter(gfx.Vec2{})
			t.lastGrab = -doubleClick * 2
			return
		}
		t.lastGrab, t.grabbed = t.tick, true
		return
	}
	t.st.Burst = true
}

// nearEmitter reports whether the window point is within reach of the cross.
func (t *Tool) nearEmitter(s *toolshell.Shell, x, y float32) bool {
	at := s.WindowRect(gfx.Rect{Min: t.pos(), Max: t.pos()}).Min
	return math.Hypot(float64(x-at.X), float64(y-at.Y)) <= grabPx
}

func (t *Tool) pos() gfx.Vec2 {
	if tr := ecs.GetCompOrNil[gfx.Transform](t.shell.Engine().World(), t.effect); tr != nil {
		return tr.Pos
	}
	return gfx.Vec2{}
}

// moveEmitter puts the effect entity at p. The position is preview only: it
// is never written to the file.
func (t *Tool) moveEmitter(p gfx.Vec2) {
	if tr := ecs.GetCompOrNil[gfx.Transform](t.shell.Engine().World(), t.effect); tr != nil {
		tr.Pos, tr.PrevPos = p, p
	}
}

// Sync implements [toolshell.Syncer]: it keeps the preview entities in step
// with the model and the clock.
func (t *Tool) Sync(s *toolshell.Shell) {
	st := t.st
	if st == nil {
		return
	}
	t.tick++
	if t.tick%reListEvery == 0 && s.Files() == nil {
		t.refreshAssets()
	}
	if t.tick%markerEvery == 1 || t.paramsKey == "\x00" {
		t.refreshMarkers()
	}
	w := s.Engine().World()
	c := ecs.GetCompOrNil[gfx.Particles](w, t.effect)
	if c == nil {
		return
	}
	if st.Rev != t.lastRev && st.Loaded {
		t.lastRev = st.Rev
		t.publish(c)
	}
	if st.Restart {
		st.Restart, st.Burst = false, false
		c.Time, c.Done, c.Pool = 0, false, 0
	}
	if st.Burst {
		st.Burst = false
		c.Time, c.Done = 0, false
	}
	st.Live, st.Max = c.Live, st.Capacity()
	t.syncBackground(s)
	t.bar.Buttons[2].Icon = "pause"
	if t.clock.Paused {
		t.bar.Buttons[2].Icon = "play"
	}
}

// publish serves the model as the preview effect. The first time it starts
// the preview; after that the engine's hot reload swaps the effect, which
// restarts the pool only for a change live particles cannot follow (see
// particles.System), so a slider drag does not restart the effect.
func (t *Tool) publish(c *gfx.Particles) {
	ov := t.shell.Overlay()
	if ov == nil {
		return
	}
	ov.Set(previewKey, t.st.previewText())
	assets := t.shell.Engine().Assets()
	if t.preview.IsZero() {
		t.preview = particles.LoadEffect(assets.Global(), previewKey)
		c.Effect, c.Stop = t.preview, false
		return
	}
	assets.Changed(previewKey)
}

// sprite returns the handle of an image key, loading it on first use. A
// missing file keeps its (failed) handle: it draws the placeholder and
// recovers when the file appears.
func (t *Tool) sprite(key string) gfx.SpriteHandle {
	if key == "" {
		return gfx.SpriteHandle{}
	}
	if h, ok := t.sprites[key]; ok {
		return h
	}
	h, _ := render.LoadSpriteSync(t.shell.Engine().Assets().Global(), key)
	t.sprites[key] = h
	return h
}

// syncBackground places the checker under the viewport and shows the
// background image.
func (t *Tool) syncBackground(s *toolshell.Shell) {
	w := s.Engine().World()
	cam := s.Engine().Camera()
	if sp := ecs.GetCompOrNil[gfx.Sprite](w, t.checker); sp != nil {
		sp.Hidden = !t.showChecker
		if t.showChecker {
			vp := cam.Viewport.Size()
			if vp.X == 0 {
				vp = s.ScreenSize()
			}
			zoom := max(cam.Zoom, 1.0/16)
			need := max(float32(max(vp.X, vp.Y))/zoom, 1)
			scale := float32(1)
			for scale*(checkerPx-4*checkerCell) < need {
				scale *= 2
			}
			period := 2 * checkerCell * scale // two cells make one repeat of the pattern
			at := gfx.Vec2{X: float32(math.Round(float64(cam.Pos.X/period))) * period, Y: float32(math.Round(float64(cam.Pos.Y/period))) * period}
			if tr := ecs.GetCompOrNil[gfx.Transform](w, t.checker); tr != nil {
				tr.Pos, tr.PrevPos, tr.Scale = at, at, gfx.Vec2{X: scale, Y: scale}
			}
		}
	}
	if sp := ecs.GetCompOrNil[gfx.Sprite](w, t.backdrop); sp != nil {
		sp.Hidden = t.bgImage == ""
		if t.bgImage != t.bgLoaded {
			t.bgLoaded = t.bgImage
			sp.ID = t.sprite(t.bgImage)
		}
	}
}

// Panel implements [toolshell.Tool].
func (t *Tool) Panel() toolshell.Panel {
	title := "Untitled effect"
	if t.key != "" {
		title = path.Base(t.key)
	}
	if t.shell.Key() != "" {
		title = path.Base(t.shell.Key())
	}
	p := t.st.Panel(title, t.shell.ViewSection())
	i := len(p.Sections) - 1 // before the view section, which is last or before Problems
	for j, sec := range p.Sections {
		if sec.ID == "view" {
			i = j
		}
	}
	if t.st.Loaded {
		p.Sections = slices.Insert(p.Sections, i, t.previewSection())
	}
	return p
}

func (t *Tool) previewSection() toolshell.Section {
	play := "Pause"
	if t.clock.Paused {
		play = "Play"
	}
	span := float64(scrubSeconds)
	var longest float32
	for _, em := range t.st.Ems {
		longest = max(longest, em.Duration)
	}
	if longest > 0 {
		span = float64(longest)
	}
	var tm float32
	if em := ecs.GetCompOrNil[gfx.Particles](t.shell.Engine().World(), t.effect); em != nil {
		tm = em.Time
	}
	bg := toolshell.Asset(idBGImage, "Background image", toolshell.AssetImage, t.bgImage, withCurrent(slices.Clone(t.st.Images), t.bgImage))
	return toolshell.Section{ID: "preview", Label: "Preview", Side: toolshell.SideLeft, Fields: []toolshell.Field{
		toolshell.Button(idPlay, play).WithHint("Enter"),
		toolshell.Button(idStep, "Step").WithHint("One tick (.)"),
		toolshell.Dropdown(idSpeed, "Speed", t.speed, speedNames...),
		toolshell.Number(idScrub, "Scrub", min(t.scrub, span)).Range(0, span).WithStep(0.05).WithDigits(2).WithHint("Jump to a time in the cycle"),
		toolshell.Label(idTime, "Time", fmt.Sprintf("%.2f s", tm)),
		toolshell.Checkbox(idOverlays, "Overlays", !t.noOverlays).WithHint("Spawn shape and direction cone"),
		toolshell.Checkbox(idChecker, "Checker", t.showChecker),
		bg,
	}}
}

// Set implements [toolshell.Tool].
func (t *Tool) Set(id string, v json.RawMessage) error {
	switch id {
	case idPlay:
		t.togglePlay()
	case idStep:
		t.step()
	case idSpeed:
		var i int
		if err := json.Unmarshal(v, &i); err != nil || i < 0 || i >= len(speedValues) {
			return fmt.Errorf("fx: no speed %s", v)
		}
		t.speed = i
		t.clock.Scale = speedValues[i]
	case idScrub:
		var f float64
		if err := json.Unmarshal(v, &f); err != nil || f < 0 || math.IsNaN(f) {
			return fmt.Errorf("fx: bad time %s", v)
		}
		t.scrub = f
		t.clock.SeekTo(float32(f))
	case idOverlays:
		var on bool
		if err := json.Unmarshal(v, &on); err != nil {
			return err
		}
		t.noOverlays = !on
	case idChecker:
		return json.Unmarshal(v, &t.showChecker)
	case idBGImage:
		var key string
		if err := json.Unmarshal(v, &key); err != nil {
			return err
		}
		if key != "" && (path.IsAbs(key) || path.Clean(key) != key || strings.HasPrefix(key, "../")) {
			return fmt.Errorf("fx: %q is not a relative asset key", key)
		}
		t.bgImage = key
	default:
		err := t.st.Set(id, v)
		if id == IDShader || id == IDApply {
			t.paramsKey = "\x00"
		}
		return err
	}
	return nil
}

// Draw implements [toolshell.Tool]: the emitter cross, the spawn shape and
// the direction cone in window pixels, then the toolbar.
func (t *Tool) Draw(s *toolshell.Shell, screen ebiten.FinalScreen) {
	if t.st == nil {
		return
	}
	t.bar.Draw(s, screen, func(i int) bool { return false })
	if t.st.Loaded {
		t.overlays(s, screen)
	}
	at := s.WindowRect(gfx.Rect{Min: t.pos(), Max: t.pos()}).Min
	white := color.RGBA{R: 255, G: 255, B: 255, A: 255}
	s.FillRect(screen, at.X-6, at.Y, 13, 1, white)
	s.FillRect(screen, at.X, at.Y-6, 1, 13, white)
}

// overlays draws the spawn shape and the cone around the emitter.
func (t *Tool) overlays(s *toolshell.Shell, screen ebiten.FinalScreen) {
	if t.noOverlays {
		return
	}
	e := t.st.Em
	pos := t.pos()
	to := func(local gfx.Vec2) gfx.Vec2 { return pos.Add(local) } // the preview entity is not rotated
	dot := func(p gfx.Vec2, c color.RGBA) {
		w := s.WindowRect(gfx.Rect{Min: p, Max: p}).Min
		s.FillRect(screen, w.X-1, w.Y-1, 2, 2, c)
	}
	line := func(a, b gfx.Vec2, c color.RGBA) {
		wa, wb := s.WindowRect(gfx.Rect{Min: a, Max: a}).Min, s.WindowRect(gfx.Rect{Min: b, Max: b}).Min
		n := int(math.Hypot(float64(wb.X-wa.X), float64(wb.Y-wa.Y))/3) + 1
		for i := 0; i <= n; i++ {
			f := float32(i) / float32(n)
			s.FillRect(screen, wa.X+(wb.X-wa.X)*f-1, wa.Y+(wb.Y-wa.Y)*f-1, 2, 2, c)
		}
	}
	shape := color.RGBA{R: 90, G: 200, B: 120, A: 200}
	cone := color.RGBA{R: 240, G: 200, B: 80, A: 200}
	switch e.Shape {
	case gfx.EmitCircle:
		const n = 48
		r := float64(e.ShapeSize.X)
		for i := range n {
			a := 2 * math.Pi * float64(i) / n
			dot(to(gfx.Vec2{X: float32(r * math.Cos(a)), Y: float32(r * math.Sin(a))}), shape)
		}
	case gfx.EmitRect:
		hw, hh := e.ShapeSize.X/2, e.ShapeSize.Y/2
		corners := []gfx.Vec2{{X: -hw, Y: -hh}, {X: hw, Y: -hh}, {X: hw, Y: hh}, {X: -hw, Y: hh}}
		for i := range corners {
			line(to(corners[i]), to(corners[(i+1)%4]), shape)
		}
	}
	// The cone: Angle is clockwise on screen from +X.
	length := float32(40) / max(s.Engine().Camera().Zoom, 0.01)
	for _, d := range []float64{-1, 0, 1} {
		a := float64(e.Angle) + d*float64(e.Spread)/2
		line(pos, to(gfx.Vec2{X: length * float32(math.Cos(a)), Y: length * float32(math.Sin(a))}), cone)
	}
}

// checkerPNG draws the checker texture: cells of 16 px in two grays.
func checkerPNG() []byte {
	img := image.NewRGBA(image.Rect(0, 0, checkerPx, checkerPx))
	dark, light := color.RGBA{R: 0x30, G: 0x30, B: 0x36, A: 255}, color.RGBA{R: 0x3c, G: 0x3c, B: 0x44, A: 255}
	for y := range checkerPx {
		for x := range checkerPx {
			c := dark
			if (x/checkerCell+y/checkerCell)%2 == 1 {
				c = light
			}
			img.SetRGBA(x, y, c)
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		panic("fxedit: " + err.Error())
	}
	return buf.Bytes()
}
