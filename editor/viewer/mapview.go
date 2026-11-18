package viewer

import (
	"encoding/json"
	"fmt"
	"image/color"
	"io/fs"
	"path"
	"strconv"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/weakpixel/asset"
	"github.com/weakpixel/ecs"
	"github.com/weakpixel/engine/editor/toolshell"
	"github.com/weakpixel/engine/gfx"
	"github.com/weakpixel/engine/tiled"
	"github.com/weakpixel/engine/tilemap"
)

// objRef points at an object: a layer of the map and an index in its
// Objects. layer is -1 for none.
type objRef struct{ layer, obj int }

var noObject = objRef{layer: -1}

func (r objRef) ok() bool { return r.layer >= 0 }

// mapView shows a Tiled map: its tile and image layers are the engine's own
// TileLayer entities, its objects are outlines over them, and the panel
// lists the layers with a checkbox each.
type mapView struct {
	tiledBase
	handle  tilemap.MapHandle
	version uint32
	ents    []ecs.Entity
	// own is Tiled's own visibility of each layer, parallel to Map.Layers
	// (the map's Visible is composed with the groups). user holds the
	// toggles the user made, by layer path, so they survive a reload.
	own   []bool
	user  map[string]bool
	names bool
	// hover and sel are the object under the mouse and the clicked one.
	hover, sel objRef
	levels     []string
}

// Open implements [toolshell.Tool].
func (v *mapView) Open(s *toolshell.Shell, key string) error {
	v.shell, v.key = s, key
	v.hover, v.sel = noObject, noObject
	v.user, v.names = map[string]bool{}, true
	h, err := tilemap.LoadMapSync(v.scope, key)
	if err != nil {
		return fmt.Errorf("view: %w", err)
	}
	v.handle = h
	v.levels = levelsOfMap(lookupOf(s), key)
	if err := v.spawn(); err != nil {
		return err
	}
	v.ready = true
	s.Frame(v.mapData().Bounds())
	return nil
}

// mapData is the loaded map.
func (v *mapView) mapData() *tilemap.Map { return v.shell.Engine().Stores().Maps().Get(v.handle) }

// mapVersion is the version of the map in its store; a hot reload raises it.
func (v *mapView) mapVersion() uint32 {
	return v.shell.Engine().Stores().Maps().Generic().Version(asset.HandleOf[tilemap.Map](v.handle.Ref))
}

// spawn creates the layer entities of the current map and reads Tiled's own
// visibility of the layers.
func (v *mapView) spawn() error {
	e := v.shell.Engine()
	ents, err := tilemap.Spawn(e.World(), e.Stores().Maps(), v.handle, tilemap.SpawnOptions{})
	if err != nil {
		return fmt.Errorf("view: %w", err)
	}
	v.ents, v.version = ents, v.mapVersion()
	v.own = ownVisibility(v.shell.FS(), v.key, len(v.mapData().Layers))
	v.applyVisibility()
	return nil
}

// ownVisibility reads the file again for each layer's own "visible" flag, in
// the order of Map.Layers (depth first, a group before its children). The
// loaded map only has the flag composed with the groups, which would leave
// the children of a hidden group unchecked after the group is checked. It
// returns nil when the file cannot be read, and the caller then falls back
// to the composed flag.
func ownVisibility(fsys fs.FS, key string, n int) []bool {
	if fsys == nil {
		return nil
	}
	data, err := fs.ReadFile(fsys, key)
	if err != nil {
		return nil
	}
	m, err := tiled.DecodeMap(data)
	if err != nil {
		return nil
	}
	var out []bool
	var walk func([]tiled.Layer)
	walk = func(ls []tiled.Layer) {
		for i := range ls {
			out = append(out, ls[i].Visible)
			walk(ls[i].Layers)
		}
	}
	walk(m.Layers)
	if len(out) != n {
		return nil
	}
	return out
}

// on is the state of the checkbox of layer i.
func (v *mapView) on(i int) bool {
	l := &v.mapData().Layers[i]
	if on, ok := v.user[l.Path]; ok {
		return on
	}
	if v.own != nil {
		return v.own[i]
	}
	return l.Visible
}

