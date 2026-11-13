package physics

import (
	"image"

	"github.com/weakpixel/ecs"
	"github.com/weakpixel/engine/collision"
	"github.com/weakpixel/engine/gfx"
	"github.com/weakpixel/engine/xmath"
)

// Position correction: the penetration left alone (so resting contacts
// stay detected and stacks do not jitter) and the share of the rest that is
// corrected per substep.
const (
	slop       = 0.01
	correction = 0.8
)

// Contact is one solid contact of a substep, as the resolver and the hooks
// see it. When both sides detect each other, the two mirrored
// collision.Contacts are merged into one, with the first in the Space's
// order as Self. Normal points from Self out of Other.
type Contact struct {
	collision.Contact
	// Bounce and Friction are the combined materials of both sides;
	// SurfaceVel is the velocity Self is dragged to along the contact,
	// relative to Other (Other's Material.SurfaceVel minus Self's).
	Bounce, Friction float32
	SurfaceVel       gfx.Vec2
	// ApproachSpeed is the relative speed along -Normal at the start of the
	// resolution, positive when the sides move into each other.
	ApproachSpeed float32
	// Impulse is the normal impulse the resolver applied this substep, >= 0
	// (mass x velocity); FrictionImpulse the tangential one. Before the
	// first pass they hold the impulses of the same contact in the last
	// substep, which the built-in resolver applies first (warm start), so
	// stacks settle in few passes.
	Impulse, FrictionImpulse float32

	a, b       int32    // body indexes, -1 for none
	invA, invB gfx.Vec2 // per-axis inverse masses; zero for a side that is not pushed
	aDet, bDet bool     // which sides detect the other
	// zeroA and zeroB mark a side whose velocity counts as zero: the
	// carrier of the other side, whose motion the rider already gets.
	zeroA, zeroB bool
}

// Resolver resolves the contacts of a substep by changing velocities. It is
// called Config.Iterations times per substep with the same contacts; the
// System then pushes overlapping bodies apart (position correction) on its
// own.
type Resolver interface {
	Resolve(s *Solve, contacts []Contact)
}

// Solve is what a [Resolver] works with. It is valid only during the call.
type Solve struct {
	// Dt is the substep length in seconds; Pass the index of the pass.
	Dt   float32
	Pass int

	sys          *System
	zeroA, zeroB gfx.Vec2
}

// Vel returns the velocities of the two sides of c. A side without a body,
// or the platform a rider stands on, gets a zero velocity that writes do
// not keep.
func (s *Solve) Vel(c *Contact) (self, other *gfx.Vec2) {
	s.zeroA, s.zeroB = gfx.Vec2{}, gfx.Vec2{}
	self, other = &s.zeroA, &s.zeroB
	if c.a >= 0 && !c.zeroA {
		self = s.sys.bodies[c.a].vel
	}
	if c.b >= 0 && !c.zeroB {
		other = s.sys.bodies[c.b].vel
	}
	return self, other
}

// InvMass returns the per-axis inverse masses of the two sides of c: zero
// for a side that must not be changed (no body, kinematic, a locked axis,
// or a side whose Mask does not have the other's layer).
func (s *Solve) InvMass(c *Contact) (self, other gfx.Vec2) { return c.invA, c.invB }

// RestThreshold returns the approach speed below which the built-in
// resolver does not bounce, so resting bodies do not jitter: twice what
// gravity adds in a substep.
func (s *Solve) RestThreshold() float32 {
	return max(1, 2*s.sys.gravity.Len()*s.Dt)
}

// impulseResolver is the built-in resolver: sequential impulses along the
// normal with restitution, then Coulomb friction along the tangent.
type impulseResolver struct{}

func (impulseResolver) Resolve(s *Solve, contacts []Contact) {
	if s.Pass == 0 {
		for i := range contacts {
			c := &contacts[i]
			va, vb := s.Vel(c)
			apply(va, vb, c.invA, c.invB, c.Normal, c.Impulse)
			apply(va, vb, c.invA, c.invB, gfx.Vec2{X: -c.Normal.Y, Y: c.Normal.X}, c.FrictionImpulse)
		}
	}
	rest := s.RestThreshold()
	for i := range contacts {
		c := &contacts[i]
		va, vb := s.Vel(c)
		ia, ib := c.invA, c.invB
		n := c.Normal
		k := effMass(n, ia, ib)
		if k == 0 {
			continue
		}
		var target float32
		if c.ApproachSpeed > rest {
			target = c.Bounce * c.ApproachSpeed
		}
		vn := va.Sub(*vb).Dot(n)
		acc := max(c.Impulse+(target-vn)/k, 0)
		apply(va, vb, ia, ib, n, acc-c.Impulse)
		c.Impulse = acc

		t := gfx.Vec2{X: -n.Y, Y: n.X}
		kt := effMass(t, ia, ib)
		if kt == 0 {
			continue
		}
		vt := va.Sub(*vb).Dot(t)
		maxF := c.Friction * c.Impulse
		acc = min(max(c.FrictionImpulse+(c.SurfaceVel.Dot(t)-vt)/kt, -maxF), maxF)
		apply(va, vb, ia, ib, t, acc-c.FrictionImpulse)
		c.FrictionImpulse = acc
	}
}

