package hierarchy

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/weakpixel/ecs"
	"github.com/weakpixel/engine/gfx"
)

// Attach makes child a child of parent. Neither may be stale, and child must
// not be an ancestor of parent or parent itself (panics with
// "hierarchy: attach would create a cycle"). It adds a gfx.Transform to
// child if it has none, and a [Parent] or, if child is already attached,
// changes its Entity. Adding the component moves the child to another
// archetype, so do not call Attach inside an Each over it.
//
// With keepWorld the Local is computed from the current world Transforms so
// that the child stays where it is, which reads parent's Transform as of the
// last System run. Without it the child keeps its existing Local (zero for a
// new attachment), so it jumps to the parent. The first placement after Attach
// sets PrevPos, so the move is not interpolated.
func Attach(w *ecs.World, child, parent ecs.Entity, keepWorld bool) {
	for a := parent; ; {
		if a == child {
			panic(fmt.Sprintf("hierarchy: attach would create a cycle: %v under %v", child, parent))
		}
		pp := ecs.GetCompOrNil[Parent](w, a)
		if pp == nil {
			break
		}
		a = pp.Entity
	}
	if !ecs.HasComp[gfx.Transform](w, child) {
		ecs.SetComp(w, child, gfx.Transform{})
	}
	var p Parent
	if cur := ecs.GetCompOrNil[Parent](w, child); cur != nil {
		p = *cur
	}
	p.Entity = parent
	p.placed = false
	if keepWorld {
		var pt gfx.Transform
		if t := ecs.GetCompOrNil[gfx.Transform](w, parent); t != nil {
			pt = *t
		}
		l := LocalOf(pt, *ecs.GetComp[gfx.Transform](w, child))
		p.Local.Pos, p.Local.Rot, p.Local.Scale = l.Pos, l.Rot, l.Scale
	}
	ecs.SetComp(w, child, p)
}

// WorldOf returns the world Transform (Pos, Rot, Scale) of e as the [System]
// would write it now: the Locals of e's chain composed onto its root's
// Transform, or onto the Transform of the nearest [PlaceAbsolute] link once
// that is placed (e's own Transform if e is that link). Use it where placement is read between a structural change and
// the next System run, such as a tool framing objects that were just spawned
// and whose Transform is not placed yet. An entity without a Parent gets its
// own Transform. ok is false when e or a link of its chain is dead, or when e
// has neither a Parent nor a Transform. It walks the chain without
// allocating, and panics like the System on a cycle or a chain deeper than
// [MaxDepth].
func WorldOf(w *ecs.World, e ecs.Entity) (gfx.Transform, bool) {
	var chain [MaxDepth]*Parent // on the stack: no allocation per call
	n := 0
	root := e
	for {
		if !w.IsAlive(root) {
			return gfx.Transform{}, false
		}
		p := ecs.GetCompOrNil[Parent](w, root)
		if p == nil {
			break
		}
		if n == MaxDepth {
			panic(fmt.Sprintf("hierarchy: cycle or chain deeper than %d at %v", MaxDepth, e))
		}
		chain[n] = p
		n++
		root = p.Entity
	}
	if n == 0 && !ecs.HasComp[gfx.Transform](w, e) {
		return gfx.Transform{}, false
	}
	world, _ := place(worldTransforms{w}, e, chain[:n], root)
	return world, true
}

// SetPlace sets the [PlaceMode] of child. Switching to PlaceAbsolute keeps
// child where it is: its Transform already holds the world placement. With
// PlaceRelative and keepWorld, the Local (Pos, Rot, Scale) is computed with
// [LocalOf] from the parent's [WorldOf] and child's Transform so that child
// stays where it is; without keepWorld it moves back to its Local in the next
// System run. Use it rather than writing Local.PlaceMode when the child
// should not jump. It panics with "hierarchy: SetPlace: <child> has no
// Parent". The component is changed in place: no archetype move.
func SetPlace(w *ecs.World, child ecs.Entity, mode PlaceMode, keepWorld bool) {
	p := ecs.GetCompOrNil[Parent](w, child)
	if p == nil {
		panic(fmt.Sprintf("hierarchy: SetPlace: %v has no Parent", child))
	}
	if mode == PlaceRelative && keepWorld && p.Local.PlaceMode == PlaceAbsolute {
		if t := ecs.GetCompOrNil[gfx.Transform](w, child); t != nil {
			parent, _ := WorldOf(w, p.Entity)
			l := LocalOf(parent, *t)
			p.Local.Pos, p.Local.Rot, p.Local.Scale = l.Pos, l.Rot, l.Scale
		}
	}
	p.Local.PlaceMode = mode
}