// shown reports whether layer i is drawn: its checkbox and those of all its
// groups are on.
func (v *mapView) shown(i int) bool {
	m := v.mapData()
	for ; i >= 0; i = m.Layers[i].Parent {
		if !v.on(i) {
			return false
		}
	}
	return true
}

// applyVisibility hides the tile and image layers that are not shown.
func (v *mapView) applyVisibility() {
	w := v.shell.Engine().World()
	for _, en := range v.ents {
		if tl := ecs.GetCompOrNil[tilemap.TileLayer](w, en); tl != nil {
			tl.Hidden = !v.shown(int(tl.Index))
		}
	}
}

// close removes the layer entities.
func (v *mapView) close(s *toolshell.Shell) {
	w := s.Engine().World()
	for _, en := range v.ents {
		if w.IsAlive(en) {
			w.DeleteEntity(en)
		}
	}
	v.ents, v.ready = nil, false
}

// anchor is the world position of an object's X, Y.
func anchor(l *tilemap.Layer, o *tiled.Object) gfx.Vec2 {
	return tilemap.ObjectPos(l, o, gfx.Vec2{})
}

// Update implements [toolshell.Tool]: a reloaded map, the object under the
// mouse and a click that selects it.
func (v *mapView) Update(s *toolshell.Shell) {
	if !v.ready {
		return
	}
	if v.mapVersion() != v.version {
		v.reload()
	}
	v.hover = v.objectAt(s.MouseWorld(), hitPx*float32(deviceScale())/pxPerUnit(s))
	if click, _ := v.ptr.update(s); click {
		if v.hover == v.sel {
			v.sel = noObject
		} else {
			v.sel = v.hover
		}
	}
}

// reload spawns the layers again after the map changed on disk. The camera
// and the toggles stay; the selection stays when the same object (by layer
// path and id) is still there.
func (v *mapView) reload() {
	var selPath string
	var selID int
	if v.sel.ok() {
		l, o := v.object(v.sel)
		selPath, selID = l.Path, o.ID
	}
	v.close(v.shell)
	if err := v.spawn(); err != nil {
		v.shell.ReportError(err)
		return
	}
	v.ready = true
	v.hover, v.sel = noObject, noObject
	if selPath != "" {
		m := v.mapData()
		for i := range m.Layers {
			if m.Layers[i].Path != selPath {
				continue
			}
			for j := range m.Layers[i].Objects {
				if m.Layers[i].Objects[j].ID == selID {
					v.sel = objRef{i, j}
				}
			}
		}
	}
	v.shell.SetContent(v.mapData().Bounds())
}

// objectAt returns the topmost object of a shown layer under world position
// p; tol is the reach in world units for lines and points.
func (v *mapView) objectAt(p gfx.Vec2, tol float32) objRef {
	m := v.mapData()
	for i := len(m.Layers) - 1; i >= 0; i-- {
		l := &m.Layers[i]
		if l.Kind != tilemap.Objects || !v.shown(i) {
			continue
		}
		for j := len(l.Objects) - 1; j >= 0; j-- {
			if hit(&l.Objects[j], anchor(l, &l.Objects[j]), p, tol) {
				return objRef{i, j}
			}
		}
	}
	return noObject
}

// object returns the referenced object.
func (v *mapView) object(r objRef) (*tilemap.Layer, *tiled.Object) {
	l := &v.mapData().Layers[r.layer]
	return l, &l.Objects[r.obj]
}

// layerColor is the outline colour of object layer i: the palette entry of
// its rank among the object layers.
func (v *mapView) layerColor(i int) color.RGBA {
	rank := 0
	for _, l := range v.mapData().Layers[:i] {
		if l.Kind == tilemap.Objects {
			rank++
		}
	}
	return layerPalette[rank%len(layerPalette)]
}

