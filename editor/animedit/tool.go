package animedit

import (
	"encoding/json"
	"fmt"
	"image/color"
	"io/fs"
	"path"
	"slices"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/weakpixel/asset"
	"github.com/weakpixel/ecs"
	"github.com/weakpixel/engine/anim"
	"github.com/weakpixel/engine/anim/animfile"
	"github.com/weakpixel/engine/editor/toolshell"
	"github.com/weakpixel/engine/gfx"
	"github.com/weakpixel/engine/input"
)

// Colors of the overlay.
var (
	colHit      = color.RGBA{R: 255, G: 70, B: 70, A: 255}
	colHurt     = color.RGBA{R: 70, G: 220, B: 90, A: 255}
	colBody     = color.RGBA{R: 80, G: 150, B: 255, A: 255}
	colSelected = color.RGBA{R: 255, G: 230, B: 60, A: 255}
	colFrame    = color.RGBA{R: 140, G: 140, B: 150, A: 255}
	colPivot    = color.RGBA{R: 255, G: 255, B: 255, A: 255}
)

const handlePx = 6 // size of a resize handle in window pixels

// Tool is the anim tool. It implements [toolshell.DocumentTool],
// [toolshell.Syncer] and the shell's optional Check.
type Tool struct {
	shell *toolshell.Shell
	key   string
	st    *State

	entity   ecs.Entity
	sheetKey string
	set      anim.ClipSetHandle
	setVer   uint32
	prevBtn  uint8
	framed   bool
	bar      *toolshell.Toolbar
	onBar    bool // the press in progress started on the toolbar
	// scope holds the clip set; nil is the global scope.
	scope *asset.Scope
}

// New returns the anim tool.
func New() toolshell.Tool { return &Tool{bar: newToolbar()} }

// NewPreview returns the tool for the preview of an Aseprite export, which
// switches files: it loads the clip set into scope, which the caller
// releases, and [Tool.Close] removes its entity.
func NewPreview(scope *asset.Scope) *Tool {
	return &Tool{bar: newToolbar(), scope: scope}
}

// Close removes the entity Open spawned. The preview calls it before it
// switches files; the editor never does, since its window or page shows one
// document.
func (t *Tool) Close(s *toolshell.Shell) {
	if w := s.Engine().World(); t.entity != (ecs.Entity{}) && w.IsAlive(t.entity) {
		w.DeleteEntity(t.entity)
	}
	t.entity, t.sheetKey, t.set, t.setVer = ecs.Entity{}, "", anim.ClipSetHandle{}, 0
}

// AsepriteSuffixes are the name suffixes of an Aseprite export that the
// tool opens read-only to play its clips.
var AsepriteSuffixes = []string{".aseprite.json", ".ase.json"}

// IsAsepriteExport reports whether key names an Aseprite export by one of
// [AsepriteSuffixes].
func IsAsepriteExport(key string) bool {
	return slices.ContainsFunc(AsepriteSuffixes, func(s string) bool { return strings.HasSuffix(key, s) })
}

// Check reports whether the tool can open the file at asset key.
func (*Tool) Check(key string) error {
	if !strings.HasSuffix(key, animfile.Suffix) && !IsAsepriteExport(key) {
		return fmt.Errorf("anim: %s is not a %s, %s or %s file", key, animfile.Suffix, AsepriteSuffixes[0], AsepriteSuffixes[1])
	}
	return nil
}

// Open implements [toolshell.Tool].
func (t *Tool) Open(s *toolshell.Shell, key string) error {
	if err := t.Check(key); err != nil {
		return err
	}
	t.shell, t.key = s, key
	t.st = NewState(s.Edit)
	t.st.ReadOnly = IsAsepriteExport(key)
	t.entity = ecs.NewEntity2(s.Engine().World(), gfx.Transform{}, gfx.Sprite{})
	return nil
}

