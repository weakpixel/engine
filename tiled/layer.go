package tiled

import (
	"encoding/json"
	"errors"
	"fmt"
)

// LayerKind is the type of a [Layer].
type LayerKind uint8

// Layer kinds.
const (
	TileLayer LayerKind = iota
	ObjectLayer
	ImageLayer
	GroupLayer
)

var layerKindNames = [...]string{"tilelayer", "objectgroup", "imagelayer", "group"}

// String returns the kind as Tiled writes it ("tilelayer", ...).
func (k LayerKind) String() string {
	if int(k) < len(layerKindNames) {
		return layerKindNames[k]
	}
	return "invalid"
}

// Layer is one layer of a map, as the file nests it. Offset, Parallax,
// Opacity, Tint and Visible are the layer's own values; a group's values
// apply to its children on top of theirs (package tilemap composes them).
type Layer struct {
	Kind  LayerKind
	ID    int
	Name  string
	Class string
	// Offset shifts the layer, in pixels.
	Offset Point
	// Parallax is the parallax factor; (1, 1), the default, scrolls with the
	// camera.
	Parallax Point
	Opacity  float64
	Visible  bool
	// Tint multiplies the layer's colors; zero means none.
	Tint  Color
	Props Properties

	// Width and Height are the layer size in cells. For a finite map they
	// equal the map size and Data holds Width*Height raw GIDs, row by row.
	// For an infinite map, StartX, StartY, Width and Height bound the
	// layer's Chunks and Data is nil.
	StartX, StartY int
	Width, Height  int
	Data           []uint32
	Chunks         []Chunk

	// DrawOrder is "topdown" (sort by Y, the default) or "index" for object
	// layers; Objects are in file order.
	DrawOrder string
	Objects   []Object

	// Image is the image of an image layer, relative to the map file.
	Image                   string
	ImageWidth, ImageHeight int
	RepeatX, RepeatY        bool

	// Layers are the children of a group, bottom to top.
	Layers []Layer
}

// Chunk is a rectangle of tile data of an infinite map. X and Y are the cell
// of its top-left corner and may be negative.
type Chunk struct {
	X, Y          int
	Width, Height int
	// Data holds Width*Height raw GIDs, row by row.
	Data []uint32
}

// rawLayer is a layer as the file stores it.
type rawLayer struct {
	Type           string          `json:"type"`
	ID             int             `json:"id"`
	Name           string          `json:"name"`
	Class          string          `json:"class"`
	OffsetX        float64         `json:"offsetx"`
	OffsetY        float64         `json:"offsety"`
	ParallaxX      *float64        `json:"parallaxx"`
	ParallaxY      *float64        `json:"parallaxy"`
	Opacity        *float64        `json:"opacity"`
	Visible        *bool           `json:"visible"`
	TintColor      string          `json:"tintcolor"`
	Properties     []rawProperty   `json:"properties"`
	StartX         int             `json:"startx"`
	StartY         int             `json:"starty"`
	Width          int             `json:"width"`
	Height         int             `json:"height"`
	Encoding       string          `json:"encoding"`
	Compression    string          `json:"compression"`
	Data           json.RawMessage `json:"data"`
	Chunks         []rawChunk      `json:"chunks"`
	DrawOrder      string          `json:"draworder"`
	Objects        []rawObject     `json:"objects"`
	Image          string          `json:"image"`
	ImageWidth     int             `json:"imagewidth"`
	ImageHeight    int             `json:"imageheight"`
	RepeatX        bool            `json:"repeatx"`
	RepeatY        bool            `json:"repeaty"`
	TransparentKey string          `json:"transparentcolor"`
	Layers         []rawLayer      `json:"layers"`
}

type rawChunk struct {
	X      int             `json:"x"`
	Y      int             `json:"y"`
	Width  int             `json:"width"`
	Height int             `json:"height"`
	Data   json.RawMessage `json:"data"`
}

