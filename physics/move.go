package physics

import (
	"image"
	"math"
	"slices"

	"github.com/weakpixel/ecs"
	"github.com/weakpixel/engine/collision"
	"github.com/weakpixel/engine/gfx"
	"github.com/weakpixel/engine/xmath"
)

// MoveOptions tune [MoveAndSlide].
type MoveOptions struct {
	// Pixel moves in whole world units and carries the fraction over in
	// *Remainder, so positions stay on the pixel grid. Remainder must be set
	// with Pixel; the game keeps it (in its own component). A hit zeroes the
	// remainder of that axis.
	Pixel     bool
	Remainder *gfx.Vec2
	// DropThrough ignores one-way colliders and tile grids for this move.
	DropThrough bool
	// CornerCorrect is the largest sideways shift, in world units, that
	// lets an upward move clear a ceiling corner it only clips; zero is
	// off. Downward hits are never corrected.
	CornerCorrect float32
	// LedgeCorrect is the largest up or down shift, in world units, that
	// lets a sideways move clear a ledge or overhang edge it only clips;
	// zero is off. It applies to any X hit.
	LedgeCorrect float32
	// Push is the mover's pushing mass, in the unit of Body.Mass; zero
	// does not push. With Push set, an X move into dynamic bodies pushes
	// them: mover and bodies move by the part of the move past the first
	// body times Push / (Push + M), where M is the bodies' total mass.
	Push float32
}

// MoveResult is what a [MoveAndSlide] call did.
type MoveResult struct {
	// Moved is how far the entity moved.
	Moved gfx.Vec2
	// HitX and HitY report that the move stopped on that axis;
	// HitXEntity and HitYEntity are the colliders hit (zero for a tile),
	// HitXTiles and HitYTiles the tile grids and HitXCell and HitYCell the
	// cells of a tile hit.
	HitX, HitY             bool
	HitXEntity, HitYEntity ecs.Entity
	HitXTiles, HitYTiles   collision.TileGrid
	HitXCell, HitYCell     image.Point
	// OnGround is set when the move hit something going down, or when the
	// mover ends on or less than one unit above a solid without moving up
	// (also a move of zero, as a pixel move that rounds to nothing);
	// OnCeiling is set for
	// a hit going up; OnWall is -1 for a hit going left, +1 going right.
	OnGround, OnCeiling bool
	OnWall              int8
	// CornerShift is the X shift of a corner correction and LedgeShift the
	// Y shift of a ledge correction, signed, zero when none. Moved includes
	// them.
	CornerShift, LedgeShift float32
	// Pushed is the first body a move with MoveOptions.Push pushed, and
	// PushedMass the total mass of the bodies it pushed; both are zero when
	// it pushed nothing (also when the bodies were blocked: then HitX is set
	// and HitXEntity is the first body).
	Pushed     ecs.Entity
	PushedMass float32
}

// moveMargin grows the broad-phase query of a sweep: colliders are found
// where the last detection placed them, but tested where they are now, and
// a collider that moved since is still found if it moved less than this.
const moveMargin = 16

// groundProbe is how far below a mover MoveAndSlide looks for the ground
// it stands on when the move itself did not hit anything going down. One
// unit covers a pixel move that stopped short of a platform that then rose
// by less than a unit: the mover still counts as standing on it, so
// physics carries it instead of letting the platform rise into it.
const groundProbe = 1

// blocker is the nearest hit of an axis sweep.
type blocker struct {
	gap    float32
	hit    bool
	entity ecs.Entity
	tiles  collision.TileGrid
	cell   image.Point
}