// Load implements [toolshell.DocumentTool]: a new version of the document.
// One that does not parse leaves the shown file as it is and marks it stale.
func (t *Tool) Load(text []byte) error {
	if t.st.ReadOnly {
		// The document is the Aseprite export itself: an empty definition
		// of it plays every tag with the sheet's own timing. The sheet is
		// read through the loaders, not from text, like the sheet of a
		// definition.
		t.st.SetFile(&animfile.File{Version: animfile.Version, Sheet: path.Base(t.key)})
		if t.sheetKey != t.key {
			t.loadSheet(t.key)
		}
		return nil
	}
	f, err := animfile.Decode(text)
	if err != nil {
		t.st.MarkStale(err)
		return fmt.Errorf("anim: %w", err)
	}
	if key := path.Join(path.Dir(t.key), f.Sheet); key != t.sheetKey {
		t.loadSheet(key)
	}
	t.st.SetFile(f)
	return nil
}

// loadSheet reads the sheet's JSON for the tags and loads it as a clip set
// for the sprites. The definition itself never goes through the asset loader:
// it rejects an invalid file, and a half-edited one must still show.
func (t *Tool) loadSheet(key string) {
	t.sheetKey = key
	t.set, t.setVer = anim.ClipSetHandle{}, 0
	data, err := t.readSheet(key)
	var sheet *animfile.Sheet
	if err == nil {
		sheet, err = animfile.DecodeSheet(data)
	}
	if err == nil {
		scope := t.scope
		if scope == nil {
			scope = t.shell.Engine().Assets().Global()
		}
		t.set, err = anim.LoadClipSetSync(scope, key)
	}
	t.st.SetSheet(sheet, err)
	if t.set != (anim.ClipSetHandle{}) {
		t.setVer = t.shell.Engine().Stores().ClipSets().Generic().Version(assetHandle(t.set))
	}
}

func (t *Tool) readSheet(key string) ([]byte, error) {
	if t.shell.FS() == nil {
		return nil, fmt.Errorf("no asset filesystem")
	}
	return fs.ReadFile(t.shell.FS(), key)
}

// Update implements [toolshell.Tool]: the keys and the mouse in the viewport.
func (t *Tool) Update(s *toolshell.Shell) {
	st := t.st
	if st == nil || !st.Ready() {
		return
	}
	t.keys(s)
	if st.ReadOnly {
		return // nothing to draw or drag
	}
	m := s.Mouse()
	down, wasDown := m.Buttons&1 != 0, t.prevBtn&1 != 0
	t.prevBtn = m.Buttons
	panning := s.PanHeld()
	p := t.fileSpace(s.MouseWorld())
	var err error
	switch {
	case down && !wasDown && t.bar.Over(s, m.X, m.Y):
		t.onBar = true
		if k, ok := t.toolButtonAt(s, m.X, m.Y); ok {
			st.Tool = k
		}
	case down && !wasDown && !panning:
		err = st.PointerDown(p, t.tolerance(s))
	case down && st.Drag != nil && !t.onBar:
		st.PointerMove(p)
	case !down && st.Drag != nil:
		err = st.PointerUp(p)
	}
	if err != nil {
		s.ReportError(err)
	}
	if !down {
		t.onBar = false
	}
	if t.bar.Over(s, m.X, m.Y) {
		ebiten.SetCursorShape(ebiten.CursorShapePointer)
		return
	}
	t.cursor(s, p)
}

// fileSpace maps a world point to file space: a flipped view is mirrored.
func (t *Tool) fileSpace(p gfx.Vec2) gfx.Vec2 {
	if t.st.Flip {
		p.X = -p.X
	}
	return p
}

// tolerance is the size of a handle in world units.
func (t *Tool) tolerance(s *toolshell.Shell) float32 {
	return handlePx / max(s.Engine().Camera().Zoom, 0.01)
}

