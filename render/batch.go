package render

import (
	"image"

	"github.com/hajimehoshi/ebiten/v2"
)

// This file batches quads. Unshaded sprites and the segments of shapes are
// all textured quads; consecutive ones that share a target image, a source
// image and a blend mode go into one DrawTriangles32 call instead of one
// DrawImage call each. Shaded sprites and canvases are quads too: consecutive
// ones that also share the shader and Params go into one
// DrawTrianglesShader32 call instead of one DrawRectShader call each. There
// is one pending batch at a time, so draw order is kept: triangles of one
// call draw in index order, and anything that cannot join the batch (text,
// a quad with another key, a group switch or composite, the end of Draw)
// flushes it first.
//
// Ebiten merges consecutive DrawImage calls into one GPU command already,
// but every call still pays its own setup on the CPU (option handling,
// shader lookup, atlas bookkeeping). With thousands of tiles or particles
// that setup dominates; a batch pays it once.
//
// Atlas frames batch together because they are drawn from their texture
// (Sprite.sheet), not from their sub-image: the source coordinates are the
// frame's rectangle, which is the sub-image's bounds. That samples exactly
// what DrawImage of the sub-image samples; both use Ebiten's unchecked
// address mode, so neither clamps to the frame.
//
// Shaded quads are drawn from the frame's sub-image instead, not from the
// sheet: a shader sees the frame as imageSrc0 (imageSrc0At is transparent
// outside it, imageSrc0Origin and imageSrc0Size are the frame), exactly as
// with DrawRectShader. So different frames of one sheet are different
// shaded batches. Ebiten could not merge them on the GPU anyway: it passes
// each source region to the shader as uniforms, and merges only draws with
// equal uniforms.
//
// A shaded quad may have a second image, Item.Aux, which the shader reads as
// imageSrc1. It is part of the batch key like the first one. In pixel mode
// Ebiten allows source images of different sizes, so Aux need not match the
// frame. imageSrc1At takes imageSrc0Origin plus a position inside Aux's
// region, so a shader maps its position there first (see the guide's
// shaders page).

// quadBatch is the pending batch: its quads' vertices and what they share.
// The vertex and index slices are reused across batches and frames, so a
// steady-state Draw allocates nothing for them.
type quadBatch struct {
	dst, src *ebiten.Image
	blend    Blend
	// shader is the shader of a shaded batch (nil: unshaded) and params
	// its Params uniform, copied into the Renderer's uniform map at flush.
	// aux is the shader's second image, imageSrc1 (nil: none).
	shader *ebiten.Shader
	aux    *ebiten.Image
	params [4]float32
	verts  []ebiten.Vertex
	// indices is the fixed two-triangle pattern of every quad, grown to the
	// largest batch so far; a batch of n quads uses indices[:6*n].
	indices []uint32
	opts    ebiten.DrawTrianglesOptions
}

// addQuad queues the src pixels inside rect (in src's bounds coordinates),
// placed by g and scaled by the premultiplied cs, for drawing onto dst. It
// flushes the pending batch first if that one draws onto another target,
// from another source or with another blend mode.
func (r *Renderer) addQuad(dst, src *ebiten.Image, rect image.Rectangle, g *ebiten.GeoM, cs *ebiten.ColorScale, blend Blend) {
	r.queueQuad(dst, src, nil, rect, g, cs, blend, nil, nil)
	r.stats.Quads++
}

// addShadedQuad queues all of img drawn through sh with Params params and
// the second image aux (nil: none), placed by g and scaled by the
// premultiplied cs (the shader's color argument), onto dst. It flushes the
// pending batch first if that one has another target, image, second image,
// blend mode, shader or Params.
func (r *Renderer) addShadedQuad(dst, img, aux *ebiten.Image, g *ebiten.GeoM, cs *ebiten.ColorScale, blend Blend, sh *ebiten.Shader, params *[4]float32) {
	r.queueQuad(dst, img, aux, img.Bounds(), g, cs, blend, sh, params)
	r.stats.ShadedQuads++
}

