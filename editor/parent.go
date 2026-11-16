package editor

import (
	"image/color"
	"slices"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/weakpixel/ecs"
	"github.com/weakpixel/engine/gfx"
	"github.com/weakpixel/engine/hierarchy"
)

// ParentKey is the component key of hierarchy.Parent. The handles of an
// object that has one edit its Local (see [Session.syncLocal]) instead of
// the Transform, which the hierarchy stage derives every tick.
const ParentKey = "hierarchy:Parent"

// parentOf returns the live Parent component of the object with id and the
// world Transform of the entity it is attached to, as of the last tick. ok
// is false for an object without a Parent, or whose parent is gone or has no
// Transform.
func (s *Session) parentOf(id string) (p *hierarchy.Parent, parent gfx.Transform, ok bool) {
	o := s.objs[id]
	if o == nil || !s.e.World().IsAlive(o.entity) {
		return nil, gfx.Transform{}, false
	}
	w := s.e.World()
	p = ecs.GetCompOrNil[hierarchy.Parent](w, o.entity)
	if p == nil {
		return nil, gfx.Transform{}, false
	}
	pt := ecs.GetCompOrNil[gfx.Transform](w, p.Entity)
	if pt == nil {
		return nil, gfx.Transform{}, false
	}
	return p, *pt, true
}

// syncLocal makes the Local of the dragged object id follow the Transform
// the drag just set: the field the current drag changes (Pos for a move, Rot
// for a rotate, Scale for a scale) is converted into the parent's space with
// [hierarchy.LocalOf]. The other fields are left alone, so a move does not
// write float noise into Rot and Scale. The hierarchy stage composes the
// same Transform back on the next tick. Objects without a Parent are not
// touched.
func (s *Session) syncLocal(id string) {
	p, parent, ok := s.parentOf(id)
	if !ok {
		return
	}
	l := hierarchy.LocalOf(parent, *s.transformOf(id))
	switch s.mouse.mode {
	case dragMove:
		p.Local.Pos = l.Pos
	case dragRotate:
		p.Local.Rot = l.Rot
	case dragScale:
		p.Local.Scale = l.Scale
	}
}

// dragged follows a drag step that changed the Transform of object id: its
// Local follows ([Session.syncLocal]), and its [hierarchy.PlaceAbsolute]
// descendants are placed from their Local again, so that in edit mode they
// move with the object like the rest of its group.
func (s *Session) dragged(id string) {
	s.syncLocal(id)
	s.repositionAbsolute(id)
}

// repositionAbsolute calls [hierarchy.Reposition] on every PlaceAbsolute
// descendant of the object id. It scans every Parent of the World: a tool
// path, run once per drag step.
func (s *Session) repositionAbsolute(id string) {
	o := s.objs[id]
	if o == nil {
		return
	}
	w := s.e.World()
	for e, p := range ecs.NewView1[hierarchy.Parent](w).Each() {
		if p.Local.PlaceMode == hierarchy.PlaceAbsolute && descendsFrom(w, e, o.entity) {
			// Reposition writes a field of the component in place: no
			// structural change inside the Each.
			hierarchy.Reposition(w, e)
		}
	}
}

// descendsFrom reports whether anc is an ancestor of e.
func descendsFrom(w *ecs.World, e, anc ecs.Entity) bool {
	for range hierarchy.MaxDepth {
		p := ecs.GetCompOrNil[hierarchy.Parent](w, e)
		if p == nil {
			return false
		}
		if p.Entity == anc {
			return true
		}
		e = p.Entity
	}
	return false
}

// emitPlacement reports the result of a drag of object id: its Parent for an
// attached object, else its Transform.
func (s *Session) emitPlacement(id string) {
	if _, _, ok := s.parentOf(id); ok {
		s.emitComponent(id, ParentKey)
		return
	}
	s.emitComponent(id, TransformKey)
}

// idOfEntity returns the id of the object that is entity e, or "".
func (s *Session) idOfEntity(e ecs.Entity) string {
	for id, o := range s.objs {
		if o.entity == e {
			return id
		}
	}
	return ""
}

// hasAncestorIn reports whether an object that id is attached to, directly or
// through other attached objects, is in ids. A drag skips such an object: it
// follows its ancestor.
func (s *Session) hasAncestorIn(id string, ids []string) bool {
	w := s.e.World()
	for range hierarchy.MaxDepth {
		o := s.objs[id]
		if o == nil || !w.IsAlive(o.entity) {
			return false
		}
		p := ecs.GetCompOrNil[hierarchy.Parent](w, o.entity)
		if p == nil {
			return false
		}
		if id = s.idOfEntity(p.Entity); id == "" {
			return false
		}
		if slices.Contains(ids, id) {
			return true
		}
	}
	return false
}

// parentLinkColor is the dotted line from a selected attached object to its
// parent.
var parentLinkColor = color.RGBA{R: 255, G: 190, B: 60, A: 200}

// drawParentLinks draws a dotted line from every selected object that is
// attached to another one to its parent, in window pixels.
func (s *Session) drawParentLinks(screen ebiten.FinalScreen) {
	cam := s.e.Camera()
	toWindow := func(p gfx.Vec2) gfx.Vec2 { return s.e.LogicalToWindow(cam.WorldToScreen(p, s.screenSize())) }
	for _, id := range s.selection {
		_, parent, ok := s.parentOf(id)
		t := s.transformOf(id)
		if !ok || t == nil {
			continue
		}
		a, b := toWindow(t.Pos), toWindow(parent.Pos)
		n := max(int(b.Sub(a).Len()/linkDotGap), 1)
		for i := 0; i <= n; i++ {
			p := a.Add(b.Sub(a).Mul(float32(i) / float32(n)))
			s.fillRect(screen, p.X-1, p.Y-1, 2, 2, parentLinkColor)
		}
	}
}

// linkDotGap is the distance, in window pixels, between the dots of a
// parent link.
const linkDotGap = 6