func (t *Tool) keys(s *toolshell.Shell) {
	if s.Down(input.KeyControlLeft) || s.Down(input.KeyControlRight) || s.Down(input.KeyMetaLeft) || s.Down(input.KeyMetaRight) {
		return
	}
	st := t.st
	var err error
	alt := s.Down(input.KeyAltLeft) || s.Down(input.KeyAltRight)
	step := int32(1)
	if s.Down(input.KeyShiftLeft) || s.Down(input.KeyShiftRight) {
		step = 8
	}
	for k, tool := range map[input.Key]ToolKind{input.KeyS: ToolSelect, input.KeyP: ToolPivot, input.KeyH: ToolHit, input.KeyU: ToolHurt, input.KeyB: ToolBody} {
		if s.Pressed(k) && !st.ReadOnly {
			st.Tool = tool
		}
	}
	alt = alt && !st.ReadOnly // Alt+arrows nudge a box: only a definition has one
	switch {
	case s.Pressed(input.KeyEnter):
		err = st.Set("play", nil)
	case s.Pressed(input.KeyComma):
		st.Play.Step(st.timeline(), -1)
	case s.Pressed(input.KeyPeriod):
		st.Play.Step(st.timeline(), 1)
	case s.Pressed(input.KeyBracketLeft):
		st.SelectClip(st.Clip - 1)
	case s.Pressed(input.KeyBracketRight):
		st.SelectClip(st.Clip + 1)
	case (s.Pressed(input.KeyDelete) || s.Pressed(input.KeyBackspace)) && !st.ReadOnly:
		err = st.DeleteSelected()
	case s.Pressed(input.KeyX):
		st.Flip = !st.Flip
	case alt && s.Pressed(input.KeyArrowLeft):
		err = st.Nudge(-step, 0)
	case alt && s.Pressed(input.KeyArrowRight):
		err = st.Nudge(step, 0)
	case alt && s.Pressed(input.KeyArrowUp):
		err = st.Nudge(0, -step)
	case alt && s.Pressed(input.KeyArrowDown):
		err = st.Nudge(0, step)
	case s.Pressed(input.KeyArrowLeft):
		st.Play.Step(st.timeline(), -1)
	case s.Pressed(input.KeyArrowRight):
		st.Play.Step(st.timeline(), 1)
	case s.Pressed(input.KeyArrowUp):
		st.SelectClip(st.Clip - 1)
	case s.Pressed(input.KeyArrowDown):
		st.SelectClip(st.Clip + 1)
	}
	if err != nil {
		s.ReportError(err)
	}
}

// cursor shows what a click would do.
func (t *Tool) cursor(s *toolshell.Shell, p gfx.Vec2) {
	st := t.st
	shape := ebiten.CursorShapeDefault
	switch st.Tool {
	case ToolSelect:
		h := HandleNone
		if st.Drag != nil && st.Drag.Kind == DragResize {
			h = st.Drag.Handle
		} else if st.Sel != nil {
			if b, ok := findBox(st.Boxes(), *st.Sel); ok {
				h = HandleAt(b.Rect, p, t.tolerance(s))
			}
		}
		shape = cursorOf(h)
	default:
		shape = ebiten.CursorShapeCrosshair
	}
	ebiten.SetCursorShape(shape)
}

func cursorOf(h Handle) ebiten.CursorShapeType {
	switch h {
	case HandleN, HandleS:
		return ebiten.CursorShapeNSResize
	case HandleE, HandleW:
		return ebiten.CursorShapeEWResize
	case HandleNW, HandleSE:
		return ebiten.CursorShapeNWSEResize
	case HandleNE, HandleSW:
		return ebiten.CursorShapeNESWResize
	}
	return ebiten.CursorShapeDefault
}

// Sync implements [toolshell.Syncer]: it advances playback, notices a
// reloaded sheet and puts the current frame's sprite on the entity.
func (t *Tool) Sync(s *toolshell.Shell) {
	st := t.st
	if st == nil {
		return
	}
	t.checkSheet()
	if !st.Ready() {
		return
	}
	st.Play.Advance(st.timeline(), TickDuration)
	e := s.Engine()
	sp := ecs.GetCompOrNil[gfx.Sprite](e.World(), t.entity)
	if sp == nil {
		return
	}
	h := t.frameSprite()
	rs := e.Stores().Sprites().Get(h)
	st.Geom = Geom{Size: rs.Size, Offset: rs.Offset, Trim: gfx.Vec2{X: float32(rs.Rect.Dx()), Y: float32(rs.Rect.Dy())}, Origin: rs.Origin}
	sp.ID, sp.FlipX = h, st.Flip
	sp.Origin = gfx.Vec2{}
	if p := st.Pivot(); p != nil && rs.Size.X > 0 && rs.Size.Y > 0 {
		sp.Origin = anim.PivotOrigin(rs, *p)
	}
	s.SetContent(t.contentRect())
	if !t.framed && h != (gfx.SpriteHandle{}) {
		t.framed = true
		s.Frame(t.contentRect())
	}
}

