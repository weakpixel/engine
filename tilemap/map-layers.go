package tilemap

import (
	"image"

	"github.com/weakpixel/engine/gfx"
	"github.com/weakpixel/engine/render"
	"github.com/weakpixel/engine/tiled"
)

// inherited is what a group passes on to its children: Tiled composes
// offsets by adding, opacity, tint and parallax by multiplying, and
// visibility by "and".
type inherited struct {
	parent   int
	path     string
	offset   gfx.Vec2
	opacity  float32
	tint     gfx.Color
	parallax gfx.Vec2
	visible  bool
}

// flattenLayers lists the layer tree depth-first, bottom to top, each group
// before its children, with effective values. images maps image-layer paths
// to their resolved sprites, shaders maps layer shader keys to theirs.
func flattenLayers(tree []tiled.Layer, images map[string]gfx.SpriteHandle, shaders map[string]gfx.ShaderHandle, sprites render.SpriteStore) []Layer {
	var out []Layer
	var walk func([]tiled.Layer, inherited)
	walk = func(ls []tiled.Layer, in inherited) {
		for i := range ls {
			src := &ls[i]
			l := Layer{
				ID: src.ID, Name: src.Name, Path: in.path + src.Name, Parent: in.parent,
				Class: src.Class, Props: src.Props,
				Offset:   in.offset.Add(vec(src.Offset)),
				Opacity:  in.opacity * float32(src.Opacity),
				Tint:     mulColor(in.tint, gfx.Color(src.Tint)),
				Parallax: gfx.Vec2{X: in.parallax.X * float32(src.Parallax.X), Y: in.parallax.Y * float32(src.Parallax.Y)},
				Visible:  in.visible && src.Visible,
			}
			switch src.Kind {
			case tiled.TileLayer:
				l.Kind = Tiles
				l.Origin, l.W, l.H, l.GIDs = cells(src)
			case tiled.ObjectLayer:
				l.Kind = Objects
				l.Objects, l.DrawOrder = src.Objects, src.DrawOrder
			case tiled.ImageLayer:
				l.Kind = Image
				l.RepeatX, l.RepeatY = src.RepeatX, src.RepeatY
				if h, ok := images[src.Image]; ok {
					l.Image, l.ImageSize = h, sprites.Get(h).Size
				}
			case tiled.GroupLayer:
				l.Kind = Group
			}
			if key := layerShaderKey(src); key != "" {
				l.Shader = shaders[key]
			}
			out = append(out, l)
			if src.Kind == tiled.GroupLayer {
				walk(src.Layers, inherited{
					parent: len(out) - 1, path: l.Path + "/", offset: l.Offset, opacity: l.Opacity,
					tint: l.Tint, parallax: l.Parallax, visible: l.Visible,
				})
			}
		}
	}
	walk(tree, inherited{parent: -1, opacity: 1, parallax: gfx.Vec2{X: 1, Y: 1}, visible: true})
	return out
}

// cells returns a tile layer's cells as one dense grid. A finite layer's data
// is used as is. An infinite layer's chunks are merged into a grid covering
// their union, which costs memory for sparse maps but keeps cell lookups and
// the extract's culling a plain index computation.
func cells(l *tiled.Layer) (origin image.Point, w, h int, gids []uint32) {
	if l.Chunks == nil {
		return image.Point{}, l.Width, l.Height, l.Data
	}
	var bounds image.Rectangle
	for _, c := range l.Chunks {
		bounds = bounds.Union(image.Rect(c.X, c.Y, c.X+c.Width, c.Y+c.Height))
	}
	w, h = bounds.Dx(), bounds.Dy()
	gids = make([]uint32, w*h)
	for _, c := range l.Chunks {
		for y := range c.Height {
			row := (c.Y-bounds.Min.Y+y)*w + c.X - bounds.Min.X
			copy(gids[row:row+c.Width], c.Data[y*c.Width:(y+1)*c.Width])
		}
	}
	return bounds.Min, w, h, gids
}

// mulColor multiplies two colors channel by channel. The zero Color means
// "no tint" (white); the product of two such is zero again.
func mulColor(a, b gfx.Color) gfx.Color {
	if a == 0 {
		return b
	}
	if b == 0 {
		return a
	}
	var out gfx.Color
	for shift := 0; shift < 32; shift += 8 {
		ca, cb := uint32(a>>shift&0xff), uint32(b>>shift&0xff)
		out |= gfx.Color((ca*cb+127)/255) << shift
	}
	return out
}

// withOpacity returns c with its alpha multiplied by opacity. The zero Color
// counts as opaque white.
func withOpacity(c gfx.Color, opacity float32) gfx.Color {
	if opacity >= 1 {
		return c
	}
	if c == 0 {
		c = gfx.White
	}
	a := float32(c&0xff) * max(opacity, 0)
	out := c&^0xff | gfx.Color(uint8(a+0.5))
	if out == 0 {
		// Fully transparent black would read as "no tint"; keep it
		// transparent.
		out = gfx.RGBA(255, 255, 255, 0)
	}
	return out
}
