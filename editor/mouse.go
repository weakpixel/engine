package editor

import (
	"image"
	"math"
	"slices"

	"github.com/weakpixel/ecs"
	"github.com/weakpixel/engine/editor/camzoom"
	"github.com/weakpixel/engine/extract"
	"github.com/weakpixel/engine/gfx"
	"github.com/weakpixel/engine/input"
)

// dragMode is what the left button is doing.
type dragMode uint8

const (
	dragNone   dragMode = iota
	dragMove            // the objects of items follow the cursor
	dragRotate          // the one object of items turns around its position
	dragScale           // the one object of items scales around its position
	dragBox             // a rubber band over empty space selects what it touches
)

// dragItem is an object being moved, with its Pos when the drag began.
type dragItem struct {
	id    string
	start gfx.Vec2
}

// mouseState is what the session remembers about the mouse between ticks.
type mouseState struct {
	buttons uint8    // last tick's buttons
	pos     gfx.Vec2 // last tick's cursor, logical pixels
	mode    dragMode
	// items are the dragged objects; items[0] is the one that was grabbed,
	// the one the snap grid rounds. A move puts each at its start plus the
	// cursor's movement since grab (a world position), limited to the axes
	// in axes; moved is set once it left start.
	items []dragItem
	grab  gfx.Vec2
	axes  gfx.Vec2
	moved bool
	// narrowTo is the object a press on a selected object of several hit:
	// released without a drag, the selection narrows to it.
	narrowTo string
	// startRot and startScale are the Transform's Rot and effective Scale
	// when a rotate or scale drag began; grabAngle and grabDist are the
	// cursor's angle and distance from the object then (window pixels).
	startRot, grabAngle float32
	startScale          gfx.Vec2
	grabDist            float32
	boxFrom, boxTo      gfx.Vec2     // the rubber band's corners, world units
	boxAdd              bool         // the band adds to the selection
	zoom                camzoom.Zoom // eases the wheel zoom
}

// bothAxes is the drag mask of a free drag.
var bothAxes = gfx.Vec2{X: 1, Y: 1}

// screenSize is the logical screen size the camera maps to.
func (s *Session) screenSize() image.Point {
	if sc := ecs.GetResourceOrNil[extract.Screen](s.e.World()); sc != nil {
		return sc.Size
	}
	return image.Point{}
}

// handleMouse applies one tick of mouse input in edit mode: left button
// picks, selects, drags, uses the gizmos and draws a selection box; the middle button, or the left button with Space held, pans; the wheel zooms
// around the cursor.
func (s *Session) handleMouse(m *input.MouseFrame) {
	ms := &s.mouse
	cam := s.e.Camera()
	size := s.screenSize()
	cur := gfx.Vec2{X: m.X, Y: m.Y}
	world := cam.ScreenToWorld(cur, size)
	pressed := func(b input.MouseButton) bool { return m.Down(b) && ms.buttons&(1<<b) == 0 }

	// A left press with Space held starts a pan, not a pick or a drag; a drag
	// that began before Space went down is not interrupted.
	spacePan := s.space && m.Down(input.MouseLeft) && ms.mode == dragNone
	switch {
	case spacePan:
	case pressed(input.MouseLeft):
		s.press(cur, world)
	case m.Down(input.MouseLeft) && ms.mode != dragNone:
		s.dragTo(cur, world)
	case !m.Down(input.MouseLeft) && ms.mode != dragNone:
		s.endDrag()
	}

	if (m.Down(input.MouseMiddle) && ms.buttons&(1<<input.MouseMiddle) != 0) ||
		(spacePan && ms.buttons&(1<<input.MouseLeft) != 0) {
		// The world point under the previous cursor moves to the current one.
		cam.Pos = cam.Pos.Add(cam.ScreenToWorld(ms.pos, size).Sub(world))
	}
	ms.zoom.Wheel(cam, m.WheelY)
	ms.zoom.Update(cam, cur, size)
	ms.buttons, ms.pos = m.Buttons, cur
}

// transformOf returns the Transform of the object with id, or nil. For an
// attached object it is the derived world Transform; a drag sets it and
// [Session.syncLocal] converts the result into its Local.
func (s *Session) transformOf(id string) *gfx.Transform {
	o := s.objs[id]
	if o == nil || !s.e.World().IsAlive(o.entity) {
		return nil
	}
	return ecs.GetCompOrNil[gfx.Transform](s.e.World(), o.entity)
}

