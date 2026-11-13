package tiled

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"
)

// TilesetSuffix is the file name suffix of an external tileset.
const TilesetSuffix = ".tsj"

// Tileset is a Tiled tileset: either one image cut into a grid of tiles
// (single-image) or one image per tile (an image collection).
type Tileset struct {
	Name  string
	Class string
	// TileWidth and TileHeight are the size of a grid cell of the image, and
	// the largest tile of a collection.
	TileWidth, TileHeight int
	// TileCount is the number of tiles. For a single-image tileset, local
	// tile IDs are 0..TileCount-1, row by row.
	TileCount int
	// Columns is the number of tiles per row of a single-image tileset; 0
	// for a collection.
	Columns int
	// Margin is the border around the image's tile grid, Spacing the gap
	// between tiles, in pixels.
	Margin, Spacing int
	// Image is the path of the tileset image relative to the tileset file;
	// empty for a collection, whose images are on its Tiles.
	Image                   string
	ImageWidth, ImageHeight int
	// TileOffset shifts every tile when drawn, in pixels.
	TileOffset Point
	// TileRenderSize is "tile" (draw tiles at their own size, the default)
	// or "grid" (scale them to the map's cell size). FillMode is "stretch".
	TileRenderSize string
	FillMode       string
	// ObjectAlignment is the anchor of tile objects; "unspecified" (the
	// default) means bottom-left in orthogonal maps.
	ObjectAlignment string
	Props           Properties
	// Tiles lists the tiles the file has data for, sorted by ID: every tile
	// of a collection, and only the tiles with properties, a class, an
	// animation or collision shapes of a single-image tileset.
	Tiles []Tile
}

// IsCollection reports whether the tileset is an image collection.
func (ts *Tileset) IsCollection() bool { return ts.Image == "" }

// Tile returns the data of local tile id, if the file has any.
func (ts *Tileset) Tile(id int) (*Tile, bool) {
	i, ok := slices.BinarySearchFunc(ts.Tiles, id, func(t Tile, id int) int { return t.ID - id })
	if !ok {
		return nil, false
	}
	return &ts.Tiles[i], true
}

// Tile is the per-tile data of a tileset.
type Tile struct {
	// ID is the local tile ID inside the tileset.
	ID int
	// Class is the tile's "class", or its "type" (see [Object.Class]).
	Class       string
	Probability float64
	// Image is the tile's own image, relative to the tileset file, in an
	// image collection; empty in a single-image tileset.
	Image                   string
	ImageWidth, ImageHeight int
	// Animation lists the frames of an animated tile; nil if it is static.
	Animation []Frame
	// Collision holds the tile's collision shapes, in tile pixels.
	Collision []Object
	Props     Properties
}

// Frame is one frame of a tile animation.
type Frame struct {
	// TileID is the local ID of the tile shown, in the same tileset.
	TileID   int
	Duration time.Duration
}

// rawTileset is a tileset as the file stores it, embedded in a map or in a
// *.tsj file of its own.
type rawTileset struct {
	FirstGID        uint32        `json:"firstgid"`
	Source          string        `json:"source"`
	Version         rawVersion    `json:"version"`
	Name            string        `json:"name"`
	Class           string        `json:"class"`
	TileWidth       int           `json:"tilewidth"`
	TileHeight      int           `json:"tileheight"`
	TileCount       int           `json:"tilecount"`
	Columns         int           `json:"columns"`
	Margin          int           `json:"margin"`
	Spacing         int           `json:"spacing"`
	Image           string        `json:"image"`
	ImageWidth      int           `json:"imagewidth"`
	ImageHeight     int           `json:"imageheight"`
	TransparentKey  string        `json:"transparentcolor"`
	TileOffset      *Point        `json:"tileoffset"`
	TileRenderSize  string        `json:"tilerendersize"`
	FillMode        string        `json:"fillmode"`
	ObjectAlignment string        `json:"objectalignment"`
	Properties      []rawProperty `json:"properties"`
	Tiles           []rawTile     `json:"tiles"`
}

