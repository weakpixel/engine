package viewer

import (
	"encoding/json"
	"fmt"
	"image/color"
	"path"
	"slices"
	"strconv"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/weakpixel/asset"
	"github.com/weakpixel/ecs"
	"github.com/weakpixel/engine/editor/toolshell"
	"github.com/weakpixel/engine/gfx"
	"github.com/weakpixel/engine/tilemap"
)

// colMapRect outlines the maps of a world.
var colMapRect = color.RGBA{R: 60, G: 140, B: 220, A: 255}

// worldMap is one map of the world as the previewer lists it.
type worldMap struct {
	key    string
	rect   gfx.Rect // in world pixels
	levels []string
	handle tilemap.MapHandle
}

// worldView shows a Tiled world: all its maps at their world positions, each
// outlined and labelled with its file and the levels that use it.
type worldView struct {
	tiledBase
	handle   tilemap.WorldHandle
	versions []uint32 // the world's, then each map's, to notice a reload
	ents     []ecs.Entity
	maps     []worldMap
	// hover and sel index maps, -1 for none.
	hover, sel int
}

// Open implements [toolshell.Tool].
func (v *worldView) Open(s *toolshell.Shell, key string) error {
	v.shell, v.key = s, key
	v.hover, v.sel = -1, -1
	h, err := tilemap.LoadWorldSync(v.scope, key)
	if err != nil {
		return fmt.Errorf("view: %w", err)
	}
	v.handle = h
	if err := v.spawn(); err != nil {
		return err
	}
	v.ready = true
	s.Frame(v.bounds())
	return nil
}

// spawn creates the layer entities of every map and lists the maps.
func (v *worldView) spawn() error {
	e := v.shell.Engine()
	ents, err := tilemap.SpawnWorld(e.World(), e.Stores().Worlds(), e.Stores().Maps(), v.handle, tilemap.SpawnOptions{})
	if err != nil {
		return fmt.Errorf("view: %w", err)
	}
	v.ents = ents
	look := lookupOf(v.shell)
	v.maps = v.maps[:0]
	for _, wm := range e.Stores().Worlds().Get(v.handle).Maps {
		key := e.Stores().Maps().Key(wm.Map).Path
		b := e.Stores().Maps().Get(wm.Map).Bounds()
		v.maps = append(v.maps, worldMap{
			key: key, handle: wm.Map, levels: levelsOfMap(look, key),
			rect: gfx.Rect{Min: b.Min.Add(wm.Pos), Max: b.Max.Add(wm.Pos)},
		})
	}
	v.versions = v.currentVersions()
	return nil
}

// currentVersions are the store versions of the world and of its maps.
func (v *worldView) currentVersions() []uint32 {
	st := v.shell.Engine().Stores()
	out := []uint32{st.Worlds().Generic().Version(asset.HandleOf[tilemap.World](v.handle.Ref))}
	for _, m := range st.Worlds().Get(v.handle).Maps {
		out = append(out, st.Maps().Generic().Version(asset.HandleOf[tilemap.Map](m.Map.Ref)))
	}
	return out
}

// bounds is the union of the map rectangles.
func (v *worldView) bounds() gfx.Rect {
	if len(v.maps) == 0 {
		return gfx.Rect{}
	}
	r := v.maps[0].rect
	for _, m := range v.maps[1:] {
		r.Min = gfx.Vec2{X: min(r.Min.X, m.rect.Min.X), Y: min(r.Min.Y, m.rect.Min.Y)}
		r.Max = gfx.Vec2{X: max(r.Max.X, m.rect.Max.X), Y: max(r.Max.Y, m.rect.Max.Y)}
	}
	return r
}

// close removes the layer entities.
func (v *worldView) close(s *toolshell.Shell) {
	w := s.Engine().World()
	for _, en := range v.ents {
		if w.IsAlive(en) {
			w.DeleteEntity(en)
		}
	}
	v.ents, v.ready = nil, false
}

// mapAt returns the index of the smallest map rectangle holding p, or -1.
func (v *worldView) mapAt(p gfx.Vec2) int {
	best, bestArea := -1, float32(0)
	for i, m := range v.maps {
		if !m.rect.Contains(p) {
			continue
		}
		area := (m.rect.Max.X - m.rect.Min.X) * (m.rect.Max.Y - m.rect.Min.Y)
		if best < 0 || area < bestArea {
			best, bestArea = i, area
		}
	}
	return best
}

// Update implements [toolshell.Tool]: a reloaded world or map, the map under
// the mouse, a click that selects it and a double click that opens its first
// level.
func (v *worldView) Update(s *toolshell.Shell) {
	if !v.ready {
		return
	}
	if !slices.Equal(v.currentVersions(), v.versions) {
		v.close(s)
		if err := v.spawn(); err != nil {
			s.ReportError(err)
			return
		}
		v.ready = true
		s.SetContent(v.bounds())
		v.hover = -1
		v.sel = min(v.sel, len(v.maps)-1)
	}
	v.hover = v.mapAt(s.MouseWorld())
	click, double := v.ptr.update(s)
	switch {
	case double:
		if v.hover >= 0 && len(v.maps[v.hover].levels) > 0 {
			s.OpenFile(v.maps[v.hover].levels[0])
		}
	case click:
		if v.hover == v.sel {
			v.sel = -1
		} else {
			v.sel = v.hover
		}
	}
}