// press handles a left press at the logical position cur (world position
// world): a gizmo of the single selected object, an object (selected,
// toggled with a modifier, and dragged), or empty space (a selection box).
func (s *Session) press(cur, world gfx.Vec2) {
	ms := &s.mouse
	// The gizmos are in front of every object.
	if g, ok := s.pickGizmo(s.e.LogicalToWindow(cur)); ok {
		s.beginGizmo(g, cur, world)
		return
	}
	id, ok := s.Pick(world)
	if s.alt {
		// Alt selects the whole instance, not the part under the cursor.
		id = s.rootOfPart(id)
	}
	if !ok {
		ms.mode, ms.boxFrom, ms.boxTo, ms.boxAdd = dragBox, world, world, s.mod
		return
	}
	switch {
	case s.mod:
		if i := slices.Index(s.selection, id); i >= 0 {
			s.setSelection(slices.Delete(slices.Clone(s.selection), i, i+1)...)
			return
		}
		s.setSelection(append(slices.Clone(s.selection), id)...)
		s.beginMove(world, bothAxes, id, s.selection)
	case slices.Contains(s.selection, id):
		s.beginMove(world, bothAxes, id, s.selection)
		if len(s.selection) > 1 {
			ms.narrowTo = id
		}
	default:
		s.setSelection(id)
		s.beginMove(world, bothAxes, id, s.selection)
	}
}

// beginMove starts moving the objects ids (that have a Transform), grabbed
// at the world position grab by the object first, along the axes of the
// mask (1 moves, 0 holds that axis).
func (s *Session) beginMove(grab, axes gfx.Vec2, first string, ids []string) {
	ms := &s.mouse
	ms.mode, ms.grab, ms.axes, ms.moved = dragMove, grab, axes, false
	ms.items = ms.items[:0]
	all := append([]string{first}, ids...)
	for _, id := range all {
		// An object whose ancestor moves too follows it.
		if s.hasAncestorIn(id, all) {
			continue
		}
		if t := s.transformOf(id); t != nil && !slices.ContainsFunc(ms.items, func(it dragItem) bool { return it.id == id }) {
			ms.items = append(ms.items, dragItem{id, t.Pos})
		}
	}
}

// beginGizmo starts the drag of gizmo g of the selected object, pressed at
// the logical position cur (world position world).
func (s *Session) beginGizmo(g gizmo, cur, world gfx.Vec2) {
	ms := &s.mouse
	id := s.selection[0]
	switch g {
	case gizmoMoveX:
		s.beginMove(world, gfx.Vec2{X: 1}, id, []string{id})
	case gizmoMoveY:
		s.beginMove(world, gfx.Vec2{Y: 1}, id, []string{id})
	case gizmoRotate, gizmoScale:
		t := s.transformOf(id)
		o, _ := s.handleOrigin()
		d := s.e.LogicalToWindow(cur).Sub(o)
		ms.mode, ms.moved = dragRotate, false
		if g == gizmoScale {
			ms.mode = dragScale
		}
		ms.items = append(ms.items[:0], dragItem{id, t.Pos})
		ms.startRot, ms.startScale = t.Rot, scaleOrOne(t.Scale)
		ms.grabAngle = float32(math.Atan2(float64(d.Y), float64(d.X)))
		ms.grabDist = max(d.Len(), 1)
	}
}

// scaleOrOne returns v, or (1, 1) for the zero Scale.
func scaleOrOne(v gfx.Vec2) gfx.Vec2 {
	if v == (gfx.Vec2{}) {
		return gfx.Vec2{X: 1, Y: 1}
	}
	return v
}

// dragTo continues the left drag with the cursor at the logical position cur
// (world position world).
func (s *Session) dragTo(cur, world gfx.Vec2) {
	ms := &s.mouse
	if ms.mode == dragBox {
		ms.boxTo = world
		return
	}
	if len(ms.items) == 0 {
		return
	}
	t := s.transformOf(ms.items[0].id)
	if t == nil {
		return
	}
	switch ms.mode {
	case dragMove:
		s.moveTo(world)
	case dragRotate:
		o, _ := s.handleOrigin()
		d := s.e.LogicalToWindow(cur).Sub(o)
		rot := ms.startRot + float32(math.Atan2(float64(d.Y), float64(d.X))) - ms.grabAngle
		if s.shift {
			rot = snapTo(rot, rotateStep)
		}
		t.Rot = rot
		s.dragged(ms.items[0].id)
		ms.moved = ms.moved || rot != ms.startRot
	case dragScale:
		o, _ := s.handleOrigin()
		ratio := s.e.LogicalToWindow(cur).Sub(o).Len() / ms.grabDist
		sc := ms.startScale.Mul(ratio)
		if s.shift {
			sc = gfx.Vec2{X: snapTo(sc.X, scaleStep), Y: snapTo(sc.Y, scaleStep)}
		} else {
			sc = gfx.Vec2{X: snapTo(sc.X, 0.001), Y: snapTo(sc.Y, 0.001)}
		}
		// A zero Scale would read as (1, 1): keep a sliver instead.
		if sc.X == 0 {
			sc.X = scaleStep / 25
		}
		if sc.Y == 0 {
			sc.Y = scaleStep / 25
		}
		t.Scale = sc
		s.dragged(ms.items[0].id)
		ms.moved = ms.moved || sc != ms.startScale
	}
}

// Steps of the shift-snapped rotate (15 degrees) and scale gizmos.
const (
	rotateStep = math.Pi / 12
	scaleStep  = 0.25
)

