package viewer

import (
	"encoding/json"
	"fmt"
	"image/color"
	"math"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/weakpixel/asset"
	"github.com/weakpixel/ecs"
	"github.com/weakpixel/engine/editor/toolshell"
	"github.com/weakpixel/engine/gfx"
	"github.com/weakpixel/engine/tiled"
	"github.com/weakpixel/engine/tilemap"
)

// tickDuration is how far the animation clock moves each tick; the shell
// runs the engine at its fixed rate.
const tickDuration = time.Second / 60

// Colors of the tileset overlay.
var (
	colCollision = color.RGBA{R: 255, G: 90, B: 90, A: 255}
	colAnimMark  = color.RGBA{R: 90, G: 255, B: 160, A: 255}
)

// animMarkPx is the side of the mark on an animated tile, in window pixels.
const animMarkPx = 4

// tileEntity is a sprite entity of a tileset tile.
type tileEntity struct {
	id     int
	entity ecs.Entity
}

// tilesetView shows every tile of a tileset on its grid, one sprite entity
// per tile, so single-image tilesets and image collections look alike.
type tilesetView struct {
	tiledBase
	handle  tilemap.TilesetHandle
	version uint32
	tiles   []tileEntity
	// cols and rows are the grid, cell the size of a cell in pixels.
	cols, rows int
	cell       gfx.Vec2
	clock      time.Duration
	ids        bool
	collision  bool
	animate    bool
	// hover and sel are tile ids, -1 for none.
	hover, sel int
}

// tileset is the loaded tileset.
func (v *tilesetView) tileset() *tilemap.Tileset {
	return v.shell.Engine().Stores().Tilesets().Get(v.handle)
}

// Open implements [toolshell.Tool].
func (v *tilesetView) Open(s *toolshell.Shell, key string) error {
	v.shell, v.key = s, key
	v.hover, v.sel = -1, -1
	v.ids, v.collision, v.animate = true, true, true
	h, err := tilemap.LoadTilesetSync(v.scope, key)
	if err != nil {
		return fmt.Errorf("view: %w", err)
	}
	v.handle = h
	v.spawn()
	v.ready = true
	s.Frame(v.bounds())
	return nil
}

// gridOf returns the columns and rows of the layout of a tileset: its own
// columns for a single image, else a square-ish grid.
func gridOf(file *tiled.Tileset, n int) (cols, rows int) {
	cols = file.Columns
	if cols <= 0 {
		cols = max(int(math.Ceil(math.Sqrt(float64(n)))), 1)
	}
	return cols, max((n+cols-1)/cols, 1)
}

// spawn puts one sprite entity per tile on the grid.
func (v *tilesetView) spawn() {
	ts := v.tileset()
	v.cell = ts.TileSize
	if v.cell.X <= 0 || v.cell.Y <= 0 {
		v.cell = gfx.Vec2{X: 16, Y: 16}
	}
	v.cols, v.rows = gridOf(ts.File, len(ts.Tiles))
	w := v.shell.Engine().World()
	v.tiles = v.tiles[:0]
	for id := range ts.Tiles {
		t := &ts.Tiles[id]
		if t.Sprite.IsZero() {
			continue
		}
		// The engine culls a sprite by its position: the center keeps a
		// half visible tile.
		pos := v.cellOrigin(id).Add(t.Size.Mul(0.5))
		en := ecs.NewEntity2(w, gfx.Transform{Pos: pos, PrevPos: pos}, gfx.Sprite{ID: t.Sprite})
		v.tiles = append(v.tiles, tileEntity{id: id, entity: en})
	}
	v.version = v.tilesetVersion()
	v.clock = 0
}

// tilesetVersion is the version of the tileset in its store.
func (v *tilesetView) tilesetVersion() uint32 {
	return v.shell.Engine().Stores().Tilesets().Generic().Version(asset.HandleOf[tilemap.Tileset](v.handle.Ref))
}

// cellOrigin is the top-left corner of the cell of tile id.
func (v *tilesetView) cellOrigin(id int) gfx.Vec2 {
	return gfx.Vec2{X: float32(id%v.cols) * v.cell.X, Y: float32(id/v.cols) * v.cell.Y}
}

