package collision

import (
	"math"
	"slices"
	"strconv"

	"github.com/weakpixel/engine/gfx"
)

// DefaultCellSize is the cell size of a [Grid] when none is given, in world
// units. A good cell is about twice the size of a typical collider.
const DefaultCellSize = 64

// Grid is the default [Broadphase]: a uniform grid of square cells. Each
// proxy is entered in every cell its bounds touch. The entries are a sorted
// slice instead of a hash map, so a rebuild reuses its memory, does not
// allocate once warm, and reads the entries in order.
//
// Cell coordinates are clamped to ±2^19 cells (±33 million world units with
// 64-unit cells): far-away colliders share the edge cells, which costs
// speed, not correctness. A grid holds at most 2^24 proxies.
type Grid struct {
	cell float32
	// entries are packed (row, column, proxy) values, see entry. Sorting
	// plain uint64s is several times faster than sorting structs with a
	// comparison function, and it orders the entries by cell, row first.
	entries []uint64
	bounds  []gfx.Rect  // by Proxy
	masks   []layerMask // by Proxy
	sorted  bool

	// Dense mode: when the cells the entries occupy fill a small rectangle,
	// sort counting-sorts the entries by cell instead of comparing them, and
	// offsets[i] is the index in entries of the first entry of cell i of
	// that rectangle (row-major), so Query finds a row without a binary
	// search. See denseCells.
	occ     cellRect // the cells the inserted entries touch
	dense   bool
	offsets []int32
	tmp     []uint64 // counting sort scratch
}

// layerMask is the layer set and mask a proxy was inserted with.
type layerMask struct{ layer, mask Layers }

// cellRect is a rectangle of cells, both ends inclusive; empty when x0 > x1.
type cellRect struct{ x0, y0, x1, y1 int32 }

var noCells = cellRect{x0: math.MaxInt32, y0: math.MaxInt32, x1: math.MinInt32, y1: math.MinInt32}

func (r cellRect) width() int32 { return r.x1 - r.x0 + 1 }

// index returns the row-major index of cell (cx, cy), which lies in r.
func (r cellRect) index(cx, cy int32) int { return int(cy-r.y0)*int(r.width()) + int(cx-r.x0) }

// minDenseCells is the size of the cell rectangle below which a grid is
// always dense; above it the rectangle may hold at most 4 cells per entry.
const minDenseCells = 4096

// Entry layout: 20 bits row, 20 bits column (both biased to be unsigned),
// 24 bits proxy. The upper 40 bits are the cell key, so the cells of a row
// are consecutive keys and one binary search finds a row's entries.
const (
	cellBits  = 20
	proxyBits = 24
	cellBias  = 1 << (cellBits - 1)
	maxProxy  = 1<<proxyBits - 1
)

// MaxGridProxies is the number of proxies a [Grid] can hold.
const MaxGridProxies = maxProxy + 1

// NewGrid returns an empty grid with square cells of the given size in
// world units; zero or less uses DefaultCellSize.
func NewGrid(cellSize float32) *Grid {
	if cellSize <= 0 {
		cellSize = DefaultCellSize
	}
	return &Grid{cell: cellSize, sorted: true, occ: noCells}
}

// cellKey packs a cell's coordinates into the upper bits of an entry.
func cellKey(cx, cy int32) uint64 {
	return (uint64(cy+cellBias)<<cellBits | uint64(cx+cellBias)) << proxyBits
}

func entry(key uint64, p Proxy) uint64 { return key | uint64(p) }

func entryKey(e uint64) uint64 { return e &^ maxProxy }

func entryProxy(e uint64) Proxy { return Proxy(e & maxProxy) }

// cellOf returns the cell coordinate of x on one axis, clamped to the
// packable range.
func (g *Grid) cellOf(x float32) int32 {
	c := math.Floor(float64(x / g.cell))
	return int32(min(max(c, -cellBias), cellBias-1))
}

// cellRange returns the cells [lo, hi] of one axis that [a, b) touches; at
// least the cell holding a, so zero-size bounds are entered once.
func (g *Grid) cellRange(a, b float32) (int32, int32) {
	lo := g.cellOf(a)
	hi := int32(min(max(math.Ceil(float64(b/g.cell))-1, -cellBias), cellBias-1))
	return lo, max(hi, lo)
}

// Reset implements Broadphase.
func (g *Grid) Reset() {
	g.entries = g.entries[:0]
	g.bounds = g.bounds[:0]
	g.masks = g.masks[:0]
	g.sorted = true
	g.dense = false
	g.occ = noCells
}

// Insert implements Broadphase. Bounds with NaN coordinates are dropped.
//
// Panics with "collision: Grid proxy <id> out of range" for a negative id
// or one of MaxGridProxies or more.
func (g *Grid) Insert(id Proxy, b gfx.Rect, layer, mask Layers) {
	if id < 0 || id > maxProxy {
		panic("collision: Grid proxy " + strconv.Itoa(int(id)) + " out of range")
	}
	if b.Min.X != b.Min.X || b.Min.Y != b.Min.Y || b.Max.X != b.Max.X || b.Max.Y != b.Max.Y {
		return
	}
	if int(id) >= len(g.bounds) {
		g.bounds = slices.Grow(g.bounds, int(id)+1-len(g.bounds))[:id+1]
		g.masks = slices.Grow(g.masks, int(id)+1-len(g.masks))[:id+1]
	}
	g.bounds[id] = b
	g.masks[id] = layerMask{layer, mask}
	x0, x1 := g.cellRange(b.Min.X, b.Max.X)
	y0, y1 := g.cellRange(b.Min.Y, b.Max.Y)
	g.occ = cellRect{min(g.occ.x0, x0), min(g.occ.y0, y0), max(g.occ.x1, x1), max(g.occ.y1, y1)}
	for cy := y0; cy <= y1; cy++ {
		for cx := x0; cx <= x1; cx++ {
			g.entries = append(g.entries, entry(cellKey(cx, cy), id))
		}
	}
	g.sorted = false
}