// Reposition places child from its Local again in the next [System] run, in
// either [PlaceMode]; for a [PlaceAbsolute] child it is the way to move it
// back next to its parent. Like the first placement it sets PrevPos, so the
// move is not interpolated. It panics with
// "hierarchy: Reposition: <child> has no Parent".
func Reposition(w *ecs.World, child ecs.Entity) {
	p := ecs.GetCompOrNil[Parent](w, child)
	if p == nil {
		panic(fmt.Sprintf("hierarchy: Reposition: %v has no Parent", child))
	}
	p.placed = false
}

// Detach removes child's [Parent]. The Transform already holds the world
// placement, so the child stays where it is. Detaching an entity that has no
// Parent does nothing. Do not call it inside an Each over child.
func Detach(w *ecs.World, child ecs.Entity) {
	if ecs.HasComp[Parent](w, child) {
		ecs.RemoveComp[Parent](w, child)
	}
}

// Children returns the entities directly attached to e. It scans every
// Parent in the World: use it in tools and one-off code, not per tick. The
// order is that of the archetype storage.
func Children(w *ecs.World, e ecs.Entity) []ecs.Entity {
	var out []ecs.Entity
	for c, p := range ecs.NewView1[Parent](w).Each() {
		if p.Entity == e {
			out = append(out, c)
		}
	}
	return out
}

// Delete deletes e and all its descendants now. Like ecs.World.DeleteEntity
// it panics inside an Each; during a tick, delete the parent instead and let
// the [System] delete the children. It scans every Parent in the World.
func Delete(w *ecs.World, e ecs.Entity) {
	var pairs [][2]ecs.Entity // child, parent
	for c, p := range ecs.NewView1[Parent](w).Each() {
		pairs = append(pairs, [2]ecs.Entity{c, p.Entity})
	}
	doomed := []ecs.Entity{e}
	for i := 0; i < len(doomed); i++ {
		for _, pr := range pairs {
			if pr[1] == doomed[i] && !slices.Contains(doomed, pr[0]) {
				doomed = append(doomed, pr[0])
			}
		}
	}
	for _, d := range doomed {
		if w.IsAlive(d) {
			w.DeleteEntity(d)
		}
	}
}

// CheckCycles reports every parent cycle, and every chain deeper than
// [MaxDepth], that starts at one of ents. The [System] panics on those, so
// the engine checks the objects of a document before it lets them run. Each
// problem is its own error in the joined result, naming its entities; name
// maps an entity to the text to print (an object id), nil prints the entity.
// Entities without a Parent or that are not alive are ignored.
func CheckCycles(w *ecs.World, ents []ecs.Entity, name func(ecs.Entity) string) error {
	if name == nil {
		name = func(e ecs.Entity) string { return fmt.Sprint(e) }
	}
	var errs []error
	reported := map[ecs.Entity]bool{}
	for _, start := range ents {
		var path []ecs.Entity
		for e := start; w.IsAlive(e) && !reported[e]; {
			if i := slices.Index(path, e); i >= 0 {
				cycle := path[i:]
				for _, c := range cycle {
					reported[c] = true
				}
				names := make([]string, len(cycle))
				for j, c := range cycle {
					names[j] = name(c)
				}
				errs = append(errs, fmt.Errorf("hierarchy: parent cycle %s", strings.Join(names, " -> ")))
				break
			}
			p := ecs.GetCompOrNil[Parent](w, e)
			if p == nil {
				break
			}
			if path = append(path, e); len(path) > MaxDepth {
				reported[e] = true
				errs = append(errs, fmt.Errorf("hierarchy: parent chain deeper than %d at %s", MaxDepth, name(start)))
				break
			}
			e = p.Entity
		}
	}
	return errors.Join(errs...)
}