// effMass is the inverse effective mass of the pair along unit vector d.
func effMass(d, ia, ib gfx.Vec2) float32 {
	return d.X*d.X*(ia.X+ib.X) + d.Y*d.Y*(ia.Y+ib.Y)
}

// apply adds the impulse lambda along d to a and its opposite to b.
func apply(va, vb *gfx.Vec2, ia, ib, d gfx.Vec2, lambda float32) {
	va.X += lambda * d.X * ia.X
	va.Y += lambda * d.Y * ia.Y
	vb.X -= lambda * d.X * ib.X
	vb.Y -= lambda * d.Y * ib.Y
}

// buildContacts turns the Space's contacts into this substep's solid
// contacts: mirrored pairs merged, sensors, one-way passes and contacts
// nobody is pushed by dropped, materials mixed, PreSolve applied.
func (s *System) buildContacts() {
	for i := range s.bodies {
		b := &s.bodies[i]
		b.fl = flags{}
		b.corrMin, b.corrMax = gfx.Vec2{}, gfx.Vec2{}
		b.oneWay = false
	}
	s.contacts = s.contacts[:0]
	clear(s.pairs)
	all := s.space.AllContacts()
	for k := range all {
		c := &all[k]
		ai, aBody := s.index[c.Self]
		if !aBody {
			ai = -1
		}
		if c.Tiles != nil {
			if aBody {
				s.addTileContact(c, ai)
			}
			continue
		}
		bi, bBody := s.index[c.Other]
		if !bBody {
			bi = -1
		}
		if !aBody && !bBody {
			continue
		}
		key := [2]ecs.Entity{c.Self, c.Other}
		if key[1].ID < key[0].ID || (key[1].ID == key[0].ID && key[1].Generation < key[0].Generation) {
			key[0], key[1] = key[1], key[0]
		}
		if j, ok := s.pairs[key]; ok {
			s.contacts[j].bDet = true // the mirror of a contact already added
			continue
		}
		s.pairs[key] = int32(len(s.contacts))
		s.contacts = append(s.contacts, Contact{Contact: *c, a: ai, b: bi, aDet: true})
	}
	// Finish the entity pairs now that both sides are known; tile contacts
	// were finished when added.
	n := 0
	for i := range s.contacts {
		c := &s.contacts[i]
		if c.Tiles == nil && !s.finishPair(c) {
			continue
		}
		if s.pre != nil && !s.pre(c) {
			continue
		}
		s.contacts[n] = *c
		n++
	}
	s.contacts = s.contacts[:n]
	for i := range s.bodies {
		if b := &s.bodies[i]; b.b.DropThrough && !b.oneWay {
			b.b.DropThrough = false
		}
	}
}

// finishPair fills the masses, materials and velocities of an entity pair
// and reports whether it is solid.
func (s *System) finishPair(c *Contact) bool {
	colA, matA := s.colliderOf(c.a, c.Self)
	colB, matB := s.colliderOf(c.b, c.Other)
	if colA == nil || colB == nil || colA.Sensor || colB.Sensor {
		return false
	}
	if c.aDet && c.a >= 0 {
		c.invA = s.bodies[c.a].inv
	}
	if c.bDet && c.b >= 0 {
		c.invB = s.bodies[c.b].inv
	}
	if c.invA == (gfx.Vec2{}) && c.invB == (gfx.Vec2{}) {
		return false
	}
	if colA.OneWay != collision.OneWayNone || colB.OneWay != collision.OneWayNone {
		_, held := s.warm[c.key()]
		if colB.OneWay != collision.OneWayNone && !s.oneWayBlocks(c.a, c.b, colB.OneWay, c.Normal, c.Depth, held) {
			return false
		}
		if colA.OneWay != collision.OneWayNone && !s.oneWayBlocks(c.b, c.a, colA.OneWay, c.Normal.Mul(-1), c.Depth, held) {
			return false
		}
	}
	if c.a >= 0 && c.b >= 0 {
		c.zeroB = s.bodies[c.a].carrier == c.b
		c.zeroA = s.bodies[c.b].carrier == c.a
	}
	s.mix(c, &matA, &matB)
	return true
}

// colliderOf returns the collider and material of a contact side: from the
// body list, or looked up for a collider without a body.
func (s *System) colliderOf(i int32, e ecs.Entity) (*collision.Collider, Material) {
	if i >= 0 {
		return s.bodies[i].col, s.bodies[i].mat
	}
	return ecs.GetCompOrNil[collision.Collider](s.w, e), s.materialOf(e)
}