// Draw implements [toolshell.Tool]: the map's border, then the outlines and
// names of the objects of the shown layers.
func (v *mapView) Draw(s *toolshell.Shell, screen ebiten.FinalScreen) {
	if !v.ready {
		return
	}
	m := v.mapData()
	b := s.WindowRect(m.Bounds()).Grow(1)
	s.StrokePolyline(screen, []gfx.Vec2{b.Min, {X: b.Max.X, Y: b.Min.Y}, b.Max, {X: b.Min.X, Y: b.Max.Y}}, true, 1, outline)
	scale := pxPerUnit(s)
	for i := range m.Layers {
		l := &m.Layers[i]
		if l.Kind != tilemap.Objects || !v.shown(i) {
			continue
		}
		c := v.layerColor(i)
		for j := range l.Objects {
			o := &l.Objects[j]
			oc := c
			if !o.Visible {
				oc = dim(c)
			}
			drawObject(s, screen, o, anchor(l, o), oc, 1)
			if v.names {
				v.drawName(s, screen, o, anchor(l, o), scale, false)
			}
		}
	}
	for _, r := range []objRef{v.hover, v.sel} {
		if !r.ok() {
			continue
		}
		l, o := v.object(r)
		c := colHoverLine
		if r == v.sel {
			c = colSelLine
		}
		drawObject(s, screen, o, anchor(l, o), c, 2)
		v.drawName(s, screen, o, anchor(l, o), scale, true)
	}
}

// objectLabel is the name written on an object: its name, else its class,
// else the GID of a tile object.
func objectLabel(o *tiled.Object) string {
	switch {
	case o.Name != "":
		return o.Name
	case o.Class != "":
		return o.Class
	case o.Shape == tiled.ShapeTile:
		return "gid " + strconv.FormatUint(uint64(o.GID), 10)
	}
	return ""
}

// drawName writes the label of o at the top-left of its outline. Unless
// forced (hover and selection), only when o is big enough on screen to carry
// it: its width must exceed the label's and be at least minLabelPx. A point
// has no size and is labelled once the view is zoomed in to two pixels per
// unit.
func (v *mapView) drawName(s *toolshell.Shell, screen ebiten.FinalScreen, o *tiled.Object, at gfx.Vec2, scale float32, force bool) {
	text := objectLabel(o)
	if text == "" {
		return
	}
	lo, hi := at, at
	for _, pl := range outlines(o, at) {
		for _, p := range pl.pts {
			lo = gfx.Vec2{X: min(lo.X, p.X), Y: min(lo.Y, p.Y)}
			hi = gfx.Vec2{X: max(hi.X, p.X), Y: max(hi.Y, p.Y)}
		}
	}
	w := (hi.X - lo.X) * scale
	if !force {
		fits := v.cache.fits(text, w)
		if o.Shape == tiled.ShapePoint {
			fits = scale >= 2
		}
		if !fits {
			return
		}
	}
	p := s.WindowPoint(lo)
	b := screen.Bounds()
	if p.X > float32(b.Dx()) || p.Y > float32(b.Dy()) || p.X < -400 || p.Y < -40 {
		return
	}
	v.cache.draw(screen, text, p.X+1, p.Y+1)
}