// MoveAndSlide moves e's Transform.Pos by delta against the solids its
// Collider.Mask detects: colliders that are not sensors, tile grids, and
// one-way colliders and grids from their blocking side. It moves along X
// first, then Y, each axis stopping flush at the first blocker, so a
// diagonal move into a wall slides along it. If e has a [Touching], it is
// written from the result.
//
// It finds the colliders through the World's collision Space and tests
// them where they are now (their Transform and Collider), so a platform or
// box that moved after the last detection is not entered; a collider that
// moved more than 16 units since then can be missed. It works with only a
// collision.System running. A mover that
// ends on or less than one unit above a solid is OnGround even when the
// move does not go down. A mover with a Touching that stands on a
// kinematic body is moved with it by the physics System. Blockers are
// tested by their bounds; a circle mover moves as its bounds. Solids the
// mover already overlaps by more than one unit do not block it; smaller
// overlaps (something moved into the mover) block it where it is. It does not allocate.
//
// With MoveOptions.Push, an X move into a dynamic body pushes it, and a
// row of up to 8 touching dynamic bodies in front of it, by moving their
// Transform.Pos (their Velocity is left alone): see [MoveOptions.Push] for
// the speed. The row stops flush at whatever blocks one of its bodies, a
// ninth body in the row blocks it, and kinematic bodies, bodies with LockX,
// colliders without a Body and tiles block as without Push. Y moves never
// push, and bodies standing on a pushed body are not moved with it. In
// pixel mode the pushed move is rounded toward zero to whole units and
// the fraction carried over in the remainder. A push comes before ledge
// correction: a blocked push can still be ledge-corrected.
//
// Panics with "physics: MoveAndSlide needs a rect, anim-body or circle
// collider" for a custom shape kind, and as ecs.GetComp when e has no
// Transform or Collider.
func MoveAndSlide(w *ecs.World, e ecs.Entity, delta gfx.Vec2, opts MoveOptions) MoveResult {
	t := ecs.GetComp[gfx.Transform](w, e)
	c := ecs.GetComp[collision.Collider](w, e)
	if opts.Pixel {
		delta = delta.Add(*opts.Remainder)
		whole := gfx.Vec2{X: float32(math.Round(float64(delta.X))), Y: float32(math.Round(float64(delta.Y)))}
		*opts.Remainder = delta.Sub(whole)
		delta = whole
	}
	var res MoveResult
	box, ok := boundsOf(t, c)
	if !ok {
		t.Pos = t.Pos.Add(delta)
		res.Moved = delta
		return res
	}
	sp := collision.SpaceOf(w)
	if delta.X != 0 {
		b := sweep(w, sp, e, nil, c.Mask, box, 0, delta.X, opts.DropThrough)
		dx := b.gap * xmath.Sign(delta.X) // delta.X when nothing is hit
		if b.hit && opts.Push > 0 && pushable(w, b.entity) {
			if p, ok := push(w, sp, b, delta.X, opts); ok {
				dx = p.moved
				res.Pushed, res.PushedMass = b.entity, p.mass
				b.hit = false
			}
		}
		if b.hit && opts.LedgeCorrect >= 1 {
			hitBox := moveRect(box, gfx.Vec2{X: dx})
			if s, nb, ok := nudge(w, sp, e, c.Mask, hitBox, 0, delta.X-dx, delta.Y, opts.LedgeCorrect, opts.DropThrough); ok {
				box = moveRect(box, gfx.Vec2{Y: s})
				res.LedgeShift, res.Moved.Y = s, s
				b, dx = nb, dx+nb.gap*xmath.Sign(delta.X)
			}
		}
		if b.hit {
			res.HitX, res.HitXEntity, res.HitXTiles, res.HitXCell = true, b.entity, b.tiles, b.cell
			res.OnWall = int8(xmath.Sign(delta.X))
			if opts.Pixel {
				opts.Remainder.X = 0
			}
		}
		box = moveRect(box, gfx.Vec2{X: dx})
		res.Moved.X = dx
	}
	if delta.Y != 0 {
		b := sweep(w, sp, e, nil, c.Mask, box, 1, delta.Y, opts.DropThrough)
		dy := b.gap * xmath.Sign(delta.Y)
		if b.hit && delta.Y < 0 && opts.CornerCorrect >= 1 {
			hitBox := moveRect(box, gfx.Vec2{Y: dy})
			if s, nb, ok := nudge(w, sp, e, c.Mask, hitBox, 1, delta.Y-dy, delta.X, opts.CornerCorrect, opts.DropThrough); ok {
				box = moveRect(box, gfx.Vec2{X: s})
				res.CornerShift = s
				res.Moved.X += s
				b, dy = nb, dy+nb.gap*xmath.Sign(delta.Y)
			}
		}
		if b.hit {
			res.HitY, res.HitYEntity, res.HitYTiles, res.HitYCell = true, b.entity, b.tiles, b.cell
			res.OnGround, res.OnCeiling = delta.Y > 0, delta.Y < 0
			if opts.Pixel {
				opts.Remainder.Y = 0
			}
		}
		res.Moved.Y += dy
		box = moveRect(box, gfx.Vec2{Y: dy})
	}
	ground := res.HitYEntity
	if !res.HitY && delta.Y >= 0 {
		// Standing flush on the ground is no hit when the move does not go
		// down (a pixel move rounds to zero every other tick at walking
		// gravity), but the mover is still on the ground.
		if b := sweep(w, sp, e, nil, c.Mask, box, 1, groundProbe, opts.DropThrough); b.hit {
			res.OnGround, ground = true, b.entity
		}
	}
	t.Pos = t.Pos.Add(res.Moved)
	if ts := ecs.GetCompOrNil[Touching](w, e); ts != nil {
		*ts = Touching{OnGround: res.OnGround, OnCeiling: res.OnCeiling, OnWall: res.OnWall}
		if res.OnGround {
			ts.Ground, ts.GroundNormal = ground, gfx.Vec2{Y: -1}
		}
	}
	return res
}

