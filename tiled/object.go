package tiled

import (
	"errors"
	"fmt"
)

// Point is a position or size in pixels.
type Point struct {
	X, Y float64
}

// Shape is the kind of an [Object].
type Shape uint8

// Object shapes.
const (
	// ShapeRect is a rectangle: X, Y is the top-left corner (before Rotation)
	// and Width, Height its size. A zero-size rectangle is valid.
	ShapeRect Shape = iota
	// ShapePoint is a point at X, Y.
	ShapePoint
	// ShapeEllipse is the ellipse inside the rectangle of a ShapeRect.
	ShapeEllipse
	// ShapePolygon is a closed polygon; Points are relative to X, Y.
	ShapePolygon
	// ShapePolyline is an open polyline; Points are relative to X, Y.
	ShapePolyline
	// ShapeText is a text box; see [Object.Text].
	ShapeText
	// ShapeTile is a tile object: GID and Flip select the tile, X, Y is its
	// bottom-left corner (Tiled's anchor for tile objects in orthogonal maps)
	// and Width, Height its drawn size.
	ShapeTile
)

var shapeNames = [...]string{"rect", "point", "ellipse", "polygon", "polyline", "text", "tile"}

// String returns the lower-case shape name.
func (s Shape) String() string {
	if int(s) < len(shapeNames) {
		return shapeNames[s]
	}
	return "invalid"
}

// Object is an object of an object layer, or a collision shape of a tile.
// Positions are pixels in the coordinate space of its container: the map for
// object layers, the tile for collision shapes. Rotation is in degrees,
// clockwise, around X, Y.
type Object struct {
	ID   int
	Name string
	// Class is the object's "class", or its "type" when the file has no class
	// (Tiled before 1.9 and files saved for compatibility write "type").
	Class string
	Shape Shape
	X, Y  float64
	// Width and Height are the size of rectangles, ellipses, text boxes and
	// tile objects. Zero for points, polygons and polylines.
	Width, Height float64
	Rotation      float64
	Visible       bool
	// Points are the vertices of polygons and polylines, relative to X, Y.
	Points []Point
	// GID and Flip select the tile of a ShapeTile object.
	GID  uint32
	Flip Flip
	// Text is set for ShapeText objects.
	Text  *Text
	Props Properties
}

// Text is the text of a [ShapeText] object.
type Text struct {
	Text       string
	FontFamily string
	PixelSize  int
	Wrap       bool
	Color      Color
	Bold       bool
	Italic     bool
	Underline  bool
	Strikeout  bool
	Kerning    bool
	// HAlign is "left", "center", "right" or "justify"; VAlign is "top",
	// "center" or "bottom".
	HAlign, VAlign string
}

// rawObject is an object as the file stores it.
type rawObject struct {
	ID         int           `json:"id"`
	Name       string        `json:"name"`
	Type       string        `json:"type"`
	Class      string        `json:"class"`
	X          float64       `json:"x"`
	Y          float64       `json:"y"`
	Width      float64       `json:"width"`
	Height     float64       `json:"height"`
	Rotation   float64       `json:"rotation"`
	Visible    *bool         `json:"visible"`
	GID        *uint32       `json:"gid"`
	Point      bool          `json:"point"`
	Ellipse    bool          `json:"ellipse"`
	Polygon    []Point       `json:"polygon"`
	Polyline   []Point       `json:"polyline"`
	Text       *rawText      `json:"text"`
	Template   string        `json:"template"`
	Properties []rawProperty `json:"properties"`
}

type rawText struct {
	Text       string `json:"text"`
	FontFamily string `json:"fontfamily"`
	PixelSize  *int   `json:"pixelsize"`
	Wrap       bool   `json:"wrap"`
	Color      string `json:"color"`
	Bold       bool   `json:"bold"`
	Italic     bool   `json:"italic"`
	Underline  bool   `json:"underline"`
	Strikeout  bool   `json:"strikeout"`
	Kerning    *bool  `json:"kerning"`
	HAlign     string `json:"halign"`
	VAlign     string `json:"valign"`
}

// classOf implements the class-over-type rule (see [Object.Class]).
func classOf(class, typ string) string {
	if class != "" {
		return class
	}
	return typ
}

// boolOr returns *b, or def when the file omits the field.
func boolOr(b *bool, def bool) bool {
	if b == nil {
		return def
	}
	return *b
}

// decodeObjects converts the objects of a layer or tile, collecting errors.
func decodeObjects(raw []rawObject) ([]Object, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	out := make([]Object, 0, len(raw))
	var errs []error
	for i := range raw {
		o, err := decodeObject(&raw[i])
		if err != nil {
			errs = append(errs, withContext(fmt.Sprintf("object %d", raw[i].ID), err))
			continue
		}
		out = append(out, o)
	}
	return out, errors.Join(errs...)
}

func decodeObject(r *rawObject) (Object, error) {
	o := Object{
		ID: r.ID, Name: r.Name, Class: classOf(r.Class, r.Type),
		X: r.X, Y: r.Y, Width: r.Width, Height: r.Height, Rotation: r.Rotation,
		Visible: boolOr(r.Visible, true),
	}
	var errs []error
	if r.Template != "" {
		errs = append(errs, unsupported("object templates", fmt.Sprintf("%q; detach the template in Tiled", r.Template)))
	}
	switch {
	case r.GID != nil:
		o.Shape = ShapeTile
		o.GID, o.Flip = SplitGID(*r.GID)
	case r.Point:
		o.Shape = ShapePoint
	case r.Ellipse:
		o.Shape = ShapeEllipse
	case r.Polygon != nil:
		o.Shape, o.Points = ShapePolygon, r.Polygon
	case r.Polyline != nil:
		o.Shape, o.Points = ShapePolyline, r.Polyline
	case r.Text != nil:
		o.Shape = ShapeText
		t, err := decodeText(r.Text)
		if err != nil {
			errs = append(errs, err)
		}
		o.Text = t
	default:
		o.Shape = ShapeRect
	}
	props, err := decodeProperties(r.Properties)
	if err != nil {
		errs = append(errs, err)
	}
	o.Props = props
	return o, errors.Join(errs...)
}

func decodeText(r *rawText) (*Text, error) {
	t := &Text{
		Text: r.Text, FontFamily: r.FontFamily, PixelSize: 16, Wrap: r.Wrap,
		Bold: r.Bold, Italic: r.Italic, Underline: r.Underline, Strikeout: r.Strikeout,
		Kerning: boolOr(r.Kerning, true), HAlign: r.HAlign, VAlign: r.VAlign,
	}
	if t.FontFamily == "" {
		t.FontFamily = "sans-serif"
	}
	if r.PixelSize != nil {
		t.PixelSize = *r.PixelSize
	}
	if t.HAlign == "" {
		t.HAlign = "left"
	}
	if t.VAlign == "" {
		t.VAlign = "top"
	}
	t.Color = Color(0x000000ff) // Tiled's default text color
	if r.Color != "" {
		c, err := ParseColor(r.Color)
		if err != nil {
			return t, fmt.Errorf("text: %w", err)
		}
		t.Color = c
	}
	return t, nil
}