// Panel implements [toolshell.Tool].
func (v *mapView) Panel() toolshell.Panel {
	p := toolshell.Panel{Title: path.Base(v.key)}
	if !v.ready {
		return p
	}
	m := v.mapData()
	size := "infinite"
	if !m.Infinite {
		size = fmt.Sprintf("%d × %d cells, %d × %d px", m.Width, m.Height, m.Width*m.TileW, m.Height*m.TileH)
	}
	var sets []string
	for _, ts := range m.Tilesets {
		sets = append(sets, ts.Name)
	}
	bg := "none"
	if m.Background != 0 {
		bg = tiled.Color(m.Background).String()
	}
	info := []toolshell.Field{
		toolshell.Label("map.size", "Size", size),
		toolshell.Label("map.cell", "Cell", fmt.Sprintf("%d × %d px", m.TileW, m.TileH)),
		toolshell.Label("map.orientation", "Orientation", "orthogonal"),
		toolshell.Label("map.class", "Class", orDash(m.Class)),
		toolshell.Label("map.layers", "Layers", fmt.Sprint(len(m.Layers))),
		toolshell.Label("map.tilesets", "Tilesets", orDash(strings.Join(sets, ", "))),
		toolshell.Label("map.background", "Background", bg+" (not drawn by the engine)"),
	}
	info = append(info, propFields("map.prop.", m.Props)...)
	layers := make([]toolshell.Field, len(m.Layers))
	for i := range m.Layers {
		l := &m.Layers[i]
		depth := 0
		for q := l.Parent; q >= 0; q = m.Layers[q].Parent {
			depth++
		}
		name := l.Name
		if name == "" {
			name = fmt.Sprintf("(%s layer %d)", l.Kind, l.ID) // a panel label is never empty
		}
		layers[i] = toolshell.Checkbox("layer."+strconv.Itoa(i), strings.Repeat("  ", depth)+name, v.on(i)).WithHint(l.Kind.String() + " layer")
	}
	left := toolshell.SideLeft
	levels := []toolshell.Field{toolshell.Label("levels.none", "Levels", "none (no level of the map's package uses it)")}
	if len(v.levels) > 0 {
		levels = levels[:0]
		for i, lv := range v.levels {
			levels = append(levels, toolshell.Button("level."+strconv.Itoa(i), "Open "+path.Base(lv)).WithHint(lv))
		}
	}
	p.Sections = []toolshell.Section{
		{ID: "map", Label: "Map", Fields: info},
		{ID: "layers", Label: "Layers", Side: left, Fields: layers},
		{ID: "objects", Label: "Objects", Side: left, Fields: []toolshell.Field{toolshell.Checkbox("objects.names", "Names", v.names)}},
		{ID: "object", Label: "Object", Fields: v.objectFields()},
		{ID: "levels", Label: "Levels", Side: left, Fields: levels},
		v.shell.ViewSection(),
	}
	return p
}

// objectFields describes the selected object, else the one under the mouse.
func (v *mapView) objectFields() []toolshell.Field {
	r, which := v.sel, "selected"
	if !r.ok() {
		r, which = v.hover, "hover"
	}
	if !r.ok() {
		return []toolshell.Field{toolshell.Label("obj.which", "Object", "click an object to select it")}
	}
	l, o := v.object(r)
	pos := anchor(l, o)
	fields := []toolshell.Field{
		toolshell.Label("obj.which", "Object", fmt.Sprintf("%d (%s)", o.ID, which)),
		toolshell.Label("obj.name", "Name", orDash(o.Name)),
		toolshell.Label("obj.class", "Class", orDash(o.Class)),
		toolshell.Label("obj.shape", "Shape", o.Shape.String()),
		toolshell.Label("obj.layer", "Layer", l.Path),
		toolshell.Label("obj.pos", "Position", pointText(pos)),
	}
	if o.Width != 0 || o.Height != 0 {
		fields = append(fields, toolshell.Label("obj.size", "Size", fmt.Sprintf("%g × %g", o.Width, o.Height)))
	}
	if o.Shape == tiled.ShapeTile {
		fields = append(fields, toolshell.Label("obj.gid", "GID", fmt.Sprint(o.GID)))
	}
	return append(fields, propFields("obj.prop.", o.Props)...)
}

// Set implements [toolshell.Tool].
func (v *mapView) Set(id string, val json.RawMessage) error {
	switch {
	case id == "objects.names":
		return json.Unmarshal(val, &v.names)
	case strings.HasPrefix(id, "layer."):
		i, err := strconv.Atoi(strings.TrimPrefix(id, "layer."))
		if m := v.mapData(); err != nil || i < 0 || i >= len(m.Layers) {
			return fmt.Errorf("view: unknown widget %q", id)
		}
		var on bool
		if err := json.Unmarshal(val, &on); err != nil {
			return fmt.Errorf("view: bad value %s", val)
		}
		v.user[v.mapData().Layers[i].Path] = on
		v.applyVisibility()
		return nil
	case strings.HasPrefix(id, "level."):
		i, err := strconv.Atoi(strings.TrimPrefix(id, "level."))
		if err != nil || i < 0 || i >= len(v.levels) {
			return fmt.Errorf("view: unknown widget %q", id)
		}
		return v.shell.OpenFile(v.levels[i])
	}
	return fmt.Errorf("view: unknown widget %q", id)
}