type rawTile struct {
	ID          int     `json:"id"`
	Type        string  `json:"type"`
	Class       string  `json:"class"`
	Probability float64 `json:"probability"`
	Image       string  `json:"image"`
	ImageWidth  int     `json:"imagewidth"`
	ImageHeight int     `json:"imageheight"`
	// x, y, width and height select a sub-rectangle of a collection image.
	X         *int `json:"x"`
	Y         *int `json:"y"`
	Width     *int `json:"width"`
	Height    *int `json:"height"`
	Animation []struct {
		TileID   int `json:"tileid"`
		Duration int `json:"duration"` // ms
	} `json:"animation"`
	ObjectGroup *struct {
		Objects []rawObject `json:"objects"`
	} `json:"objectgroup"`
	Properties []rawProperty `json:"properties"`
}

// rawVersion is the format "version", a string since Tiled 1.6 and a number
// before.
type rawVersion string

func (v *rawVersion) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	if len(b) > 0 && b[0] == '"' {
		var s string
		if err := json.Unmarshal(b, &s); err != nil {
			return err
		}
		*v = rawVersion(s)
		return nil
	}
	*v = rawVersion(b) // a number such as 1.2, kept as written
	return nil
}

// check rejects format versions whose major part is not 1.
func (v rawVersion) check() error {
	if v == "" || v == "null" {
		return nil
	}
	major, _, _ := strings.Cut(string(v), ".")
	if n, err := strconv.Atoi(major); err != nil || n != 1 {
		return unsupported("format version", fmt.Sprintf("%q (only 1.x is supported)", string(v)))
	}
	return nil
}

// DecodeTileset decodes an external tileset (*.tsj). It checks what the file
// alone can tell: positive tile sizes, a consistent grid, tile IDs in range,
// animation frames that refer to existing tiles, and unsupported features.
func DecodeTileset(data []byte) (*Tileset, error) {
	var r rawTileset
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, fmt.Errorf("tileset: invalid JSON: %w", err)
	}
	return decodeTileset(&r)
}

// decodeTileset converts a raw tileset, from a *.tsj file or embedded in a
// map, and collects every problem.
func decodeTileset(r *rawTileset) (*Tileset, error) {
	ts := &Tileset{
		Name: r.Name, Class: r.Class,
		TileWidth: r.TileWidth, TileHeight: r.TileHeight, TileCount: r.TileCount,
		Columns: r.Columns, Margin: r.Margin, Spacing: r.Spacing,
		Image: r.Image, ImageWidth: r.ImageWidth, ImageHeight: r.ImageHeight,
		TileRenderSize: r.TileRenderSize, FillMode: r.FillMode, ObjectAlignment: r.ObjectAlignment,
	}
	if ts.TileRenderSize == "" {
		ts.TileRenderSize = "tile"
	}
	if ts.FillMode == "" {
		ts.FillMode = "stretch"
	}
	if ts.ObjectAlignment == "" {
		ts.ObjectAlignment = "unspecified"
	}
	if r.TileOffset != nil {
		ts.TileOffset = *r.TileOffset
	}

	var errs []error
	add := func(err error) {
		if err != nil {
			errs = append(errs, err)
		}
	}
	add(r.Version.check())
	if r.TransparentKey != "" {
		add(unsupported("transparentcolor", "use an image with an alpha channel"))
	}
	if ts.TileRenderSize != "tile" && ts.TileRenderSize != "grid" {
		add(fmt.Errorf("tilerendersize %q: want \"tile\" or \"grid\"", ts.TileRenderSize))
	}
	if ts.FillMode != "stretch" {
		add(unsupported("fillmode", fmt.Sprintf("%q (only \"stretch\" is supported)", ts.FillMode)))
	}
	if ts.TileWidth <= 0 || ts.TileHeight <= 0 {
		add(fmt.Errorf("tilewidth and tileheight must be positive, got %dx%d", ts.TileWidth, ts.TileHeight))
	}
	if ts.Margin < 0 || ts.Spacing < 0 {
		add(fmt.Errorf("margin and spacing must not be negative, got %d and %d", ts.Margin, ts.Spacing))
	}
	if !ts.IsCollection() {
		if ts.Columns <= 0 || ts.TileCount <= 0 {
			add(fmt.Errorf("columns and tilecount must be positive, got %d and %d", ts.Columns, ts.TileCount))
		}
	}
	props, err := decodeProperties(r.Properties)
	add(err)
	ts.Props = props

	ts.Tiles = make([]Tile, 0, len(r.Tiles))
	for i := range r.Tiles {
		// A tile with errors is kept, so later checks (animation frames)
		// still see it and report their own problems.
		t, err := decodeTile(ts, &r.Tiles[i])
		if err != nil {
			add(withContext(fmt.Sprintf("tile %d", r.Tiles[i].ID), err))
		}
		ts.Tiles = append(ts.Tiles, t)
	}
	slices.SortFunc(ts.Tiles, func(a, b Tile) int { return a.ID - b.ID })
	for i := 1; i < len(ts.Tiles); i++ {
		if ts.Tiles[i].ID == ts.Tiles[i-1].ID {
			add(fmt.Errorf("tile %d: listed twice", ts.Tiles[i].ID))
		}
	}
	// Animation frames are checked once every tile is known: a collection's
	// frames refer to tiles that may come later in the file.
	for _, t := range ts.Tiles {
		for fi, f := range t.Animation {
			if !ts.hasTile(f.TileID) {
				add(fmt.Errorf("tile %d: animation frame %d: no tile %d in the tileset", t.ID, fi, f.TileID))
			}
		}
	}
	return ts, errors.Join(errs...)
}