// nudge is corner and ledge correction. box stopped on axis with d left to
// move; nudge looks for the smallest whole shift s, 1 up to limit units
// along the other axis, such that box can move by s (no solid beside it is
// in the way) and from there moves along axis by at least min(1, |d|). The
// sign of perp, the move along the other axis, picks the direction:
// negative tries only negative shifts, positive only positive ones, zero
// tries -k then +k for each k. It returns s and the sweep from the shifted
// box, or false when no shift works.
func nudge(w *ecs.World, sp *collision.Space, self ecs.Entity, mask collision.Layers, box gfx.Rect, axis int, d, perp, limit float32, drop bool) (float32, blocker, bool) {
	other := 1 - axis
	need := min(1, xmath.Abs(d))
	for k := 1; k <= int(limit); k++ {
		for _, dir := range [2]float32{-1, 1} {
			if dir*perp < 0 {
				continue
			}
			s := dir * float32(k)
			if b := sweep(w, sp, self, nil, mask, box, other, s, drop); b.gap < float32(k) {
				continue
			}
			if b := sweep(w, sp, self, nil, mask, moveRect(box, axisVec(other, s)), axis, d, drop); b.gap >= need {
				return s, b, true
			}
		}
	}
	return 0, blocker{}, false
}

// sweep finds the nearest solid that box hits when it moves by d along
// axis (0 X, 1 Y), ignoring self and the colliders in skip.
func sweep(w *ecs.World, sp *collision.Space, self ecs.Entity, skip []ecs.Entity, mask collision.Layers, box gfx.Rect, axis int, d float32, drop bool) blocker {
	reach := moveRect(box, axisVec(axis, d))
	q := gfx.Rect{
		Min: gfx.Vec2{X: min(box.Min.X, reach.Min.X), Y: min(box.Min.Y, reach.Min.Y)},
		Max: gfx.Vec2{X: max(box.Max.X, reach.Max.X), Y: max(box.Max.Y, reach.Max.Y)},
	}
	best := blocker{gap: xmath.Abs(d)}
	for sh := range sp.QueryShapes(q.Grow(moveMargin), mask) {
		if sh.Entity == self || sh.Collider.Sensor || slices.Contains(skip, sh.Entity) {
			continue
		}
		if sh.Collider.OneWay != collision.OneWayNone && (drop || !oneWayFaces(sh.Collider.OneWay, axis, d)) {
			continue
		}
		if g, ok := gapTo(box, currentBounds(w, sh), axis, d); ok && g < best.gap {
			best = blocker{gap: g, hit: true, entity: sh.Entity}
		}
	}
	for g, opts := range sp.TileGrids() {
		layer := opts.Layer
		if layer == 0 {
			layer = collision.DefaultLayer
		}
		if !mask.Any(layer) || (opts.OneWay != collision.OneWayNone && (drop || !oneWayFaces(opts.OneWay, axis, d))) {
			continue
		}
		x0, y0 := g.CellOf(q.Min)
		x1, y1 := g.CellOf(q.Max)
		for cy := y0; cy <= y1; cy++ {
			for cx := x0; cx <= x1; cx++ {
				if !g.Solid(cx, cy) {
					continue
				}
				if gap, ok := gapTo(box, g.CellRect(cx, cy), axis, d); ok && gap < best.gap {
					best = blocker{gap: gap, hit: true, tiles: g, cell: image.Point{X: cx, Y: cy}}
				}
			}
		}
	}
	return best
}

