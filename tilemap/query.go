package tilemap

import (
	"iter"
	"math"

	"github.com/weakpixel/engine/gfx"
	"github.com/weakpixel/engine/tiled"
)

// This file is the read-only data API of a Map, for game code: layers,
// tiles, cells and objects. Nothing here allocates except the iterators'
// closures.

// LayerByName returns the first layer in draw order called name.
func (m *Map) LayerByName(name string) (*Layer, bool) {
	for i := range m.Layers {
		if m.Layers[i].Name == name {
			return &m.Layers[i], true
		}
	}
	return nil, false
}

// LayerByPath returns the first layer in draw order whose Path is path
// ("entity-group/entities"). Tiled allows equal names, so a group may hold a
// tile layer and an object layer of the same name; use [Map.LayerByPathKind]
// to tell them apart.
func (m *Map) LayerByPath(path string) (*Layer, bool) {
	for i := range m.Layers {
		if m.Layers[i].Path == path {
			return &m.Layers[i], true
		}
	}
	return nil, false
}

// LayerByPathKind is [Map.LayerByPath] restricted to layers of kind k.
func (m *Map) LayerByPathKind(path string, k LayerKind) (*Layer, bool) {
	for i := range m.Layers {
		if l := &m.Layers[i]; l.Path == path && l.Kind == k {
			return l, true
		}
	}
	return nil, false
}

// Tile returns the tile of a GID (flag bits are ignored). ok is false for 0
// and for GIDs that are not a tile of the map's tilesets.
func (m *Map) Tile(gid uint32) (*Tile, bool) {
	gid &= tiled.GIDMask
	if gid == 0 || gid >= uint32(len(m.tiles)) || m.tiles[gid].Sprite.IsZero() {
		return nil, false
	}
	return &m.tiles[gid], true
}

// Cell returns the tile of cell (cx, cy) of tile layer l, in map cells
// (negative cells exist in infinite maps). gid is 0 for an empty cell. ok is
// false if l is not a tile layer or the cell is outside its grid.
func (m *Map) Cell(l *Layer, cx, cy int) (gid uint32, flip tiled.Flip, ok bool) {
	if l.Kind != Tiles {
		return 0, 0, false
	}
	x, y := cx-l.Origin.X, cy-l.Origin.Y
	if x < 0 || y < 0 || x >= l.W || y >= l.H {
		return 0, 0, false
	}
	gid, flip = tiled.SplitGID(l.GIDs[y*l.W+x])
	return gid, flip, true
}

// CellAt is [Map.Cell] for a position in map pixels, the space objects are
// in (subtract the map's world position first). The layer's Offset is taken
// into account; parallax is not.
func (m *Map) CellAt(l *Layer, pos gfx.Vec2) (gid uint32, flip tiled.Flip, ok bool) {
	p := pos.Sub(l.Offset)
	cx := int(math.Floor(float64(p.X) / float64(m.TileW)))
	cy := int(math.Floor(float64(p.Y) / float64(m.TileH)))
	return m.Cell(l, cx, cy)
}

// Objects yields every object of every object layer, in draw order of the
// layers and file order inside a layer.
func (m *Map) Objects() iter.Seq2[*Layer, *tiled.Object] {
	return func(yield func(*Layer, *tiled.Object) bool) {
		for i := range m.Layers {
			l := &m.Layers[i]
			for j := range l.Objects {
				if !yield(l, &l.Objects[j]) {
					return
				}
			}
		}
	}
}

// FindObject returns the first object called name, with its layer.
func (m *Map) FindObject(name string) (*Layer, *tiled.Object, bool) {
	for l, o := range m.Objects() {
		if o.Name == name {
			return l, o, true
		}
	}
	return nil, nil, false
}

// ObjectsOfClass yields the objects whose Class is class, with their layers.
func (m *Map) ObjectsOfClass(class string) iter.Seq2[*Layer, *tiled.Object] {
	return func(yield func(*Layer, *tiled.Object) bool) {
		for l, o := range m.Objects() {
			if o.Class == class && !yield(l, o) {
				return
			}
		}
	}
}

// ObjectPos returns the world position of object o of layer l for a map
// spawned at mapPos: mapPos + the layer's Offset + the object's X, Y. It is
// the top-left corner of rectangles, ellipses and text, the point of points,
// polygons and polylines, and the bottom-left corner of tile objects (Tiled's
// convention). Parallax is ignored: objects are gameplay data.
func ObjectPos(l *Layer, o *tiled.Object, mapPos gfx.Vec2) gfx.Vec2 {
	return mapPos.Add(l.Offset).Add(gfx.Vec2{X: float32(o.X), Y: float32(o.Y)})
}

// Bounds returns the map's rectangle in map pixels (add the map's world
// position, such as a WorldMap's Pos, to place it in the world): from (0,0)
// to (Width*TileW, Height*TileH). For an infinite map it is the union of its
// tile layers' cells, which may start at negative coordinates (or the
// informational Width x Height when it has no cells). Layer offsets and
// tiles taller than their cell are not included. Use it, for example, for
// camera.Rig.Bounds.
func (m *Map) Bounds() gfx.Rect {
	tw, th := float32(m.TileW), float32(m.TileH)
	full := gfx.Rect{Max: gfx.Vec2{X: float32(m.Width) * tw, Y: float32(m.Height) * th}}
	if !m.Infinite {
		return full
	}
	var r gfx.Rect
	found := false
	for i := range m.Layers {
		l := &m.Layers[i]
		if l.Kind != Tiles || l.W == 0 || l.H == 0 {
			continue
		}
		lr := gfx.Rect{
			Min: gfx.Vec2{X: float32(l.Origin.X) * tw, Y: float32(l.Origin.Y) * th},
			Max: gfx.Vec2{X: float32(l.Origin.X+l.W) * tw, Y: float32(l.Origin.Y+l.H) * th},
		}
		if !found {
			r, found = lr, true
			continue
		}
		r = gfx.Rect{
			Min: gfx.Vec2{X: min(r.Min.X, lr.Min.X), Y: min(r.Min.Y, lr.Min.Y)},
			Max: gfx.Vec2{X: max(r.Max.X, lr.Max.X), Y: max(r.Max.Y, lr.Max.Y)},
		}
	}
	if !found {
		return full
	}
	return r
}
