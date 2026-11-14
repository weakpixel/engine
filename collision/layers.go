package collision

import (
	"strconv"
	"strings"
	"sync/atomic"
)

// Layers is a bitmask of collision layers 0..31. Name single layers once
// with [Layer] and combine them with [LayersOf] and the methods, so game
// code never spells out bit operations:
//
//	var (
//		Player = collision.Layer(1)
//		Wall   = collision.Layer(4)
//	)
//	hero := collision.Collider{Layer: Player, Mask: collision.LayersOf(Wall)}
type Layers uint32

// Common layer sets.
const (
	// NoLayers is the empty set.
	NoLayers Layers = 0
	// DefaultLayer is layer 0, where a collider with a zero Layer is.
	DefaultLayer Layers = 1
	// AllLayers has every layer.
	AllLayers Layers = ^Layers(0)
)

// Layer returns the set holding only layer n.
//
// Panics with "collision: layer <n> out of range 0..31" for other n.
func Layer(n int) Layers {
	if n < 0 || n > 31 {
		panic("collision: layer " + strconv.Itoa(n) + " out of range 0..31")
	}
	return 1 << n
}

// LayersOf returns the union of ls.
func LayersOf(ls ...Layers) Layers {
	var u Layers
	for _, l := range ls {
		u |= l
	}
	return u
}

// Has reports whether l holds every layer of o.
func (l Layers) Has(o Layers) bool { return l&o == o }

// Any reports whether l holds at least one layer of o.
func (l Layers) Any(o Layers) bool { return l&o != 0 }

// With returns l plus the layers of o.
func (l Layers) With(o Layers) Layers { return l | o }

// Without returns l minus the layers of o.
func (l Layers) Without(o Layers) Layers { return l &^ o }

// IsEmpty reports whether l holds no layer.
func (l Layers) IsEmpty() bool { return l == 0 }

// layerNames holds the names given with NameLayers. It is swapped as a whole
// so String may run concurrently with a (startup) NameLayers call.
var layerNames atomic.Pointer[[32]string]

// NameLayers gives single layers names that [Layers.String] and the debug
// overlay print instead of numbers. Later calls add to or replace earlier
// names. It changes no behavior; call it at startup.
//
// Panics with "collision: NameLayers: <l> is not a single layer" if a key
// holds no layer or more than one.
func NameLayers(names map[Layers]string) {
	var next [32]string
	if cur := layerNames.Load(); cur != nil {
		next = *cur
	}
	for l, name := range names {
		if l == 0 || l&(l-1) != 0 {
			panic("collision: NameLayers: " + strconv.FormatUint(uint64(l), 10) + " is not a single layer")
		}
		next[bitIndex(l)] = name
	}
	layerNames.Store(&next)
}

// bitIndex returns the index of the lowest set bit of a non-zero l.
func bitIndex(l Layers) int {
	i := 0
	for l&1 == 0 {
		l >>= 1
		i++
	}
	return i
}

// String lists the layers of l by name, or by number when unnamed, joined
// by "|": "1|3|4", "player|wall". The empty set is "none".
func (l Layers) String() string {
	if l == 0 {
		return "none"
	}
	names := layerNames.Load()
	var b strings.Builder
	for i := range 32 {
		if l&(1<<i) == 0 {
			continue
		}
		if b.Len() > 0 {
			b.WriteByte('|')
		}
		if names != nil && names[i] != "" {
			b.WriteString(names[i])
		} else {
			b.WriteString(strconv.Itoa(i))
		}
	}
	return b.String()
}

// LayerOrDefault returns c.Layer, or DefaultLayer when it is zero.
func (c *Collider) LayerOrDefault() Layers {
	if c.Layer == 0 {
		return DefaultLayer
	}
	return c.Layer
}

// Detects reports whether c reacts to other: c's Mask has a layer other is
// on. This is the one rule events, contacts and physics follow.
func (c *Collider) Detects(other *Collider) bool { return c.Mask.Any(other.LayerOrDefault()) }

// DetectedBy reports whether other reacts to c.
func (c *Collider) DetectedBy(other *Collider) bool { return other.Detects(c) }