// currentBounds returns where a collider is now: statics as the last
// detection placed them, other colliders from their current Transform and
// Collider (custom kinds by their placed bounds).
func currentBounds(w *ecs.World, sh *collision.Shape) gfx.Rect {
	if sh.Collider.Static {
		return sh.Bounds
	}
	t := ecs.GetCompOrNil[gfx.Transform](w, sh.Entity)
	c := ecs.GetCompOrNil[collision.Collider](w, sh.Entity)
	if t == nil || c == nil || c.Shape >= collision.ShapeCustom {
		return sh.Bounds
	}
	if r, ok := boundsOf(t, c); ok {
		return r
	}
	return sh.Bounds
}

// Overlap tolerances of a sweep. A solid that moved into the mover after
// the last move (a body bounced up into it, a platform rose) overlaps it a
// little: up to sinkTol along the move it still blocks (gap 0), so the
// mover does not fall into it. Overlaps of up to sideTol on the other axis
// count as side by side, so a mover sunk a little into a floor still walks
// along it and onto the next box.
const (
	sinkTol = 1
	sideTol = 0.5
)

// gapTo returns how far box can move along axis in the direction of d
// before it touches b, and false if b is not in its way: beside it on the
// other axis, behind it, or overlapping it by more than sinkTol.
func gapTo(box, b gfx.Rect, axis int, d float32) (float32, bool) {
	lo, hi, blo, bhi := box.Min.X, box.Max.X, b.Min.X, b.Max.X
	olo, ohi, oblo, obhi := box.Min.Y, box.Max.Y, b.Min.Y, b.Max.Y
	if axis == 1 {
		lo, hi, blo, bhi, olo, ohi, oblo, obhi = olo, ohi, oblo, obhi, lo, hi, blo, bhi
	}
	if olo >= obhi-sideTol || oblo >= ohi-sideTol {
		return 0, false // side by side on the other axis
	}
	if d > 0 {
		if blo < hi-sinkTol {
			return 0, false
		}
		return max(blo-hi, 0), true
	}
	if bhi > lo+sinkTol {
		return 0, false
	}
	return max(lo-bhi, 0), true
}

// oneWayFaces reports whether a one-way solid blocks a move along axis in
// the direction of d: the move goes into its blocking face.
func oneWayFaces(ow collision.OneWay, axis int, d float32) bool {
	f := face(ow)
	return f.Dot(axisVec(axis, d)) < 0
}

// boundsOf returns the world box of collider c at t, as package collision
// places it, and false for an anim-body collider without a size yet.
func boundsOf(t *gfx.Transform, c *collision.Collider) (gfx.Rect, bool) {
	s := t.Scale
	if s == (gfx.Vec2{}) {
		s = gfx.Vec2{X: 1, Y: 1}
	}
	switch c.Shape {
	case collision.ShapeAnimBody:
		if c.Size == (gfx.Vec2{}) {
			return gfx.Rect{}, false
		}
		fallthrough
	case collision.ShapeRect:
		x0, x1 := span(t.Pos.X, c.Offset.X, c.Size.X, s.X)
		y0, y1 := span(t.Pos.Y, c.Offset.Y, c.Size.Y, s.Y)
		return gfx.Rect{Min: gfx.Vec2{X: x0, Y: y0}, Max: gfx.Vec2{X: x1, Y: y1}}, true
	case collision.ShapeCircle:
		ctr := gfx.Vec2{X: t.Pos.X + c.Offset.X*s.X, Y: t.Pos.Y + c.Offset.Y*s.Y}
		r := c.Size.X * xmath.Abs(s.X)
		return gfx.Rect{Min: gfx.Vec2{X: ctr.X - r, Y: ctr.Y - r}, Max: gfx.Vec2{X: ctr.X + r, Y: ctr.Y + r}}, true
	}
	panic("physics: MoveAndSlide needs a rect, anim-body or circle collider")
}

// span maps [p, p+size) by scale s to pos, keeping lo <= hi (as collision
// does).
func span(pos, p, size, s float32) (lo, hi float32) {
	lo, hi = pos+p*s, pos+(p+size)*s
	if lo > hi {
		lo, hi = hi, lo
	}
	return lo, hi
}

func moveRect(r gfx.Rect, d gfx.Vec2) gfx.Rect { return gfx.Rect{Min: r.Min.Add(d), Max: r.Max.Add(d)} }

func axisVec(axis int, d float32) gfx.Vec2 {
	if axis == 0 {
		return gfx.Vec2{X: d}
	}
	return gfx.Vec2{Y: d}
}
