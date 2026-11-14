package tiled

import (
	"fmt"
	"strconv"
	"strings"
)

// Color is a packed 0xRRGGBBAA color, non-premultiplied, with the same layout
// as gfx.Color, so gfx.Color(c) converts it. The zero Color means "not set"
// (an absent tint or background); "#00000000" decodes to it too.
type Color uint32

// RGBA returns the channels.
func (c Color) RGBA() (r, g, b, a uint8) {
	return uint8(c >> 24), uint8(c >> 16), uint8(c >> 8), uint8(c)
}

// ParseColor reads a Tiled color: "#RRGGBB" (opaque) or "#AARRGGBB". The '#'
// is optional. The empty string is the zero Color.
func ParseColor(s string) (Color, error) {
	if s == "" {
		return 0, nil
	}
	hex := strings.TrimPrefix(s, "#")
	if len(hex) != 6 && len(hex) != 8 {
		return 0, fmt.Errorf("color %q: want #RRGGBB or #AARRGGBB", s)
	}
	v, err := strconv.ParseUint(hex, 16, 32)
	if err != nil {
		return 0, fmt.Errorf("color %q: want #RRGGBB or #AARRGGBB", s)
	}
	if len(hex) == 6 {
		return Color(v<<8 | 0xff), nil
	}
	// #AARRGGBB -> 0xRRGGBBAA
	return Color(v<<8 | v>>24), nil
}

// String returns the color as Tiled writes it, "#AARRGGBB".
func (c Color) String() string {
	r, g, b, a := c.RGBA()
	return fmt.Sprintf("#%02x%02x%02x%02x", a, r, g, b)
}