// mapLabel is what is written on a map: its file and its levels.
func (m *worldMap) label() string {
	text := path.Base(m.key)
	if len(m.levels) > 0 {
		names := make([]string, len(m.levels))
		for i, l := range m.levels {
			names[i] = path.Base(l)
		}
		text += " → " + strings.Join(names, ", ")
	}
	return text
}

// Draw implements [toolshell.Tool]: each map's rectangle with its label.
func (v *worldView) Draw(s *toolshell.Shell, screen ebiten.FinalScreen) {
	if !v.ready {
		return
	}
	rect := func(r gfx.Rect, c color.RGBA, width float32) {
		w := s.WindowRect(r)
		s.StrokePolyline(screen, []gfx.Vec2{w.Min, {X: w.Max.X, Y: w.Min.Y}, w.Max, {X: w.Min.X, Y: w.Max.Y}}, true, width, c)
	}
	for i := range v.maps {
		m := &v.maps[i]
		rect(m.rect, colMapRect, 1)
		w := s.WindowRect(m.rect)
		if text := m.label(); v.cache.fits(text, w.Max.X-w.Min.X) {
			v.cache.draw(screen, text, w.Min.X+2, w.Min.Y+2)
		}
	}
	for _, i := range []int{v.hover, v.sel} {
		if i < 0 || i >= len(v.maps) {
			continue
		}
		c := colHoverLine
		if i == v.sel {
			c = colSelLine
		}
		rect(v.maps[i].rect, c, 2)
		w := s.WindowRect(v.maps[i].rect)
		v.cache.draw(screen, v.maps[i].label(), w.Min.X+2, w.Min.Y+2)
	}
}

// Panel implements [toolshell.Tool].
func (v *worldView) Panel() toolshell.Panel {
	p := toolshell.Panel{Title: path.Base(v.key)}
	if !v.ready {
		return p
	}
	b := v.bounds()
	world := []toolshell.Field{
		toolshell.Label("world.maps", "Maps", strconv.Itoa(len(v.maps))),
		toolshell.Label("world.bounds", "Bounds", rectText(b)),
	}
	p.Sections = []toolshell.Section{v.mapsSection(), {ID: "world", Label: "World", Fields: world}, v.mapSection(), v.shell.ViewSection()}
	return p
}

// maxMapRows is the rows the map list shows before it scrolls.
const maxMapRows = 8

// mapsSection is the left section with the list of maps, "(none)" first
// (index 0 clears the selection), each entry the base name of the map file.
func (v *worldView) mapsSection() toolshell.Section {
	options := make([]string, 0, len(v.maps)+1)
	options = append(options, "(none)")
	for _, m := range v.maps {
		options = append(options, path.Base(m.key))
	}
	return toolshell.Section{ID: "maps", Label: "Maps", Side: toolshell.SideLeft, Fields: []toolshell.Field{
		toolshell.Dropdown("world.map", "Map", v.sel+1, options...).AsList(maxMapRows),
	}}
}

// mapSection describes the selected map, else the one under the mouse.
func (v *worldView) mapSection() toolshell.Section {
	i, which := v.sel, "selected"
	if i < 0 {
		i, which = v.hover, "hover"
	}
	if i < 0 || i >= len(v.maps) {
		return toolshell.Section{ID: "map", Label: "Map", Fields: []toolshell.Field{
			toolshell.Label("map.which", "Map", "click a map to select it, double click to open its level"),
		}}
	}
	m := &v.maps[i]
	fields := []toolshell.Field{
		toolshell.Label("map.which", "Map", fmt.Sprintf("%d (%s)", i, which)),
		toolshell.Label("map.file", "File", m.key),
		toolshell.Label("map.pos", "Position", pointText(m.rect.Min)),
		toolshell.Label("map.size", "Size", fmt.Sprintf("%g × %g px", m.rect.Max.X-m.rect.Min.X, m.rect.Max.Y-m.rect.Min.Y)),
	}
	if len(m.levels) == 0 {
		fields = append(fields, toolshell.Label("levels.none", "Levels", "none"))
	}
	for j, l := range m.levels {
		fields = append(fields, toolshell.Button("level."+strconv.Itoa(j), "Open "+path.Base(l)).WithHint(l))
	}
	fields = append(fields, toolshell.Button("map.preview", "Preview map").WithHint(m.key))
	return toolshell.Section{ID: "map", Label: "Map", Fields: fields}
}

// shownMap is the map the panel describes, or nil.
func (v *worldView) shownMap() *worldMap {
	for _, i := range []int{v.sel, v.hover} {
		if i >= 0 && i < len(v.maps) {
			return &v.maps[i]
		}
	}
	return nil
}

// Set implements [toolshell.Tool].
func (v *worldView) Set(id string, val json.RawMessage) error {
	if id == "world.map" {
		// The same selection as a click on the map; 0 is "(none)". Unlike a
		// click, choosing the selected map again does not clear it, and it
		// does not open a level.
		var i int
		if err := json.Unmarshal(val, &i); err != nil || i < 0 || i > len(v.maps) {
			return fmt.Errorf("view: bad map %s", val)
		}
		v.sel = i - 1
		return nil
	}
	m := v.shownMap()
	switch {
	case id == "map.preview" && m != nil:
		return v.shell.OpenFile(m.key)
	case strings.HasPrefix(id, "level.") && m != nil:
		j, err := strconv.Atoi(strings.TrimPrefix(id, "level."))
		if err == nil && j >= 0 && j < len(m.levels) {
			return v.shell.OpenFile(m.levels[j])
		}
	}
	return fmt.Errorf("view: unknown widget %q", id)
}
