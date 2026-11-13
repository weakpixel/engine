package editor

import (
	"cmp"
	"fmt"
	"slices"

	"github.com/weakpixel/asset"
	"github.com/weakpixel/ecs"
	"github.com/weakpixel/engine/gfx"
	"github.com/weakpixel/engine/tilemap"
)

// MapLayer is one tile or image layer the game spawned for a Tiled map
// behind the level (its backdrop), as a shell lists it.
type MapLayer struct {
	// Map is the map's asset key (the .tmj file) and Path the layer's Tiled
	// path ("group/layer"); together they name the layer. Paths need not be
	// unique.
	Map, Path string
	// Name is the layer's own name in Tiled.
	Name string
	// Kind is "tiles" or "image".
	Kind string
	// Layer is the render layer the layer was spawned on
	// (tilemap.TileLayer.Layer), which a game's LayerFor may have chosen.
	Layer int16
	// Hidden is whether the layer is not drawn: invisible in Tiled, or
	// hidden by [Session.SetMapLayerHidden].
	Hidden bool
}

// LayersChanged reports the map layers after they changed (a backdrop
// loaded, replaced or hot-reloaded, a layer hidden). Layers is in draw
// order and empty when the level has no backdrop.
type LayersChanged struct{ Layers []MapLayer }

func (LayersChanged) event() {}

// layerRef is a spawned map layer: its entity and the map it belongs to.
type layerRef struct {
	entity ecs.Entity
	tl     *tilemap.TileLayer
	pos    gfx.Vec2
	m      *tilemap.Map
	key    string
}

// mapLayerRefs returns every spawned tile and image layer of the world whose
// map is loaded, in draw order: render layer, then map key, then index in
// the map. The refs point into the world and are valid until it changes.
func (s *Session) mapLayerRefs() []layerRef {
	if !s.layerViewOK {
		s.layerView, s.layerViewOK = ecs.NewView2[tilemap.TileLayer, gfx.Transform](s.e.World()), true
	}
	maps := s.e.Stores().Maps()
	refs := s.layerRefs[:0]
	for en, c := range s.layerView.Each() {
		tl, tr := c.Unpack()
		m, state := maps.Lookup(tl.Map)
		if state != asset.Ready || tl.Index < 0 || int(tl.Index) >= len(m.Layers) {
			continue
		}
		refs = append(refs, layerRef{entity: en, tl: tl, pos: tr.Pos, m: m, key: maps.Key(tl.Map).Path})
	}
	slices.SortFunc(refs, func(a, b layerRef) int {
		return cmp.Or(cmp.Compare(a.tl.Layer, b.tl.Layer), cmp.Compare(a.key, b.key), cmp.Compare(a.tl.Index, b.tl.Index))
	})
	s.layerRefs = refs
	return refs
}

// MapLayers returns the tile and image layers of the Tiled maps spawned in
// the world (the level's backdrop), in draw order. Object and group layers
// spawn nothing and are not listed.
func (s *Session) MapLayers() []MapLayer {
	if s.e == nil {
		return nil
	}
	return layerInfos(s.mapLayerRefs(), nil)
}

// layerInfos converts refs to MapLayers, appended to dst.
func layerInfos(refs []layerRef, dst []MapLayer) []MapLayer {
	for _, r := range refs {
		l := &r.m.Layers[r.tl.Index]
		dst = append(dst, MapLayer{Map: r.key, Path: l.Path, Name: l.Name, Kind: l.Kind.String(), Layer: r.tl.Layer, Hidden: r.tl.Hidden})
	}
	return dst
}

// reportLayers emits [LayersChanged] when the map layers differ from the
// last ones reported. It runs once per edit tick: a handful of entities,
// no allocation while nothing changes.
func (s *Session) reportLayers() {
	refs := s.mapLayerRefs()
	s.syncHidden(refs)
	cur := layerInfos(refs, s.layerScratch[:0])
	s.layerScratch = cur
	if slices.Equal(cur, s.layersSent) {
		return
	}
	s.layersSent = append(s.layersSent[:0], cur...)
	s.emit(LayersChanged{Layers: slices.Clone(cur)})
}