// decodeLayers converts a list of layers. path is the group path of the
// list ("" at the top), used to give errors context.
func decodeLayers(raw []rawLayer, m *Map, path string) ([]Layer, error) {
	out := make([]Layer, 0, len(raw))
	var errs []error
	for i := range raw {
		p := raw[i].Name
		if path != "" {
			p = path + "/" + p
		}
		l, err := decodeLayer(&raw[i], m, p)
		if err != nil {
			errs = append(errs, err)
		}
		out = append(out, l)
	}
	return out, errors.Join(errs...)
}

func decodeLayer(r *rawLayer, m *Map, path string) (Layer, error) {
	l := Layer{
		ID: r.ID, Name: r.Name, Class: r.Class,
		Offset:   Point{r.OffsetX, r.OffsetY},
		Parallax: Point{floatOr(r.ParallaxX, 1), floatOr(r.ParallaxY, 1)},
		Opacity:  floatOr(r.Opacity, 1),
		Visible:  boolOr(r.Visible, true),
	}
	var errs []error
	add := func(err error) {
		if err != nil {
			errs = append(errs, err)
		}
	}
	tint, err := ParseColor(r.TintColor)
	add(err)
	l.Tint = tint
	props, err := decodeProperties(r.Properties)
	add(err)
	l.Props = props

	switch r.Type {
	case "tilelayer":
		l.Kind = TileLayer
		add(decodeTileData(r, m, &l))
	case "objectgroup":
		l.Kind = ObjectLayer
		l.DrawOrder = r.DrawOrder
		if l.DrawOrder == "" {
			l.DrawOrder = "topdown"
		}
		objs, err := decodeObjects(r.Objects)
		add(err)
		l.Objects = objs
	case "imagelayer":
		l.Kind = ImageLayer
		l.Image, l.ImageWidth, l.ImageHeight = r.Image, r.ImageWidth, r.ImageHeight
		l.RepeatX, l.RepeatY = r.RepeatX, r.RepeatY
		if r.TransparentKey != "" {
			add(unsupported("transparentcolor", "use an image with an alpha channel"))
		}
	case "group":
		l.Kind = GroupLayer
		children, err := decodeLayers(r.Layers, m, path)
		l.Layers = children
		// Children report with their own paths.
		return l, errors.Join(withContext(fmt.Sprintf("layer %q", path), errors.Join(errs...)), err)
	default:
		add(fmt.Errorf("unknown layer type %q", r.Type))
	}
	return l, withContext(fmt.Sprintf("layer %q", path), errors.Join(errs...))
}

// decodeTileData fills the cells of a tile layer: Data for a finite map,
// Chunks for an infinite one.
func decodeTileData(r *rawLayer, m *Map, l *Layer) error {
	l.StartX, l.StartY, l.Width, l.Height = r.StartX, r.StartY, r.Width, r.Height
	if !m.Infinite {
		if r.Width != m.Width || r.Height != m.Height {
			return fmt.Errorf("size %dx%d differs from the map's %dx%d", r.Width, r.Height, m.Width, m.Height)
		}
		data, err := decodeData(r.Data, r.Encoding, r.Compression, r.Width*r.Height)
		l.Data = data
		return err
	}
	if len(r.Data) > 0 && string(r.Data) != "null" {
		return errors.New("infinite map layer has data outside chunks")
	}
	var errs []error
	for i, c := range r.Chunks {
		if c.Width <= 0 || c.Height <= 0 {
			errs = append(errs, fmt.Errorf("chunk %d at (%d,%d): size %dx%d must be positive", i, c.X, c.Y, c.Width, c.Height))
			continue
		}
		data, err := decodeData(c.Data, r.Encoding, r.Compression, c.Width*c.Height)
		if err != nil {
			errs = append(errs, withContext(fmt.Sprintf("chunk %d at (%d,%d)", i, c.X, c.Y), err))
			continue
		}
		l.Chunks = append(l.Chunks, Chunk{X: c.X, Y: c.Y, Width: c.Width, Height: c.Height, Data: data})
	}
	return errors.Join(errs...)
}

func floatOr(f *float64, def float64) float64 {
	if f == nil {
		return def
	}
	return *f
}
