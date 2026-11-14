package room

import (
	"fmt"

	"github.com/weakpixel/ecs"
	"github.com/weakpixel/engine/gfx"
)

// Debug shape colors of exits and spawns.
var (
	exitColor  = gfx.RGBA(255, 190, 40, 255)
	spawnColor = gfx.RGBA(80, 220, 255, 255)
)

// spawnRadius is the radius of a spawn point's debug circle.
const spawnRadius = 3

// checkExits fires the first exit a traveler entered (OnEnter) or pressed
// the action in (OnAction) this tick.
func (r *Rooms) checkExits() {
	for en, tc := range r.travelers.Each() {
		t, _ := tc.Unpack()
		for _, ec := range r.exits.Each() {
			et, ex := ec.Unpack()
			area := gfx.Rect{Min: et.Pos, Max: et.Pos.Add(ex.Size)}
			if !area.Contains(t.Pos) {
				continue
			}
			fire := false
			switch ex.Trigger {
			case OnEnter:
				// PrevPos is the position before this tick's systems ran
				// (the engine snapshots it), so this is the entering edge.
				fire = !area.Contains(t.PrevPos)
			case OnAction:
				fire = r.pressed(en, ex.Action)
			}
			if fire {
				r.GoTo(ex.Target, ex.Spawn, With(ex.Transition))
				return
			}
		}
	}
}

// pressed reports whether the player of the traveler en (its PlayerSlot, or
// player 0) pressed the action named name this tick.
func (r *Rooms) pressed(en ecs.Entity, name string) bool {
	a, ok := r.actions[name]
	if !ok {
		if acts := r.e.Input().Actions(); acts != nil {
			a, ok = acts.Lookup(name)
		}
		if !ok {
			r.reportOnce("action "+name, fmt.Errorf("room: exit action %q is not declared", name))
			return false
		}
		r.actions[name] = a
	}
	player := 0
	if slot := ecs.GetCompOrNil[gfx.PlayerSlot](r.w, en); slot != nil {
		player = int(slot.Player)
	}
	p := r.e.Input().Player(player)
	return p != nil && p.State().JustPressed(a)
}

// markShapes gives exits and spawns without a gfx.DebugShape one, so the
// debug overlay shows them and the editor can pick them, and keeps an
// exit's shape the size of its area (an editor may change it). The shape is
// runtime-only (not serialized). New entities are collected first and
// changed after the views' loops, because adding a component moves the
// entity to another archetype.
func (r *Rooms) markShapes() {
	r.marks = r.marks[:0]
	for en, c := range r.exits.Each() {
		_, ex := c.Unpack()
		if ds := ecs.GetCompOrNil[gfx.DebugShape](r.w, en); ds != nil {
			ds.Size = ex.Size
		} else {
			r.marks = append(r.marks, en)
		}
	}
	exits := len(r.marks)
	for en := range r.spawns.Each() {
		if !ecs.HasComp[gfx.DebugShape](r.w, en) {
			r.marks = append(r.marks, en)
		}
	}
	for i, en := range r.marks {
		shape := gfx.DebugShape{Kind: gfx.ShapeCircle, Size: gfx.Vec2{X: spawnRadius}, Color: spawnColor}
		if i < exits {
			shape = gfx.DebugShape{Kind: gfx.ShapeRect, Size: ecs.GetComp[Exit](r.w, en).Size, Color: exitColor}
		}
		ecs.SetComp(r.w, en, shape)
	}
}