// sort orders the entries by cell, once after a batch of inserts.
func (g *Grid) sort() {
	if g.sorted {
		return
	}
	g.sorted = true
	if g.denseCells() {
		g.countingSort()
		return
	}
	slices.Sort(g.entries)
}

// denseCells reports whether the occupied cell rectangle is small enough
// for a counting sort and an offsets table: at most max(4 per entry, 4096)
// cells. A scene with far-away colliders is not, and sorts the entries.
func (g *Grid) denseCells() bool {
	if len(g.entries) == 0 {
		return false
	}
	area := int64(g.occ.width()) * int64(g.occ.y1-g.occ.y0+1)
	return area <= max(4*int64(len(g.entries)), minDenseCells)
}

// countingSort orders the entries by cell like slices.Sort does (row first;
// the proxies of a cell keep their insert order, which is ascending when the
// ids were inserted in order) in O(entries + cells), and fills offsets.
func (g *Grid) countingSort() {
	r := g.occ
	n := r.index(r.x1, r.y1) + 1
	g.offsets = slices.Grow(g.offsets[:0], n+1)[:n+1]
	clear(g.offsets)
	for _, e := range g.entries {
		g.offsets[entryCell(e, r)+1]++
	}
	for i := 1; i <= n; i++ {
		g.offsets[i] += g.offsets[i-1]
	}
	// offsets[c] is now where cell c starts; placing an entry advances it,
	// and after the loop it holds where cell c+1 starts, so shift it back.
	g.tmp = slices.Grow(g.tmp[:0], len(g.entries))[:len(g.entries)]
	for _, e := range g.entries {
		c := entryCell(e, r)
		g.tmp[g.offsets[c]] = e
		g.offsets[c]++
	}
	copy(g.offsets[1:], g.offsets[:n])
	g.offsets[0] = 0
	g.entries, g.tmp = g.tmp, g.entries
	g.dense = true
}

// entryCell returns the index in r of the cell an entry is in.
func entryCell(e uint64, r cellRect) int {
	k := e >> proxyBits
	return r.index(int32(k&(1<<cellBits-1))-cellBias, int32(k>>cellBits)-cellBias)
}

// owns reports whether the cell with key k is the one that reports the
// overlap of a and b: the cell holding the top-left corner of their
// intersection. Every pair shares that cell and no other cell claims it, so
// a pair that shares several cells is still reported once.
func (g *Grid) owns(k uint64, a, b gfx.Rect) bool {
	return k == cellKey(g.cellOf(max(a.Min.X, b.Min.X)), g.cellOf(max(a.Min.Y, b.Min.Y)))
}

// Pairs implements Broadphase.
func (g *Grid) Pairs(out []Pair) []Pair {
	g.sort()
	es := g.entries
	for i := 0; i < len(es); {
		key := entryKey(es[i])
		j := i + 1
		for j < len(es) && entryKey(es[j]) == key {
			j++
		}
		run := es[i:j]
		for a := range run {
			pa := entryProxy(run[a])
			ma := g.masks[pa]
			ba := g.bounds[pa]
			for b := a + 1; b < len(run); b++ {
				pb := entryProxy(run[b])
				// The mask test first: it is cheaper than the bounds, and
				// it drops the pairs that nobody reacts to.
				if mb := g.masks[pb]; !ma.mask.Any(mb.layer) && !mb.mask.Any(ma.layer) {
					continue
				}
				bb := g.bounds[pb]
				if ba.Overlaps(bb) && g.owns(key, ba, bb) {
					out = append(out, Pair{pa, pb})
				}
			}
		}
		i = j
	}
	return out
}

// Query implements Broadphase.
func (g *Grid) Query(q gfx.Rect, out []Proxy) []Proxy {
	g.sort()
	x0, x1 := g.cellRange(q.Min.X, q.Max.X)
	y0, y1 := g.cellRange(q.Min.Y, q.Max.Y)
	if g.dense {
		x0, y0, x1, y1 = max(x0, g.occ.x0), max(y0, g.occ.y0), min(x1, g.occ.x1), min(y1, g.occ.y1)
		if x0 > x1 {
			return out
		}
	}
	for cy := y0; cy <= y1; cy++ {
		// The cells x0..x1 of a row are consecutive keys: the first entry
		// comes from the offsets table or one binary search, then the scan
		// runs to the row's last cell.
		end := cellKey(x1, cy) | maxProxy
		var i, stop int
		if g.dense {
			i, stop = int(g.offsets[g.occ.index(x0, cy)]), int(g.offsets[g.occ.index(x1, cy)+1])
		} else {
			i, _ = slices.BinarySearch(g.entries, cellKey(x0, cy))
			stop = len(g.entries)
		}
		for ; i < stop && g.entries[i] <= end; i++ {
			p := entryProxy(g.entries[i])
			b := g.bounds[p]
			if intersects(b, q) && g.owns(entryKey(g.entries[i]), b, q) {
				out = append(out, p)
			}
		}
	}
	return out
}
