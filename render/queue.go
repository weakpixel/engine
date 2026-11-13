package render

import (
	"cmp"
	"math"
	"slices"

	"github.com/weakpixel/engine/gfx"
)

// Queue is the frame's command list. Systems Push items during Update,
// Sort orders them, and the Renderer reads them in Draw.
//
// The Queue is reused every tick: Reset keeps the capacity, so the steady
// state allocates nothing. It is not safe for concurrent use.
type Queue struct {
	items []Item
	// order holds indices into items in draw order. Sorting indices instead
	// of the ~120-byte Items keeps the sort cheap and the items in place.
	order []int32
	// keys holds the batch key of every item (parallel to items): shader
	// slot in the high 32 bits, texture slot in the low ones. It is filled
	// once per Sort so that the comparison does no store lookups.
	keys []uint64
	// sorted is true once Sort has covered every pushed item.
	sorted bool
	// culled counts the items extract systems skipped this tick because they
	// were outside the camera bounds (see AddCulled).
	culled int
	// floor is the lowest layer Sort keeps while floored is set (see
	// SetMinLayer). The zero Queue keeps every layer.
	floor   int16
	floored bool
}

// Reset empties the queue and keeps its capacity. Text strings are cleared
// so the queue does not keep them alive.
func (q *Queue) Reset() {
	clear(q.items)
	q.items = q.items[:0]
	q.order = q.order[:0]
	q.sorted = false
	q.culled = 0
}

// Push appends a copy of it.
func (q *Queue) Push(it Item) {
	q.items = append(q.items, it)
	q.sorted = false
}

// Sprite appends a world-space sprite item with Scale (1,1) and returns a
// pointer to the slot for filling in more fields. The pointer is valid until
// the next Push, Sprite or Reset.
func (q *Queue) Sprite(layer int16, id gfx.SpriteHandle, pos, prev gfx.Vec2) *Item {
	q.Push(Item{Kind: KindSprite, Layer: layer, Sprite: id, Pos: pos, PrevPos: prev, Scale: gfx.Vec2{X: 1, Y: 1}})
	return &q.items[len(q.items)-1]
}

// AddCulled records that an extract system skipped n items this tick
// because they were outside the camera bounds. Reset clears the count; the
// Renderer reports it as [Stats.Culled]. Extract systems call it once per
// Run with their total, not once per item.
func (q *Queue) AddCulled(n int) { q.culled += n }

// Culled returns the number of items culled since the last Reset.
func (q *Queue) Culled() int { return q.culled }

// SetMinLayer makes Sort drop the items on layers below l and count them as
// culled (see [Queue.Culled]). The engine sets it every tick from the scene
// stack, so a scene that does not draw the scenes below it hides their
// layers. math.MinInt16 (or any value no item is below) removes the floor;
// the zero Queue has none. Reset keeps it.
func (q *Queue) SetMinLayer(l int16) {
	q.floor, q.floored = l, l > math.MinInt16
}

// dropBelowFloor removes, in place and keeping the order, the items below
// the floor set by SetMinLayer.
func (q *Queue) dropBelowFloor() {
	kept := q.items[:0]
	for i := range q.items {
		if q.items[i].Layer >= q.floor {
			kept = append(kept, q.items[i])
		}
	}
	q.culled += len(q.items) - len(kept)
	clear(q.items[len(kept):]) // drop the strings of the removed items
	q.items = kept
}

// Len returns the number of queued items.
func (q *Queue) Len() int { return len(q.items) }

// At returns the i-th item in draw order, 0 <= i < Len. Before Sort (or
// after a Push that followed it) this is insertion order. The pointer is
// valid until the next Push or Reset.
func (q *Queue) At(i int) *Item {
	if q.sorted {
		return &q.items[q.order[i]]
	}
	return &q.items[i]
}

// Sort puts the items in draw order: by layer, then by SortY inside layers
// that cfg marks YSort, then (when sprites is not the zero SpriteStore) by
// shader and then texture, so batches stay together: Ebiten only merges
// consecutive draws with the same source, shader and uniforms. Items that
// compare equal keep their insertion order. Items below the layer set by
// [Queue.SetMinLayer] are dropped first. Missing, loading and failed
// sprites share the placeholder's texture, and canvas items sort before the
// sprites of their layer (in insertion order). cfg may be nil and sprites may be
// the zero SpriteStore (no shader or texture ordering).
func (q *Queue) Sort(cfg *LayerConfig, sprites SpriteStore) {
	if q.floored {
		q.dropBelowFloor()
	}
	n := len(q.items)
	q.order = slices.Grow(q.order[:0], n)[:n]
	for i := range q.order {
		q.order[i] = int32(i)
	}
	items := q.items
	// Performance trick: the batch key is looked up once per item here, not
	// twice per comparison (O(n log n) store lookups). Do not move it into
	// the comparison function.
	keys := q.fillKeys(sprites)
	slices.SortStableFunc(q.order, func(a, b int32) int {
		ia, ib := &items[a], &items[b]
		if c := cmp.Compare(ia.Layer, ib.Layer); c != 0 {
			return c
		}
		if cfg.ySortLayer(ia.Layer) {
			if c := cmp.Compare(ia.SortY, ib.SortY); c != 0 {
				return c
			}
		}
		if keys != nil && keyed(ia.Kind) && keyed(ib.Kind) {
			return cmp.Compare(keys[a], keys[b])
		}
		return 0
	})
	q.sorted = true
}

// keyed reports whether items of kind k have a batch key. Canvases have one
// (0, see fillKeys) so that the comparison stays a strict weak order when
// canvases and sprites share a layer.
func keyed(k Kind) bool { return k == KindSprite || k == KindNineSlice || k == KindCanvas }

// fillKeys fills q.keys with each sprite item's batch key (shader slot,
// texture slot) and returns it, or returns nil when there is no store to look
// sprites up in. The shader slot is the handle's own index: equal handles
// share it, and the zero handle (no shader) is 0, so unshaded sprites sort
// first in their layer. Canvas items get key 0: they sort before the sprites
// of their layer (a map chunk under the animated tiles drawn on top of it)
// and keep their insertion order among themselves.
func (q *Queue) fillKeys(sprites SpriteStore) []uint64 {
	if sprites.s == nil {
		return nil
	}
	q.keys = slices.Grow(q.keys[:0], len(q.items))[:len(q.items)]
	for i := range q.items {
		if it := &q.items[i]; it.Kind == KindSprite || it.Kind == KindNineSlice {
			q.keys[i] = uint64(it.Shader.Index())<<32 | uint64(sprites.Get(it.Sprite).Texture.Index())
		} else if it.Kind == KindCanvas {
			q.keys[i] = 0
		}
	}
	return q.keys
}