// checkSheet reloads the tags when the sheet's JSON changed on disk.
func (t *Tool) checkSheet() {
	if t.set == (anim.ClipSetHandle{}) {
		return
	}
	store := t.shell.Engine().Stores().ClipSets().Generic()
	if v := store.Version(assetHandle(t.set)); v != t.setVer {
		t.loadSheet(t.sheetKey)
	}
}

// frameSprite is the sprite of the frame shown, the zero handle (the
// placeholder) when the clip set is not ready.
func (t *Tool) frameSprite() gfx.SpriteHandle {
	if t.set == (anim.ClipSetHandle{}) {
		return gfx.SpriteHandle{}
	}
	srv := t.shell.Engine().Assets()
	set := anim.ClipSetStoreOf(srv).Get(t.set)
	if set == nil {
		return gfx.SpriteHandle{}
	}
	ch, ok := set.Clips[t.st.ClipName()]
	if !ok {
		return gfx.SpriteHandle{}
	}
	c := anim.ClipStoreOf(srv).Get(ch)
	if c == nil || t.st.Play.Frame >= len(c.Frames) {
		return gfx.SpriteHandle{}
	}
	return c.Frames[t.st.Play.Frame].Sprite
}

// spriteRect is the trimmed image of the shown frame in world (pivot)
// space. Without a pivot the sprite is drawn around its default origin.
func (t *Tool) spriteRect() gfx.Rect {
	g := t.st.Geom
	ox, oy := g.Origin.X*g.Size.X-g.Offset.X, g.Origin.Y*g.Size.Y-g.Offset.Y
	if p := t.st.Pivot(); p != nil {
		ox, oy = float32(p.X), float32(p.Y)
	}
	r := gfx.Rect{Min: gfx.Vec2{X: -ox, Y: -oy}, Max: gfx.Vec2{X: g.Trim.X - ox, Y: g.Trim.Y - oy}}
	if t.st.Flip {
		r.Min.X, r.Max.X = -r.Max.X, -r.Min.X
	}
	return r
}

// contentRect is what Fit frames: the sprite and the origin.
func (t *Tool) contentRect() gfx.Rect {
	r := t.spriteRect()
	if r.Max.X <= r.Min.X || r.Max.Y <= r.Min.Y {
		return gfx.Rect{Min: gfx.Vec2{X: -16, Y: -16}, Max: gfx.Vec2{X: 16, Y: 16}}
	}
	return r
}

// Panel implements [toolshell.Tool].
func (t *Tool) Panel() toolshell.Panel {
	return t.st.Panel(path.Base(t.key), t.shell.ViewSection())
}

// Set implements [toolshell.Tool].
func (t *Tool) Set(id string, v json.RawMessage) error {
	if id == "clip.sel" {
		defer func() { t.shell.SetContent(t.contentRect()) }()
	}
	return t.st.Set(id, v)
}