// moveTo puts the dragged objects where the cursor at the world position
// cur takes them: start plus the movement since the grab, on the allowed
// axes. The grabbed object is rounded to the snap grid unless Alt is held,
// and the others keep their offset to it. PrevPos moves with Pos, so the
// paused frame shows the new place without blending.
func (s *Session) moveTo(cur gfx.Vec2) {
	ms := &s.mouse
	first := ms.items[0]
	d := cur.Sub(ms.grab)
	want := first.start
	if ms.axes.X != 0 {
		want.X += d.X
	}
	if ms.axes.Y != 0 {
		want.Y += d.Y
	}
	if s.snap > 0 && !s.alt {
		if ms.axes.X != 0 {
			want.X = snapTo(want.X, s.snap)
		}
		if ms.axes.Y != 0 {
			want.Y = snapTo(want.Y, s.snap)
		}
	}
	delta := want.Sub(first.start)
	for _, it := range ms.items {
		t := s.transformOf(it.id)
		if t == nil {
			continue
		}
		if p := it.start.Add(delta); p != t.Pos {
			t.PrevPos = t.PrevPos.Add(p.Sub(t.Pos))
			t.Pos = p
			s.dragged(it.id)
		}
	}
	ms.moved = ms.moved || delta != (gfx.Vec2{})
}

// snapTo rounds v to the nearest multiple of grid.
func snapTo(v, grid float32) float32 {
	return float32(math.Round(float64(v/grid))) * grid
}

// cancelDrag drops the drag in progress without reporting anything.
func (s *Session) cancelDrag() {
	ms := &s.mouse
	ms.mode, ms.moved, ms.narrowTo, ms.items = dragNone, false, "", ms.items[:0]
}

// endDrag finishes a drag. A box selects what it touches. If objects moved,
// rotated or scaled, it emits one Edit per object with its whole Transform
// (its Transform override for a prefab instance). The session records that
// JSON as applied, so the shell sending the edited document back changes
// nothing. A press on a selected object of several that never moved narrows
// the selection to it. An attached object reports its Parent, whose Local
// the drag changed, instead of its Transform.
func (s *Session) endDrag() {
	ms := &s.mouse
	mode, moved, narrow := ms.mode, ms.moved, ms.narrowTo
	items := slices.Clone(ms.items)
	s.cancelDrag()
	switch {
	case mode == dragBox:
		s.finishBox()
	case !moved:
		if narrow != "" {
			s.setSelection(narrow)
		}
	default:
		for _, it := range items {
			s.emitPlacement(it.id)
		}
	}
}

// boxMinSize is the side, in window pixels, below which a selection box
// counts as a click on empty space.
const boxMinSize = 3

// finishBox selects the objects whose bounds the rubber band touches (added
// to the selection when a modifier was held), or clears the selection for a
// band so small it is a click.
func (s *Session) finishBox() {
	ms := &s.mouse
	wr := s.windowRect(normRect(ms.boxFrom, ms.boxTo))
	if wr.Max.X-wr.Min.X < boxMinSize && wr.Max.Y-wr.Min.Y < boxMinSize {
		if !ms.boxAdd {
			s.setSelection()
		}
		return
	}
	box := normRect(ms.boxFrom, ms.boxTo)
	var ids []string
	if ms.boxAdd {
		ids = slices.Clone(s.selection)
	}
	for _, id := range s.order {
		if r, ok := s.Bounds(id); ok && r.Min.X <= box.Max.X && box.Min.X <= r.Max.X && r.Min.Y <= box.Max.Y && box.Min.Y <= r.Max.Y && !slices.Contains(ids, id) {
			ids = append(ids, id)
		}
	}
	s.setSelection(ids...)
}

// rootOfPart returns the id of the instance root when id is a part of a
// multi-object prefab instance, else id: clicking a part in the viewport
// selects its instance.
func (s *Session) rootOfPart(id string) string {
	if o := s.objs[id]; o != nil && o.part != "" && s.objs[o.root] != nil {
		return o.root
	}
	return id
}

// emitTransform reports the Transform of object id as an Edit.
func (s *Session) emitTransform(id string) { s.emitComponent(id, TransformKey) }

// emitComponent reports the component key of object id as an Edit.
func (s *Session) emitComponent(id, key string) {
	raw, err := s.encode(id, key)
	if err != nil {
		return // no such component on the object: nothing to report
	}
	o := s.objs[id]
	if o == nil {
		return
	}
	o.comps[key] = compactJSON(raw)
	ev := Edit{ID: id, Component: key, JSON: raw}
	if o.part != "" {
		ev.ID, ev.Part = o.root, o.part
	}
	if o.prefab != "" {
		// An instance keeps only what differs from its prefab.
		if ov, err := s.override(o, key, raw); err == nil {
			ev.JSON, ev.Override = ov, true
		}
	}
	s.emit(ev)
}