// bounds is the rectangle of the whole grid.
func (v *tilesetView) bounds() gfx.Rect {
	return gfx.Rect{Max: gfx.Vec2{X: float32(v.cols) * v.cell.X, Y: float32(v.rows) * v.cell.Y}}
}

// tileAt returns the id of the tile whose cell holds world position p, or -1
// for no cell or a cell without a tile.
func (v *tilesetView) tileAt(p gfx.Vec2) int {
	if !v.bounds().Contains(p) {
		return -1
	}
	id := int(p.Y/v.cell.Y)*v.cols + int(p.X/v.cell.X)
	ts := v.tileset()
	if id >= len(ts.Tiles) || ts.Tiles[id].Sprite.IsZero() {
		return -1
	}
	return id
}

// close removes the tile entities.
func (v *tilesetView) close(s *toolshell.Shell) {
	w := s.Engine().World()
	for _, t := range v.tiles {
		if w.IsAlive(t.entity) {
			w.DeleteEntity(t.entity)
		}
	}
	v.tiles, v.ready = v.tiles[:0], false
}

// Sync implements [toolshell.Syncer]: animated tiles show the frame of the
// clock, or their first frame when Animate is off.
func (v *tilesetView) Sync(s *toolshell.Shell) {
	if !v.ready {
		return
	}
	if v.animate {
		v.clock += tickDuration
	} else {
		v.clock = 0
	}
	ts, w := v.tileset(), s.Engine().World()
	for _, t := range v.tiles {
		tile := &ts.Tiles[t.id]
		if tile.AnimTotal <= 0 {
			continue
		}
		if sp := ecs.GetCompOrNil[gfx.Sprite](w, t.entity); sp != nil {
			sp.ID = tile.FrameAt(v.clock)
		}
	}
}

// Update implements [toolshell.Tool]: a reloaded tileset, the tile under the
// mouse and a click that selects it.
func (v *tilesetView) Update(s *toolshell.Shell) {
	if !v.ready {
		return
	}
	if v.tilesetVersion() != v.version {
		v.close(s)
		v.spawn()
		v.ready = true
		s.SetContent(v.bounds())
		if v.sel >= len(v.tileset().Tiles) {
			v.sel = -1
		}
	}
	v.hover = v.tileAt(s.MouseWorld())
	if click, _ := v.ptr.update(s); click {
		if v.hover == v.sel {
			v.sel = -1
		} else {
			v.sel = v.hover
		}
	}
}

// Draw implements [toolshell.Tool]: tile ids, collision shapes and the mark
// of animated tiles.
func (v *tilesetView) Draw(s *toolshell.Shell, screen ebiten.FinalScreen) {
	if !v.ready {
		return
	}
	b := s.WindowRect(v.bounds()).Grow(1)
	s.StrokePolyline(screen, []gfx.Vec2{b.Min, {X: b.Max.X, Y: b.Min.Y}, b.Max, {X: b.Min.X, Y: b.Max.Y}}, true, 1, outline)
	sw, sh := float32(screen.Bounds().Dx()), float32(screen.Bounds().Dy())
	ts := v.tileset()
	for _, t := range v.tiles {
		tile := &ts.Tiles[t.id]
		r := s.WindowRect(gfx.Rect{Min: v.cellOrigin(t.id), Max: v.cellOrigin(t.id).Add(v.cell)})
		if r.Max.X < 0 || r.Max.Y < 0 || r.Min.X > sw || r.Min.Y > sh {
			continue
		}
		if v.collision {
			for i := range tile.Shapes {
				o := &tile.Shapes[i]
				drawObject(s, screen, o, v.cellOrigin(t.id).Add(gfx.Vec2{X: float32(o.X), Y: float32(o.Y)}), colCollision, 1)
			}
		}
		if tile.AnimTotal > 0 {
			s.FillRect(screen, r.Max.X-animMarkPx-1, r.Min.Y+1, animMarkPx, animMarkPx, colAnimMark)
		}
		if text := strconv.Itoa(t.id); v.ids && v.cache.fits(text, r.Max.X-r.Min.X) {
			v.cache.draw(screen, text, r.Min.X+1, r.Min.Y+1)
		}
	}
	for _, id := range []int{v.hover, v.sel} {
		if id < 0 || id >= len(ts.Tiles) {
			continue
		}
		c := colHoverLine
		if id == v.sel {
			c = colSelLine
		}
		r := s.WindowRect(gfx.Rect{Min: v.cellOrigin(id), Max: v.cellOrigin(id).Add(v.cell)})
		s.StrokePolyline(screen, []gfx.Vec2{r.Min, {X: r.Max.X, Y: r.Min.Y}, r.Max, {X: r.Min.X, Y: r.Max.Y}}, true, 2, c)
		v.cache.draw(screen, strconv.Itoa(id), r.Min.X+1, r.Min.Y+1)
	}
}