// Draw implements [toolshell.Tool]: the overlay in window pixels.
func (t *Tool) Draw(s *toolshell.Shell, screen ebiten.FinalScreen) {
	st := t.st
	if st == nil || !st.Ready() {
		return
	}
	fade := uint8(255)
	if st.Stale != "" {
		fade = 90
	}
	line := func(r gfx.Rect, c color.RGBA, w float32) {
		w0 := s.WindowRect(r)
		x, y, ww, hh := w0.Min.X, w0.Min.Y, w0.Max.X-w0.Min.X, w0.Max.Y-w0.Min.Y
		c = scale(c, fade)
		s.FillRect(screen, x, y, ww, w, c)
		s.FillRect(screen, x, y+hh-w, ww, w, c)
		s.FillRect(screen, x, y, w, hh, c)
		s.FillRect(screen, x+ww-w, y, w, hh, c)
	}
	fill := func(r gfx.Rect, c color.RGBA) {
		w0 := s.WindowRect(r)
		s.FillRect(screen, w0.Min.X, w0.Min.Y, w0.Max.X-w0.Min.X, w0.Max.Y-w0.Min.Y, scale(c, uint8(int(fade)*40/255)))
	}
	view := func(r animfile.Rect) gfx.Rect {
		if st.Flip {
			r = Flip(r)
		}
		return ToRect(r)
	}

	line(t.spriteRect(), colFrame, 1)
	for _, b := range st.Boxes() {
		r := b.Rect
		if d := st.Drag; d != nil && (d.Kind == DragMove || d.Kind == DragResize) && d.Ref == b.Ref {
			r = d.Rect
		}
		c := kindColor(b.Kind)
		if b.Replaced {
			c = scale(c, 70)
		}
		fill(view(r), c)
		line(view(r), c, 1)
	}
	if d := st.Drag; d != nil && d.Kind == DragDraw {
		line(view(d.Rect), kindColor(d.Box), 1)
	}
	if st.Sel != nil {
		if b, ok := findBox(st.Boxes(), *st.Sel); ok {
			r := b.Rect
			if d := st.Drag; d != nil && (d.Kind == DragMove || d.Kind == DragResize) {
				r = d.Rect
			}
			line(view(r), colSelected, 2)
			t.handles(s, screen, view(r))
		}
	}
	at := gfx.Vec2{}
	if d := st.Drag; d != nil && d.Kind == DragPivot {
		at = gfx.Vec2{X: float32(d.At.X), Y: float32(d.At.Y)}
		if st.Flip {
			at.X = -at.X
		}
	}
	if st.Pivot() != nil || (st.Drag != nil && st.Drag.Kind == DragPivot) {
		t.cross(s, screen, at, scale(colPivot, fade))
	}
	if !st.ReadOnly {
		t.drawToolbar(s, screen)
	}
}

// handles draws the eight resize handles of the selected box.
func (t *Tool) handles(s *toolshell.Shell, screen ebiten.FinalScreen, r gfx.Rect) {
	w := s.WindowRect(r)
	xm, ym := (w.Min.X+w.Max.X)/2, (w.Min.Y+w.Max.Y)/2
	for _, p := range [][2]float32{
		{w.Min.X, w.Min.Y}, {xm, w.Min.Y}, {w.Max.X, w.Min.Y}, {w.Max.X, ym},
		{w.Max.X, w.Max.Y}, {xm, w.Max.Y}, {w.Min.X, w.Max.Y}, {w.Min.X, ym},
	} {
		s.FillRect(screen, p[0]-handlePx/2, p[1]-handlePx/2, handlePx, handlePx, colSelected)
	}
}

// cross draws the pivot cross at a world point.
func (t *Tool) cross(s *toolshell.Shell, screen ebiten.FinalScreen, at gfx.Vec2, c color.RGBA) {
	w := s.WindowRect(gfx.Rect{Min: at, Max: at})
	x, y := w.Min.X, w.Min.Y
	s.FillRect(screen, x-6, y, 13, 1, c)
	s.FillRect(screen, x, y-6, 1, 13, c)
}

func kindColor(kind string) color.RGBA {
	switch kind {
	case KindHit:
		return colHit
	case KindHurt:
		return colHurt
	}
	return colBody
}

// scale returns c with its alpha multiplied by a/255 (premultiplied).
func scale(c color.RGBA, a uint8) color.RGBA {
	f := func(v uint8) uint8 { return uint8(int(v) * int(a) / 255) }
	return color.RGBA{R: f(c.R), G: f(c.G), B: f(c.B), A: f(c.A)}
}

func assetHandle(h anim.ClipSetHandle) asset.Handle[anim.ClipSet] {
	return asset.HandleOf[anim.ClipSet](h.Ref)
}
