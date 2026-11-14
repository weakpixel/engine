package tileset

import (
	"encoding/xml"
	"errors"
	"fmt"
	"image"
	"io"
)

// SubTexture is one <SubTexture> of a Starling/ShoeBox TextureAtlas.
type SubTexture struct {
	Name string
	// Rect is the sub-texture's pixels in the atlas image.
	Rect image.Rectangle
	// Frame is the size of the untrimmed sprite, and Trim the offset of Rect
	// inside it (the negated frameX and frameY). Without frame attributes
	// the frame is Rect's size and Trim is zero.
	Frame image.Point
	Trim  image.Point
	// Rotated means the atlas stores the pixels turned 90 degrees clockwise;
	// the tile gets them turned back. Size is then Rect's size swapped.
	Rotated bool
}

// Size is the size of the sub-texture's pixels once upright.
func (s SubTexture) Size() image.Point {
	if s.Rotated {
		return image.Pt(s.Rect.Dy(), s.Rect.Dx())
	}
	return s.Rect.Size()
}

// Atlas is a parsed TextureAtlas: the image it names and its sub-textures in
// file order.
type Atlas struct {
	ImagePath   string
	SubTextures []SubTexture
}

// xmlAtlas and xmlSubTexture mirror the XML. Starling writes frameX and
// frameY as zero or negative numbers ("-0" included).
type xmlAtlas struct {
	ImagePath   string          `xml:"imagePath,attr"`
	SubTextures []xmlSubTexture `xml:"SubTexture"`
}

type xmlSubTexture struct {
	Name        string `xml:"name,attr"`
	X           int    `xml:"x,attr"`
	Y           int    `xml:"y,attr"`
	Width       int    `xml:"width,attr"`
	Height      int    `xml:"height,attr"`
	FrameX      int    `xml:"frameX,attr"`
	FrameY      int    `xml:"frameY,attr"`
	FrameWidth  int    `xml:"frameWidth,attr"`
	FrameHeight int    `xml:"frameHeight,attr"`
	Rotated     bool   `xml:"rotated,attr"`
}

// ParseAtlas reads a TextureAtlas and checks everything that does not need
// the image: positive sizes, a frame that holds the trimmed pixels, no
// rotation, unique names. All problems are returned together, each naming
// its sub-texture.
func ParseAtlas(r io.Reader) (*Atlas, error) {
	var x xmlAtlas
	if err := xml.NewDecoder(r).Decode(&x); err != nil {
		return nil, fmt.Errorf("invalid atlas XML: %w", err)
	}
	a := &Atlas{ImagePath: x.ImagePath}
	var errs []error
	seen := map[string]bool{}
	for i, s := range x.SubTextures {
		bad := func(format string, args ...any) {
			errs = append(errs, fmt.Errorf("SubTexture %q: "+format, append([]any{s.Name}, args...)...))
		}
		if s.Name == "" {
			errs = append(errs, fmt.Errorf("SubTexture #%d: missing name", i))
		} else if seen[s.Name] {
			bad("duplicate name")
		}
		seen[s.Name] = true
		st := SubTexture{
			Name:  s.Name,
			Rect:  image.Rect(s.X, s.Y, s.X+s.Width, s.Y+s.Height),
			Frame: image.Pt(s.FrameWidth, s.FrameHeight),
			Trim:  image.Pt(-s.FrameX, -s.FrameY),
		}
		// ShoeBox does not always set rotated: a frame that is the rect
		// with width and height swapped marks a rotated sub-texture too.
		st.Rotated = s.Rotated || (s.Width != s.Height &&
			s.FrameWidth == s.Height && s.FrameHeight == s.Width)
		size := st.Size()
		if s.FrameWidth == 0 && s.FrameHeight == 0 {
			st.Frame = size
		}
		switch {
		case s.Width <= 0 || s.Height <= 0:
			bad("size %dx%d must be positive", s.Width, s.Height)
		case s.X < 0 || s.Y < 0:
			bad("position %d,%d must not be negative", s.X, s.Y)
		case st.Trim.X < 0 || st.Trim.Y < 0 ||
			st.Trim.X+size.X > st.Frame.X || st.Trim.Y+size.Y > st.Frame.Y:
			bad("frame %dx%d with offset %d,%d does not hold the %dx%d pixels",
				st.Frame.X, st.Frame.Y, st.Trim.X, st.Trim.Y, size.X, size.Y)
		}
		a.SubTextures = append(a.SubTextures, st)
	}
	if len(x.SubTextures) == 0 {
		errs = append(errs, errors.New("atlas has no SubTexture"))
	}
	return a, errors.Join(errs...)
}

// CheckBounds reports every sub-texture that reaches outside an image of the
// given size.
func (a *Atlas) CheckBounds(bounds image.Rectangle) error {
	var errs []error
	for _, s := range a.SubTextures {
		if !s.Rect.In(bounds) {
			errs = append(errs, fmt.Errorf("SubTexture %q: %v is outside the %dx%d image",
				s.Name, s.Rect, bounds.Dx(), bounds.Dy()))
		}
	}
	return errors.Join(errs...)
}