// queueQuad adds one quad to the pending batch after flushing it if its key
// (target, source, second image, blend, shader, Params) differs. aux and
// params are nil for unshaded quads.
func (r *Renderer) queueQuad(dst, src, aux *ebiten.Image, rect image.Rectangle, g *ebiten.GeoM, cs *ebiten.ColorScale, blend Blend, sh *ebiten.Shader, params *[4]float32) {
	b := &r.batch
	if len(b.verts) > 0 && (b.dst != dst || b.src != src || b.aux != aux || b.blend != blend || b.shader != sh || (sh != nil && b.params != *params)) {
		r.flush()
	}
	b.dst, b.src, b.aux, b.blend, b.shader = dst, src, aux, blend, sh
	if sh != nil {
		b.params = *params
	}
	// The corners are computed like DrawImage and DrawRectShader compute
	// them (float32 matrix elements, snapped destination), so a batched quad
	// covers exactly the pixels the same unbatched call would.
	a, bb, c, d := float32(g.Element(0, 0)), float32(g.Element(0, 1)), float32(g.Element(1, 0)), float32(g.Element(1, 1))
	tx, ty := float32(g.Element(0, 2)), float32(g.Element(1, 2))
	w, h := float32(rect.Dx()), float32(rect.Dy())
	ax, by, cx, dy := a*w, bb*h, c*w, d*h
	sx0, sy0 := float32(rect.Min.X), float32(rect.Min.Y)
	sx1, sy1 := float32(rect.Max.X), float32(rect.Max.Y)
	v := ebiten.Vertex{ColorR: cs.R(), ColorG: cs.G(), ColorB: cs.B(), ColorA: cs.A()}
	corner := func(x, y, sx, sy float32) ebiten.Vertex {
		v.DstX, v.DstY, v.SrcX, v.SrcY = snapPixel(x), snapPixel(y), sx, sy
		return v
	}
	b.verts = append(b.verts,
		corner(tx, ty, sx0, sy0),
		corner(ax+tx, cx+ty, sx1, sy0),
		corner(by+tx, dy+ty, sx0, sy1),
		corner(ax+by+tx, cx+dy+ty, sx1, sy1),
	)
}

// snapPixel moves a destination coordinate off pixel centers, to about a
// third of a pixel, exactly like Ebiten does for DrawImage
// (adjustDestinationPixel in ebiten/internal/graphics, see Ebiten issues
// #929 and #1171). DrawTriangles does not snap, so without this a sprite at
// a half-pixel position would cover different pixels batched than unbatched,
// and tiles could show seams.
func snapPixel(x float32) float32 {
	ix := float32(int(x))
	if x < 0 && x != ix {
		ix--
	}
	switch frac := x - ix; {
	case frac < 3.0/16:
		return ix
	case frac < 8.0/16:
		return ix + 5.0/16
	case frac < 13.0/16:
		return ix + 11.0/16
	default:
		return ix + 1
	}
}

// flush draws the pending batch, if any, with one DrawTriangles32 call, or
// one DrawTrianglesShader32 call for a shaded batch.
func (r *Renderer) flush() {
	b := &r.batch
	n := len(b.verts) / 4
	if n == 0 {
		return
	}
	for q := len(b.indices) / 6; q < n; q++ {
		v := uint32(4 * q)
		b.indices = append(b.indices, v, v+1, v+2, v+1, v+3, v+2)
	}
	if b.shader != nil {
		r.flushShaded(n)
	} else {
		// Vertex colors are premultiplied, like DrawImage's ColorScale.
		b.opts.ColorScaleMode = ebiten.ColorScaleModePremultipliedAlpha
		b.opts.Blend = ebitenBlend(b.blend)
		b.dst.DrawTriangles32(b.verts, b.indices[:6*n], b.src, &b.opts)
	}
	r.stats.DrawCalls++
	b.verts = b.verts[:0]
	// Don't keep images or shaders alive (or deallocated ones referenced)
	// past Draw.
	b.dst, b.src, b.aux, b.shader = nil, nil, nil, nil
}

// flushShaded draws the n quads of a shaded batch. The vertex colors are the
// premultiplied ColorScale that DrawRectShader would pass as the shader's
// color argument.
func (r *Renderer) flushShaded(n int) {
	b := &r.batch
	o := &r.sopts
	o.Blend = ebitenBlend(b.blend)
	o.Images[0], o.Images[1] = b.src, b.aux
	// In place, never r.sopts.Uniforms[...] = ...: see newUniforms.
	copy(r.params, b.params[:])
	b.dst.DrawTrianglesShader32(b.verts, b.indices[:6*n], b.shader, o)
	o.Images[0], o.Images[1] = nil, nil
	r.stats.ShaderDraws++
}