// Panel implements [toolshell.Tool].
func (v *tilesetView) Panel() toolshell.Panel {
	p := toolshell.Panel{Title: path.Base(v.key)}
	if !v.ready {
		return p
	}
	ts := v.tileset()
	kind, columns := "image collection", "-"
	if !ts.File.IsCollection() {
		kind, columns = "single image", strconv.Itoa(ts.File.Columns)
	}
	info := []toolshell.Field{
		toolshell.Label("tileset.name", "Name", ts.Name),
		toolshell.Label("tileset.class", "Class", orDash(ts.Class)),
		toolshell.Label("tileset.tile", "Tile size", fmt.Sprintf("%g × %g px", ts.TileSize.X, ts.TileSize.Y)),
		toolshell.Label("tileset.count", "Tiles", strconv.Itoa(ts.File.TileCount)),
		toolshell.Label("tileset.columns", "Columns", columns),
		toolshell.Label("tileset.kind", "Kind", kind),
		toolshell.Label("tileset.grid", "Grid scale", fmt.Sprint(ts.GridScale)),
	}
	info = append(info, propFields("tileset.prop.", ts.Props)...)
	show := []toolshell.Field{
		toolshell.Checkbox("show.ids", "Ids", v.ids),
		toolshell.Checkbox("show.collision", "Collision", v.collision),
		toolshell.Checkbox("show.animate", "Animate", v.animate),
	}
	p.Sections = []toolshell.Section{
		{ID: "tileset", Label: "Tileset", Fields: info},
		{ID: "show", Label: "Show", Side: toolshell.SideLeft, Fields: show},
		{ID: "tile", Label: "Tile", Fields: v.tileFields()},
		v.shell.ViewSection(),
	}
	return p
}

// tileFields describes the selected tile, else the one under the mouse.
func (v *tilesetView) tileFields() []toolshell.Field {
	id, which := v.sel, "selected"
	if id < 0 {
		id, which = v.hover, "hover"
	}
	ts := v.tileset()
	if id < 0 || id >= len(ts.Tiles) {
		return []toolshell.Field{toolshell.Label("tile.which", "Tile", "click a tile to select it")}
	}
	t := &ts.Tiles[id]
	fields := []toolshell.Field{
		toolshell.Label("tile.which", "Tile", fmt.Sprintf("%d (%s)", id, which)),
		toolshell.Label("tile.class", "Class", orDash(t.Class)),
		toolshell.Label("tile.size", "Image size", fmt.Sprintf("%g × %g px", t.Size.X, t.Size.Y)),
		toolshell.Label("tile.shapes", "Collision shapes", strconv.Itoa(len(t.Shapes))),
	}
	if t.AnimTotal > 0 {
		fields = append(fields, toolshell.Label("tile.anim", "Animation",
			fmt.Sprintf("%d frames, %s: %s", len(t.Anim), durationText(t.AnimTotal), frameIDs(ts.File, id))))
	}
	return append(fields, propFields("tile.prop.", t.Props)...)
}

// frameIDs lists the tile ids of the frames of tile id's animation.
func frameIDs(file *tiled.Tileset, id int) string {
	ft, ok := file.Tile(id)
	if !ok {
		return ""
	}
	ids := make([]string, len(ft.Animation))
	for i, f := range ft.Animation {
		ids[i] = strconv.Itoa(f.TileID)
	}
	return strings.Join(ids, ", ")
}

// Set implements [toolshell.Tool].
func (v *tilesetView) Set(id string, val json.RawMessage) error {
	var target *bool
	switch id {
	case "show.ids":
		target = &v.ids
	case "show.collision":
		target = &v.collision
	case "show.animate":
		target = &v.animate
	default:
		return fmt.Errorf("view: unknown widget %q", id)
	}
	return json.Unmarshal(val, target)
}
