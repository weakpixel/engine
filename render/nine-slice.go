package render

import (
	"image"
	"math"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/weakpixel/engine/gfx"
)

// nineSlice cuts a frame into the pieces of a nine-slice panel of size
// (zero in an axis: the frame's own size), calling emit once per piece with
// the source rectangle (relative to the frame's top-left corner, in pixels)
// and the destination rectangle (relative to the panel's top-left corner).
//
// Insets that exceed the frame are clamped to it. When size is smaller than
// the borders together, the borders shrink in proportion and the center has
// no width. Pieces without area are skipped. With tile the edges and the
// center repeat at the source's pixel size, the last repeat cropped;
// otherwise each piece is one stretched quad. It does not allocate when emit
// does not.
func nineSlice(frame image.Point, in gfx.Insets, size gfx.Vec2, tile bool, emit func(src image.Rectangle, dst gfx.Rect)) {
	if size.X == 0 {
		size.X = float32(frame.X)
	}
	if size.Y == 0 {
		size.Y = float32(frame.Y)
	}
	if size.X <= 0 || size.Y <= 0 || frame.X <= 0 || frame.Y <= 0 {
		return
	}
	sx, dx := axisSlices(frame.X, int(in.Left), int(in.Right), size.X)
	sy, dy := axisSlices(frame.Y, int(in.Top), int(in.Bottom), size.Y)
	for row := range 3 {
		for col := range 3 {
			src := image.Rect(sx[col], sy[row], sx[col+1], sy[row+1])
			dst := gfx.Rect{
				Min: gfx.Vec2{X: dx[col], Y: dy[row]},
				Max: gfx.Vec2{X: dx[col+1], Y: dy[row+1]},
			}
			if src.Empty() || dst.Max.X <= dst.Min.X || dst.Max.Y <= dst.Min.Y {
				continue
			}
			// Corners (col and row both 0 or 2) never repeat.
			if !tile || (col != 1 && row != 1) {
				emit(src, dst)
				continue
			}
			tilePiece(src, dst, col == 1, row == 1, emit)
		}
	}
}

// axisSlices returns the four cut positions of one axis: in the source
// (0, near, n-far, n) and in the panel (0, near, size-far, size) after
// clamping the borders to the frame and shrinking them to fit size.
func axisSlices(n, near, far int, size float32) (src [4]int, dst [4]float32) {
	near = min(near, n)
	far = min(far, n-near)
	src = [4]int{0, near, n - far, n}
	dn, df := float32(near), float32(far)
	if sum := dn + df; sum > size {
		k := size / sum
		dn, df = dn*k, df*k
	}
	dst = [4]float32{0, dn, size - df, size}
	return src, dst
}

// tilePiece repeats src across dst along the axes that stretch (x for the
// top and bottom edges and the center, y for the side edges and the center).
func tilePiece(src image.Rectangle, dst gfx.Rect, repeatX, repeatY bool, emit func(src image.Rectangle, dst gfx.Rect)) {
	stepX, stepY := dst.Max.X-dst.Min.X, dst.Max.Y-dst.Min.Y
	if repeatX {
		stepX = float32(src.Dx())
	}
	if repeatY {
		stepY = float32(src.Dy())
	}
	for y := dst.Min.Y; y < dst.Max.Y; y += stepY {
		h := min(stepY, dst.Max.Y-y)
		for x := dst.Min.X; x < dst.Max.X; x += stepX {
			w := min(stepX, dst.Max.X-x)
			// A fractional last repeat takes the next whole source pixel and
			// squeezes it into the remaining space.
			crop := image.Rect(src.Min.X, src.Min.Y,
				src.Min.X+min(src.Dx(), int(math.Ceil(float64(w)))),
				src.Min.Y+min(src.Dy(), int(math.Ceil(float64(h)))))
			emit(crop, gfx.Rect{Min: gfx.Vec2{X: x, Y: y}, Max: gfx.Vec2{X: x + w, Y: y + h}})
		}
	}
}

// drawNineSlice draws the pieces of the item's panel into the batch. All
// pieces share the sprite's texture, so they join one draw call (and the
// neighbouring sprites of that texture).
func (r *Renderer) drawNineSlice(dst *ebiten.Image, it *Item, t transform) {
	sp := r.stores.Sprites.Get(it.Sprite)
	if sp == r.placeholder {
		r.stats.Placeholders++
	}
	frame := sp.Image.Bounds()
	size := it.Size
	if size.X == 0 {
		size.X = float32(frame.Dx())
	}
	if size.Y == 0 {
		size.Y = float32(frame.Dy())
	}
	pivot := gfx.Vec2{X: it.Origin.X * size.X, Y: it.Origin.Y * size.Y}
	r.setStyle(it.Color, it.Blend)
	src := sp.source()
	nineSlice(frame.Size(), it.Insets, it.Size, it.Tile, func(rect image.Rectangle, d gfx.Rect) {
		g := &r.opts.GeoM
		g.Reset()
		// Stretch the piece to its panel rectangle, then place the panel
		// like a sprite: pivot at t.pos, scaled and rotated around it.
		g.Scale(float64((d.Max.X-d.Min.X)/float32(rect.Dx())), float64((d.Max.Y-d.Min.Y)/float32(rect.Dy())))
		g.Translate(float64(d.Min.X-pivot.X), float64(d.Min.Y-pivot.Y))
		g.Scale(float64(t.scale.X), float64(t.scale.Y))
		if t.rot != 0 {
			g.Rotate(float64(t.rot))
		}
		g.Translate(float64(t.pos.X), float64(t.pos.Y))
		r.addQuad(dst, src, rect.Add(frame.Min), g, &r.opts.ColorScale, r.blend)
	})
}
