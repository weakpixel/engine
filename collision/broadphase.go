package collision

import "github.com/weakpixel/engine/gfx"

// Proxy identifies a shape in a [Broadphase]: the index the [Space] gave it
// for this tick.
type Proxy int32

// Pair is a candidate pair of proxies from [Broadphase.Pairs].
type Pair struct{ A, B Proxy }

// Broadphase finds candidate pairs cheaply; the narrow phase tests them.
// The [Space] uses two: one for dynamic colliders, refilled every tick, and
// one for static colliders, refilled when one of them changes. [Grid] is the
// default; a sweep-and-prune or a tree can take its place through [Config].
//
// The methods append to a slice the caller owns instead of taking a
// callback, so a Space calls them through the interface without allocating.
type Broadphase interface {
	// Reset drops every proxy.
	Reset()
	// Insert adds a proxy with the given world bounds, layer set and mask.
	Insert(id Proxy, bounds gfx.Rect, layer, mask Layers)
	// Pairs appends every pair of proxies whose bounds overlap (share an
	// area) and in which at least one side detects the other (its mask holds
	// a layer of the other), each pair once, in any order, and returns the
	// extended slice. A pair that no side detects may be left out, which
	// keeps a crowd of bullets that ignore each other free.
	Pairs(out []Pair) []Pair
	// Query appends every proxy whose bounds intersect q, whatever its layers, each once, and
	// returns the extended slice. An axis on which q has zero size matches
	// when q lies in [Min, Max) of the proxy's bounds, so a point query
	// works as gfx.Rect.Contains.
	Query(q gfx.Rect, out []Proxy) []Proxy
}