// syncHidden returns a layer hidden by the editor to Tiled's visibility
// when its map was hot-reloaded since (the map owns the file).
func (s *Session) syncHidden(refs []layerRef) {
	if len(s.hiddenAt) == 0 {
		return
	}
	maps := s.e.Stores().Maps().Generic()
	for _, r := range refs {
		v, ok := s.hiddenAt[r.entity]
		if !ok {
			continue
		}
		if now := maps.Version(asset.HandleOf[tilemap.Map](r.tl.Map.Ref)); now != v {
			r.tl.Hidden = !r.m.Layers[r.tl.Index].Visible
			delete(s.hiddenAt, r.entity)
		}
	}
	for en := range s.hiddenAt {
		if !s.e.World().IsAlive(en) {
			delete(s.hiddenAt, en)
		}
	}
}

// SetMapLayerHidden hides or shows the map layer named by the map key and
// Tiled path of a [MapLayer], in edit mode only: nothing is written to the
// map, which Tiled owns. Play and Stop keep it; a map hot reload or a new
// backdrop starts from Tiled's visibility again. It returns an error for
// an unknown layer and in Play.
func (s *Session) SetMapLayerHidden(mapKey, path string, hidden bool) error {
	if s.e == nil {
		return fmt.Errorf("editor: SetMapLayerHidden before the session is attached to an engine")
	}
	if s.play {
		return fmt.Errorf("editor: map layers cannot be hidden while playing")
	}
	for _, r := range s.mapLayerRefs() {
		if r.key != mapKey || r.m.Layers[r.tl.Index].Path != path {
			continue
		}
		r.tl.Hidden = hidden
		if s.hiddenAt == nil {
			s.hiddenAt = map[ecs.Entity]uint32{}
		}
		s.hiddenAt[r.entity] = s.e.Stores().Maps().Generic().Version(asset.HandleOf[tilemap.Map](r.tl.Map.Ref))
		return nil
	}
	return fmt.Errorf("editor: no map layer %q in %q", path, mapKey)
}

// DefaultLayer returns the render layer a new object placed at pos starts
// on, so it lands between the tile layers of the Tiled map the way it
// would sit in Tiled: the render layer of the nearest drawable layer below
// the map's topmost object layer, or the map's lowest render layer when
// there is none below. With several maps (a world) it uses the map whose
// bounds contain pos, else the first. It reports false when there is no
// backdrop map or the map has no object layer, and the caller keeps its
// own default.
func (s *Session) DefaultLayer(pos gfx.Vec2) (int16, bool) {
	if s.e == nil {
		return 0, false
	}
	refs := s.mapLayerRefs()
	if len(refs) == 0 {
		return 0, false
	}
	// The map to use: the first (lowest key, then position) that contains
	// pos, else the first.
	type mapAt struct {
		key string
		pos gfx.Vec2
	}
	var maps []mapAt
	for _, r := range refs {
		if !slices.Contains(maps, mapAt{r.key, r.pos}) {
			maps = append(maps, mapAt{r.key, r.pos})
		}
	}
	slices.SortFunc(maps, func(a, b mapAt) int {
		return cmp.Or(cmp.Compare(a.key, b.key), cmp.Compare(a.pos.X, b.pos.X), cmp.Compare(a.pos.Y, b.pos.Y))
	})
	pick := maps[0]
	for _, ma := range maps {
		if r := mapRef(refs, ma.key, ma.pos); r.m.Bounds().Contains(pos.Sub(ma.pos)) {
			pick = ma
			break
		}
	}
	return s.layerBelowObjects(refs, pick.key, pick.pos)
}

// layerBelowObjects is DefaultLayer within one map instance of refs.
func (s *Session) layerBelowObjects(refs []layerRef, key string, pos gfx.Vec2) (int16, bool) {
	m := mapRef(refs, key, pos).m
	top := -1
	for i := range m.Layers {
		if m.Layers[i].Kind == tilemap.Objects {
			top = i
		}
	}
	if top < 0 {
		return 0, false
	}
	below, lowest, haveLowest := int16(0), int16(0), false
	nearest := -1
	for _, r := range refs {
		if r.key != key || r.pos != pos {
			continue
		}
		if !haveLowest || r.tl.Layer < lowest {
			lowest, haveLowest = r.tl.Layer, true
		}
		if i := int(r.tl.Index); i < top && i > nearest {
			nearest, below = i, r.tl.Layer
		}
	}
	if nearest >= 0 {
		return below, true
	}
	return lowest, haveLowest
}

// mapRef returns the first ref of the map key placed at pos.
func mapRef(refs []layerRef, key string, pos gfx.Vec2) layerRef {
	for _, r := range refs {
		if r.key == key && r.pos == pos {
			return r
		}
	}
	panic("editor: no layer of map " + key)
}