// mix combines the materials of a contact and computes its approach speed.
func (s *System) mix(c *Contact, a, b *Material) {
	c.Bounce = max(a.Bounce, b.Bounce)
	c.Friction = xmath.Sqrt(a.Friction * b.Friction)
	c.SurfaceVel = b.SurfaceVel.Sub(a.SurfaceVel)
	va, vb := s.solve.Vel(c)
	c.ApproachSpeed = -va.Sub(*vb).Dot(c.Normal)
}

// correctPositions pushes overlapping bodies apart by most of the depth
// beyond the slop, split by inverse mass. The depth was detected before the
// positions moved, so what the resolved velocities moved the sides apart
// along the normal is taken off.
func (s *System) correctPositions() {
	for i := range s.contacts {
		c := &s.contacts[i]
		n := c.Normal
		wa := n.X*n.X*c.invA.X + n.Y*n.Y*c.invA.Y
		wb := n.X*n.X*c.invB.X + n.Y*n.Y*c.invB.Y
		va, vb := s.solve.Vel(c)
		depth := c.Depth - va.Sub(*vb).Dot(n)*s.solve.Dt
		amt := max(depth-slop, 0) * correction
		if wa+wb == 0 || amt == 0 {
			continue
		}
		if wa > 0 {
			s.bodies[c.a].push(n.Mul(amt * wa / (wa + wb)))
		}
		if wb > 0 {
			s.bodies[c.b].push(n.Mul(-amt * wb / (wa + wb)))
		}
	}
	for i := range s.bodies {
		b := &s.bodies[i]
		d := b.corrMin.Add(b.corrMax)
		if b.inv.X == 0 {
			d.X = 0
		}
		if b.inv.Y == 0 {
			d.Y = 0
		}
		b.t.Pos = b.t.Pos.Add(d)
	}
}

// push records a position correction.
func (b *body) push(d gfx.Vec2) {
	b.corrMin.X, b.corrMax.X = min(b.corrMin.X, d.X), max(b.corrMax.X, d.X)
	b.corrMin.Y, b.corrMax.Y = min(b.corrMin.Y, d.Y), max(b.corrMax.Y, d.Y)
}

// writeFlags sets the ground, wall and ceiling flags of the detecting sides
// of the solid contacts.
func (s *System) writeFlags() {
	for i := range s.contacts {
		c := &s.contacts[i]
		if c.aDet && c.a >= 0 {
			s.bodies[c.a].fl.add(c.Normal, c.Other)
		}
		if c.bDet && c.b >= 0 {
			s.bodies[c.b].fl.add(c.Normal.Mul(-1), c.Self)
		}
	}
}

// add records a contact with normal n (pointing out of other).
func (f *flags) add(n gfx.Vec2, other ecs.Entity) {
	const axis = 0.7 // about 45 degrees
	switch {
	case n.Y < -axis:
		f.onGround, f.ground, f.groundNormal = true, other, n
	case n.Y > axis:
		f.onCeiling = true
	case n.X > axis:
		f.onWall = -1
	case n.X < -axis:
		f.onWall = 1
	}
}

// sendImpacts sends an Impact for each detecting side of a solid pair that
// was not solid in the last tick nor earlier in this one.
func (s *System) sendImpacts() {
	for i := range s.contacts {
		c := &s.contacts[i]
		if c.aDet {
			s.impact(impactKey{c.Self, c.Other, c.Tiles}, c.Normal, c.ApproachSpeed)
		}
		if c.bDet {
			s.impact(impactKey{c.Other, c.Self, nil}, c.Normal.Mul(-1), c.ApproachSpeed)
		}
	}
}

func (s *System) impact(k impactKey, n gfx.Vec2, speed float32) {
	if _, ok := s.impSeen[k]; ok {
		return
	}
	s.impSeen[k] = struct{}{}
	if _, ok := s.impPrev[k]; !ok {
		s.impacts.Send(Impact{Self: k.self, Other: k.other, Tiles: k.tiles, Normal: n, Speed: max(speed, 0)})
	}
}

// warmKey identifies a contact across substeps.
type warmKey struct {
	self, other ecs.Entity
	tiles       collision.TileGrid
	cell        image.Point
}

func (c *Contact) key() warmKey { return warmKey{c.Self, c.Other, c.Tiles, c.Cell} }

type warmImpulse struct{ n, t float32 }

// warmStart loads the impulses of the last substep into the contacts.
func (s *System) warmStart() {
	for i := range s.contacts {
		c := &s.contacts[i]
		if w, ok := s.warm[c.key()]; ok {
			c.Impulse, c.FrictionImpulse = w.n, w.t
		}
	}
}

// keepImpulses stores the impulses of this substep for the next one.
func (s *System) keepImpulses() {
	clear(s.warmNext)
	for i := range s.contacts {
		c := &s.contacts[i]
		s.warmNext[c.key()] = warmImpulse{c.Impulse, c.FrictionImpulse}
	}
	s.warm, s.warmNext = s.warmNext, s.warm
}