// hasTile reports whether local tile id exists.
func (ts *Tileset) hasTile(id int) bool {
	if ts.IsCollection() {
		_, ok := ts.Tile(id)
		return ok
	}
	return id >= 0 && id < ts.TileCount
}

func decodeTile(ts *Tileset, r *rawTile) (Tile, error) {
	t := Tile{
		ID: r.ID, Class: classOf(r.Class, r.Type), Probability: r.Probability,
		Image: r.Image, ImageWidth: r.ImageWidth, ImageHeight: r.ImageHeight,
	}
	var errs []error
	if r.ID < 0 || (!ts.IsCollection() && ts.TileCount > 0 && r.ID >= ts.TileCount) {
		errs = append(errs, fmt.Errorf("id out of range 0..%d", ts.TileCount-1))
	}
	if ts.IsCollection() {
		if r.Image == "" {
			errs = append(errs, errors.New("collection tile has no image"))
		}
		if subRect(r) {
			errs = append(errs, unsupported("image sub-rectangles", "collection tiles must use their whole image"))
		}
	} else if r.Image != "" {
		errs = append(errs, errors.New("tile has its own image in a single-image tileset"))
	}
	for i, f := range r.Animation {
		if f.Duration <= 0 {
			errs = append(errs, fmt.Errorf("animation frame %d: duration must be positive, got %d ms", i, f.Duration))
		}
		t.Animation = append(t.Animation, Frame{TileID: f.TileID, Duration: time.Duration(f.Duration) * time.Millisecond})
	}
	if r.ObjectGroup != nil {
		objs, err := decodeObjects(r.ObjectGroup.Objects)
		if err != nil {
			errs = append(errs, withContext("collision", err))
		}
		t.Collision = objs
	}
	props, err := decodeProperties(r.Properties)
	if err != nil {
		errs = append(errs, err)
	}
	t.Props = props
	return t, errors.Join(errs...)
}

// subRect reports whether a collection tile uses part of its image only.
// Tiled 1.9+ may write x, y, width and height even for the whole image.
func subRect(r *rawTile) bool {
	nonZero := func(p *int) bool { return p != nil && *p != 0 }
	differs := func(p *int, full int) bool { return p != nil && *p != full }
	return nonZero(r.X) || nonZero(r.Y) || differs(r.Width, r.ImageWidth) || differs(r.Height, r.ImageHeight)
}
