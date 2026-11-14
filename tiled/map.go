package tiled

import (
	"encoding/json"
	"errors"
	"fmt"
)

// MapSuffix is the file name suffix of a map.
const MapSuffix = ".tmj"

// Map is a Tiled map. Only orthogonal maps decode.
type Map struct {
	// Width and Height are the size in cells; for an infinite map they are
	// informational (the layers' chunks hold the cells).
	Width, Height int
	// TileWidth and TileHeight are the cell size in pixels.
	TileWidth, TileHeight int
	Infinite              bool
	// RenderOrder is the order tiles are drawn in: "right-down" (the
	// default), "right-up", "left-down" or "left-up".
	RenderOrder string
	// ParallaxOrigin is the point where parallax layers line up with the
	// others, in pixels.
	ParallaxOrigin Point
	// Background is the background color; zero means none.
	Background Color
	Class      string
	Props      Properties
	// Tilesets are sorted by FirstGID.
	Tilesets []MapTileset
	// Layers are the top-level layers, bottom to top.
	Layers []Layer
}

// MapTileset is a tileset reference of a map.
type MapTileset struct {
	// FirstGID is the GID of the tileset's local tile 0.
	FirstGID uint32
	// Source is the path of an external tileset (*.tsj) relative to the map
	// file; empty for an embedded tileset.
	Source string
	// Embedded is the tileset itself when it is stored in the map. Only
	// image collections can be embedded (see the package doc).
	Embedded *Tileset
}

// rawMap is a map as the file stores it.
type rawMap struct {
	Version         rawVersion    `json:"version"`
	Orientation     string        `json:"orientation"`
	RenderOrder     string        `json:"renderorder"`
	Width           int           `json:"width"`
	Height          int           `json:"height"`
	TileWidth       int           `json:"tilewidth"`
	TileHeight      int           `json:"tileheight"`
	Infinite        bool          `json:"infinite"`
	ParallaxOriginX float64       `json:"parallaxoriginx"`
	ParallaxOriginY float64       `json:"parallaxoriginy"`
	BackgroundColor string        `json:"backgroundcolor"`
	Class           string        `json:"class"`
	Properties      []rawProperty `json:"properties"`
	Tilesets        []rawTileset  `json:"tilesets"`
	Layers          []rawLayer    `json:"layers"`
}

// DecodeMap decodes a map (*.tmj). It decodes the tile data of every layer
// and checks what the file alone can tell; whether every GID belongs to a
// tileset needs the external tilesets and is checked by package tilemap.
func DecodeMap(data []byte) (*Map, error) {
	var r rawMap
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, fmt.Errorf("map: invalid JSON: %w", err)
	}
	m := &Map{
		Width: r.Width, Height: r.Height, TileWidth: r.TileWidth, TileHeight: r.TileHeight,
		Infinite: r.Infinite, RenderOrder: r.RenderOrder,
		ParallaxOrigin: Point{r.ParallaxOriginX, r.ParallaxOriginY}, Class: r.Class,
	}
	if m.RenderOrder == "" {
		m.RenderOrder = "right-down"
	}

	var errs []error
	add := func(err error) {
		if err != nil {
			errs = append(errs, err)
		}
	}
	add(r.Version.check())
	if r.Orientation != "orthogonal" {
		add(unsupported("orientation", fmt.Sprintf("%q (only \"orthogonal\" is supported)", r.Orientation)))
	}
	switch m.RenderOrder {
	case "right-down", "right-up", "left-down", "left-up":
	default:
		add(fmt.Errorf("renderorder %q: want right-down, right-up, left-down or left-up", m.RenderOrder))
	}
	if m.TileWidth <= 0 || m.TileHeight <= 0 {
		add(fmt.Errorf("tilewidth and tileheight must be positive, got %dx%d", m.TileWidth, m.TileHeight))
	}
	if !m.Infinite && (m.Width <= 0 || m.Height <= 0) {
		add(fmt.Errorf("width and height must be positive, got %dx%d", m.Width, m.Height))
	}
	bg, err := ParseColor(r.BackgroundColor)
	add(withContext("backgroundcolor", err))
	m.Background = bg
	props, err := decodeProperties(r.Properties)
	add(err)
	m.Props = props

	add(m.decodeTilesets(r.Tilesets))
	layers, err := decodeLayers(r.Layers, m, "")
	add(err)
	m.Layers = layers
	return m, errors.Join(errs...)
}

// decodeTilesets fills m.Tilesets: external references as they are, embedded
// collections decoded, embedded single-image tilesets rejected.
func (m *Map) decodeTilesets(raw []rawTileset) error {
	var errs []error
	var prev uint32
	for i := range raw {
		r := &raw[i]
		ctx := fmt.Sprintf("tileset %d (firstgid %d)", i, r.FirstGID)
		if r.FirstGID < 1 || r.FirstGID <= prev {
			errs = append(errs, fmt.Errorf("%s: firstgid must be positive and increasing", ctx))
		}
		prev = r.FirstGID
		mt := MapTileset{FirstGID: r.FirstGID, Source: r.Source}
		if r.Source == "" {
			if r.Image != "" {
				errs = append(errs, fmt.Errorf("%s %q: %w", ctx, r.Name,
					unsupported("embedded single-image tilesets", "export the tileset to a .tsj file")))
			} else {
				ts, err := decodeTileset(r)
				errs = append(errs, withContext(fmt.Sprintf("%s %q", ctx, r.Name), err))
				mt.Embedded = ts
			}
		}
		m.Tilesets = append(m.Tilesets, mt)
	}
	return errors.Join(errs...)
}
