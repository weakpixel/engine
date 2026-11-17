package render

import (
	"image"
	"image/color"
	"math"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/text/v2"

	"github.com/weakpixel/engine/gfx"
	"github.com/weakpixel/engine/xmath"
)

// Stats describes the last Draw call.
type Stats struct {
	// Items is the number of queue items drawn.
	Items int
	// DrawCalls is the number of Ebiten draw calls issued (Ebiten may merge
	// consecutive calls into fewer GPU batches). A batch of quads (see
	// Quads) is one draw call.
	DrawCalls int
	// Quads is how many unshaded textured quads went into batched draw
	// calls: one per unshaded sprite, and one per edge or segment of a rect,
	// line or circle. Consecutive quads with the same target, source texture
	// and blend mode share one draw call.
	Quads int
	// ShadedQuads is how many items were drawn through their shader
	// (Item.Shader resolved to a loaded shader), one quad each. Consecutive
	// sprites and canvases with the same target, image (atlas frame), blend
	// mode, shader and Params share one draw call; a shaded text, nine-slice
	// or shape is always a draw call of its own, plus the calls that draw
	// it into the scratch image first (counted in DrawCalls and Quads).
	ShadedQuads int
	// ShaderDraws is how many of the draw calls went through a shader: the
	// batches of shaded quads, plus shaded layer group composites and post
	// passes.
	ShaderDraws int
	// Groups is how many layer groups were composited (see [GroupConfig]).
	// Their composite draws count in DrawCalls.
	Groups int
	// Placeholders is how many sprite items drew the placeholder sprite
	// because their handle was zero, stale, loading or failed. A steady
	// non-zero value usually means a wrong asset key.
	Placeholders int
	// Culled is how many items the extract systems skipped because they
	// were outside the camera bounds, as reported by [Queue.Culled]. It is
	// not a renderer count: culling happens in extract, not in Draw.
	Culled int
	// PostPasses is how many post-processing passes ran (see [PostChain]),
	// including the final pass of [Renderer.DrawFinal]. Each one is also a
	// draw call and a shader draw.
	PostPasses int
}

// circleSegments is how many line segments approximate a circle outline.
const circleSegments = 32

// Renderer turns a sorted Queue into Ebiten draw calls. Sprites and shapes
// are batched: consecutive unshaded ones that share a target, a source
// texture and a blend mode become one DrawTriangles call, consecutive shaded
// ones that share a target, an image, a blend mode, a shader and Params one
// DrawTrianglesShader call. It owns reused draw options and vertex buffers,
// so steady-state sprite and shape drawing allocates nothing. It has no access to the ecs World.
type Renderer struct {
	stores Stores
	// canvases resolves KindCanvas items (nil: they draw nothing).
	canvases *Canvases
	// opts holds the placement (GeoM) and style of the item being drawn;
	// batched quads read them from here too.
	opts ebiten.DrawImageOptions
	// batch is the pending quad batch (see batch.go). noBatch draws every
	// quad with its own call instead (DrawImage, or DrawTrianglesShader for a
	// shaded quad); benchmarks use it to compare both paths.
	batch   quadBatch
	noBatch bool
	// scratch is where shaded text, nine-slices and shapes are drawn before
	// their shader runs (see shade-scratch.go).
	scratch shadeScratch
	// blend is the Blend that setStyle last put into opts, for batching.
	blend Blend
	topts text.DrawOptions
	// sopts and its Uniforms map are reused for every shaded batch; params
	// is the slice behind the map's Params entry (see newUniforms).
	sopts  ebiten.DrawTrianglesShaderOptions
	params []float32

	// groups is the layer group configuration (nil: no groups). targets
	// holds each group's offscreen image by GroupID and below the copy for
	// SampleBelow; both are allocated on first use and when the screen size
	// changes, never per frame.
	groups  *GroupConfig
	targets []*ebiten.Image
	below   *ebiten.Image
	// gopts, gsopts and gparams are the reused options of group composites
	// (gparams backs gsopts' Params uniform, see newUniforms).
	gopts   ebiten.DrawImageOptions
	gsopts  ebiten.DrawTrianglesShaderOptions
	gparams []float32
	// next is the position in groups.order of the first group Draw has not
	// reached yet, and active the group being drawn into (-1: none).
	next   int
	active GroupID
	// frame counts Draws and begun[g] is the frame group g's image was
	// last cleared in, so a pass reading a group no item fell into can
	// tell its image is stale (see postInput).
	frame uint64
	begun []uint64

	// post is the post-processing chain (nil: none). postImgs are the
	// frame image and the ping-pong image of its passes, allocated on first
	// use and when the screen size changes. popts, psopts/pparams and
	// fsopts/fparams are the reused options of the copy, the passes and the
	// final pass (each params slice backs its map's Params uniform).
	post     *PostChain
	postImgs [2]*ebiten.Image
	// postAlt is the third chain image of chains that read TargetFrame
	// after writing the chain, ptargets the images of PostChain.Targets,
	// hist the history images behind TargetPrevious (histIdx is the one
	// holding the previous frame, histOK that it does).
	postAlt  *ebiten.Image
	builtins [builtinCount]*ebiten.Shader // compiled on first use
	ptargets []*ebiten.Image
	hist     [2]*ebiten.Image
	histIdx  int
	histOK   bool
	popts    ebiten.DrawImageOptions
	psopts   ebiten.DrawTrianglesShaderOptions
	pverts   [4]ebiten.Vertex
	pparams  []float32
	fsopts   ebiten.DrawTrianglesShaderOptions
	fparams  []float32
	fadeopts ebiten.DrawImageOptions // PostChain.Fade
	// time is the one-element slice behind the Time uniform of all four
	// uniform maps above (see newUniforms and SetTime).
	time []float32
	// white is a 1x1 white image: rects, lines and circles are scaled,
	// rotated and tinted copies of it, which is allocation-free (unlike
	// vector paths).
	white *ebiten.Image
	// logical screen size of the current Draw.
	screen image.Point
	// placeholder is the sprite store's placeholder for the current Draw:
	// Get returns exactly this pointer for any handle that is not a ready
	// sprite, so one pointer compare counts Stats.Placeholders. It is
	// refreshed every Draw because the pointer is only valid until the next
	// asset Server.Update.
	placeholder *Sprite
	stats       Stats
}

// NewRenderer returns a Renderer that resolves sprite and font handles
// through st. See [StoresOf].
func NewRenderer(st Stores) *Renderer {
	white := ebiten.NewImage(1, 1)
	white.Fill(color.White)
	r := &Renderer{stores: st, white: white, active: -1, time: make([]float32, 1)}
	r.sopts.Uniforms, r.params = newUniforms(r.time)
	r.gsopts.Uniforms, r.gparams = newUniforms(r.time)
	r.psopts.Uniforms, r.pparams = newUniforms(r.time)
	r.fsopts.Uniforms, r.fparams = newUniforms(r.time)
	return r
}

// SetGroups makes the Renderer draw the layer groups of g; nil means no
// groups. The Renderer reads g in every Draw, so later changes through
// [GroupConfig.Set] apply from the next frame.
func (r *Renderer) SetGroups(g *GroupConfig) { r.groups = g }

// SetCanvases makes the Renderer draw KindCanvas items from c; nil (the
// default) means they draw nothing.
func (r *Renderer) SetCanvases(c *Canvases) { r.canvases = c }

// SetPost sets the post-processing chain Draw and DrawFinal run (nil: none).
// The renderer keeps the pointer and reads the chain on every frame, so
// changes to it apply from the next Draw.
func (r *Renderer) SetPost(p *PostChain) { r.post = p }

// SetTime sets the value of the Time uniform (see [ShaderUniformTime]) for
// the following Draw and DrawFinal calls: t in seconds, as a float32. It
// starts at 0 and only changes through SetTime; game.Engine sets it every
// frame to the interpolated simulation time. It allocates nothing.
//
// A float32 has about 7 significant digits, so after a few hours Time
// advances in steps of a millisecond or more. Shaders that animate with it
// should wrap it themselves (mod(Time, period)) rather than rely on long
// absolute values.
func (r *Renderer) SetTime(t time.Duration) { r.time[0] = float32(t.Seconds()) }

// Stats returns the statistics of the last Draw.
func (r *Renderer) Stats() Stats { return r.stats }

// Draw renders q onto dst through cam. alpha in [0,1] blends PrevPos to Pos
// of every item and of the camera, so that a 60 TPS simulation looks smooth
// at any frame rate. q should have been sorted. Draw never reads the World
// and allocates nothing for sprites (shaded or not) and shapes; text may
// allocate. With a post chain (see [Renderer.SetPost]) whose passes run,
// the frame, including what dst held before Draw, is drawn offscreen and
// the passes write dst; without one Draw draws straight onto dst.
// PostChain.Fade is drawn over the result.
func (r *Renderer) Draw(dst *ebiten.Image, q *Queue, cam *Camera, alpha float64) {
	b := dst.Bounds()
	r.screen = image.Pt(b.Dx(), b.Dy())
	a := float32(min(max(alpha, 0), 1))
	// Round once per frame, after blending (see Camera.PixelSnap); the copy
	// has PixelSnap cleared, so per-item WorldToScreen calls don't round
	// again.
	view := cam.At(a).pixelSnapped(r.screen)
	r.stats = Stats{Items: q.Len(), Culled: q.Culled()}
	r.placeholder = nil
	if r.stores.Sprites.s != nil {
		r.placeholder = r.stores.Sprites.Get(gfx.SpriteHandle{})
	}
	r.next, r.active = 0, -1
	r.frame++
	screen := dst
	if r.postOn() {
		dst = r.beginPost(screen)
	}

	for i := range q.Len() {
		it := q.At(i)
		t := transform{pos: it.PrevPos.Lerp(it.Pos, a), rot: it.Rot, scale: it.Scale}
		if it.Space == WorldSpace {
			t = t.toScreen(&view, r.screen)
		}
		target := r.targetFor(dst, it.Layer)
		switch it.Kind {
		case KindSprite:
			r.drawSprite(target, it, t)
		case KindRect:
			r.drawRect(target, it, t)
		case KindLine:
			r.drawLine(target, it, t)
		case KindCircle:
			r.drawCircle(target, it, t)
		case KindText:
			r.drawText(target, it, t)
		case KindCanvas:
			r.drawCanvas(target, it, t)
		case KindNineSlice:
			r.drawNineSlice(target, it, t)
		}
	}
	r.flush()
	r.finishGroups(dst)
	if dst != screen {
		r.runPost(screen, dst)
	}
	if r.post.fadeOn() {
		r.drawFade(screen)
	}
}

// transform is an item's placement on screen, in logical pixels.
type transform struct {
	pos   gfx.Vec2
	rot   float32
	scale gfx.Vec2
}

// toScreen applies the camera: position, zoom and rotation.
func (t transform) toScreen(c *Camera, screen image.Point) transform {
	z := c.zoom()
	return transform{
		pos:   c.WorldToScreen(t.pos, screen),
		rot:   t.rot - c.Rotation,
		scale: gfx.Vec2{X: t.scale.X * z, Y: t.scale.Y * z},
	}
}

// geoM builds the matrix that draws an image whose top-left corner is shifted
// by shift pixels (before scale) relative to the pivot, which is placed at t,
// mirrored by flipX/flipY.
func geoM(g *ebiten.GeoM, shift gfx.Vec2, flipX, flipY bool, t transform) {
	g.Reset()
	g.Translate(float64(shift.X), float64(shift.Y))
	sx, sy := float64(t.scale.X), float64(t.scale.Y)
	if flipX {
		sx = -sx
	}
	if flipY {
		sy = -sy
	}
	g.Scale(sx, sy)
	if t.rot != 0 {
		g.Rotate(float64(t.rot))
	}
	g.Translate(float64(t.pos.X), float64(t.pos.Y))
}

func (r *Renderer) setStyle(c gfx.Color, blend Blend) {
	cr, cg, cb, ca := c.Floats()
	// ColorScale works on premultiplied colors.
	r.opts.ColorScale.Set(cr*ca, cg*ca, cb*ca, ca)
	r.topts.ColorScale = r.opts.ColorScale
	r.opts.Blend = ebitenBlend(blend)
	r.blend = blend
}

// blendMultiply is dst * src + dst * (1 - src alpha) on premultiplied
// colors: an opaque source multiplies what is below, a transparent one
// leaves it unchanged.
var blendMultiply = ebiten.Blend{
	BlendFactorSourceRGB:        ebiten.BlendFactorDestinationColor,
	BlendFactorSourceAlpha:      ebiten.BlendFactorDestinationColor,
	BlendFactorDestinationRGB:   ebiten.BlendFactorOneMinusSourceAlpha,
	BlendFactorDestinationAlpha: ebiten.BlendFactorOneMinusSourceAlpha,
	BlendOperationRGB:           ebiten.BlendOperationAdd,
	BlendOperationAlpha:         ebiten.BlendOperationAdd,
}

// blendScreen is src + dst * (1 - src) per color channel on premultiplied
// colors; alpha composites like normal blending.
var blendScreen = ebiten.Blend{
	BlendFactorSourceRGB:        ebiten.BlendFactorOne,
	BlendFactorSourceAlpha:      ebiten.BlendFactorOne,
	BlendFactorDestinationRGB:   ebiten.BlendFactorOneMinusSourceColor,
	BlendFactorDestinationAlpha: ebiten.BlendFactorOneMinusSourceAlpha,
	BlendOperationRGB:           ebiten.BlendOperationAdd,
	BlendOperationAlpha:         ebiten.BlendOperationAdd,
}

// blendSubtract is dst - src on color and leaves alpha unchanged.
var blendSubtract = ebiten.Blend{
	BlendFactorSourceRGB:        ebiten.BlendFactorOne,
	BlendFactorSourceAlpha:      ebiten.BlendFactorZero,
	BlendFactorDestinationRGB:   ebiten.BlendFactorOne,
	BlendFactorDestinationAlpha: ebiten.BlendFactorOne,
	BlendOperationRGB:           ebiten.BlendOperationReverseSubtract,
	BlendOperationAlpha:         ebiten.BlendOperationAdd,
}

// blendLighten is max(src, dst) on every channel. The max operation ignores
// blend factors; a transparent source (all zero, premultiplied) leaves the
// destination unchanged.
var blendLighten = ebiten.Blend{
	BlendFactorSourceRGB:        ebiten.BlendFactorOne,
	BlendFactorSourceAlpha:      ebiten.BlendFactorOne,
	BlendFactorDestinationRGB:   ebiten.BlendFactorOne,
	BlendFactorDestinationAlpha: ebiten.BlendFactorOne,
	BlendOperationRGB:           ebiten.BlendOperationMax,
	BlendOperationAlpha:         ebiten.BlendOperationMax,
}

// ebitenBlend maps b to Ebiten's blend state. An unknown value draws as
// BlendNormal.
func ebitenBlend(b Blend) ebiten.Blend {
	switch b {
	case BlendAdditive:
		return ebiten.BlendLighter
	case BlendMultiply:
		return blendMultiply
	case BlendScreen:
		return blendScreen
	case BlendSubtract:
		return blendSubtract
	case BlendLighten:
		return blendLighten
	case BlendErase:
		return ebiten.BlendDestinationOut
	default:
		return ebiten.BlendSourceOver
	}
}

// spriteShift returns where the sprite's image starts relative to its pivot,
// in pixels before scale. The pivot is a fraction of the untrimmed frame
// size, and a trimmed image sits at Offset inside that frame, so trimmed
// frames keep their pivot. A zero origin means the sprite's default pivot.
func spriteShift(sp *Sprite, origin gfx.Vec2) gfx.Vec2 {
	if origin == (gfx.Vec2{}) {
		origin = sp.Origin
	}
	return gfx.Vec2{X: sp.Offset.X - origin.X*sp.Size.X, Y: sp.Offset.Y - origin.Y*sp.Size.Y}
}

func (r *Renderer) drawSprite(dst *ebiten.Image, it *Item, t transform) {
	// Get never returns nil: zero, stale, loading and failed handles give the
	// placeholder sprite.
	sp := r.stores.Sprites.Get(it.Sprite)
	if sp == r.placeholder {
		r.stats.Placeholders++
	}
	geoM(&r.opts.GeoM, spriteShift(sp, it.Origin), it.FlipX, it.FlipY, t)
	r.setStyle(it.Color, it.Blend)
	if sh := r.shader(it.Shader); sh != nil {
		r.drawShaded(dst, sp.Image, sh, it)
		return
	}
	r.drawQuad(dst, sp.source(), sp.Image)
}

// drawCanvas draws a pre-rendered canvas like a sprite: through Item.Shader
// when it resolves, otherwise batched. Canvases are separate images, so each
// one is its own batch.
func (r *Renderer) drawCanvas(dst *ebiten.Image, it *Item, t transform) {
	img := r.canvases.image(it.Canvas)
	if img == nil {
		return
	}
	b := img.Bounds()
	shift := gfx.Vec2{X: -it.Origin.X * float32(b.Dx()), Y: -it.Origin.Y * float32(b.Dy())}
	geoM(&r.opts.GeoM, shift, it.FlipX, it.FlipY, t)
	r.setStyle(it.Color, it.Blend)
	if sh := r.shader(it.Shader); sh != nil {
		r.drawShaded(dst, img, sh, it)
		return
	}
	r.drawQuad(dst, img, img)
}

// drawQuad draws the pixels of img (src itself or a sub-image of it) with
// the placement and style in r.opts: into the pending batch, or with
// DrawImage when batching is off.
func (r *Renderer) drawQuad(dst, src, img *ebiten.Image) {
	if r.noBatch {
		dst.DrawImage(img, &r.opts)
		r.stats.DrawCalls++
		return
	}
	r.addQuad(dst, src, img.Bounds(), &r.opts.GeoM, &r.opts.ColorScale, r.blend)
}

// shader returns the loaded shader of h, or nil for "draw normally": the
// zero handle (checked first, so unshaded sprites skip the store lookup), a
// shader that is loading or failed, or a Renderer without a shader store.
func (r *Renderer) shader(h gfx.ShaderHandle) *ebiten.Shader {
	if h.IsZero() || r.stores.Shaders.s == nil {
		return nil
	}
	return r.stores.Shaders.Get(h).Shader
}

// aux returns the image of a shaded item's second image h, or nil: the
// zero handle (checked first, so most items skip the store lookup), a
// sprite that is loading or failed, or a Renderer without a sprite store.
// It is never the placeholder sprite, so a shader that reads imageSrc1
// sees transparent pixels until the image is ready instead of the
// placeholder's checkerboard.
func (r *Renderer) aux(h gfx.SpriteHandle) *ebiten.Image {
	if h.IsZero() || r.stores.Sprites.s == nil {
		return nil
	}
	sp := r.stores.Sprites.Get(h)
	if sp == r.placeholder {
		return nil
	}
	return sp.Image
}

// drawShaded draws img through sh with the placement and style that
// drawSprite or drawCanvas already put into r.opts: into the pending batch,
// or as its own draw call when batching is off. The quad has img's size, so
// the same GeoM places it exactly like DrawImage.
func (r *Renderer) drawShaded(dst, img *ebiten.Image, sh *ebiten.Shader, it *Item) {
	r.addShadedQuad(dst, img, r.aux(it.Aux), &r.opts.GeoM, &r.opts.ColorScale, r.blend, sh, &it.Params)
	if r.noBatch {
		r.flush()
	}
}

// quad draws the white pixel stretched to w x h pixels (before t's scale),
// with its top-left corner at t.pos rotated by t.rot.
func (r *Renderer) quad(dst *ebiten.Image, w, h float32, t transform) {
	geoM(&r.opts.GeoM, gfx.Vec2{}, false, false, transform{pos: t.pos, rot: t.rot, scale: gfx.Vec2{X: w, Y: h}})
	r.drawQuad(dst, r.white, r.white)
}

// drawRect draws a 1-pixel outline of an it.Size rectangle whose top-left
// is at t, scaled and rotated with it. Through a shader the outline is
// drawn at scale 1 first, so its width scales with the item.
func (r *Renderer) drawRect(dst *ebiten.Image, it *Item, t transform) {
	if sh := r.shader(it.Shader); sh != nil {
		if region, at, ok := r.beginScratch(gfx.Vec2{}, it.Size); ok {
			r.rectOutline(region, it, at)
			r.endScratch(dst, region, sh, it, t, gfx.Vec2{})
			return
		}
	}
	r.setStyle(it.Color, it.Blend)
	r.rectOutline(dst, it, t)
}

// rectOutline draws the four edges of drawRect in the current style.
func (r *Renderer) rectOutline(dst *ebiten.Image, it *Item, t transform) {
	w, h := it.Size.X*t.scale.X, it.Size.Y*t.scale.Y
	sin, cos := xmath.Sincos(t.rot)
	edge := func(ox, oy, ew, eh float32) {
		p := gfx.Vec2{X: t.pos.X + ox*cos - oy*sin, Y: t.pos.Y + ox*sin + oy*cos}
		r.quad(dst, ew, eh, transform{pos: p, rot: t.rot})
	}
	edge(0, 0, w, 1)
	edge(0, h-1, w, 1)
	edge(0, 0, 1, h)
	edge(w-1, 0, 1, h)
}

// drawLine draws a 1-pixel line from t to t + it.Size (scaled, rotated).
func (r *Renderer) drawLine(dst *ebiten.Image, it *Item, t transform) {
	if sh := r.shader(it.Shader); sh != nil {
		// The line may run in any direction from the pivot, and its one
		// pixel of width lies beside it: one more pixel on every side.
		lo := gfx.Vec2{X: min(it.Size.X, 0) - 1, Y: min(it.Size.Y, 0) - 1}
		size := gfx.Vec2{X: xmath.Abs(it.Size.X) + 2, Y: xmath.Abs(it.Size.Y) + 2}
		if region, at, ok := r.beginScratch(lo, size); ok {
			r.segment(region, at.pos, it.Size)
			r.endScratch(dst, region, sh, it, t, lo)
			return
		}
	}
	r.setStyle(it.Color, it.Blend)
	r.segment(dst, t.pos, gfx.Vec2{X: it.Size.X * t.scale.X, Y: it.Size.Y * t.scale.Y}.Rotate(t.rot))
}

// segment draws a 1-pixel line from p to p+d.
func (r *Renderer) segment(dst *ebiten.Image, p, d gfx.Vec2) {
	length := float32(math.Hypot(float64(d.X), float64(d.Y)))
	if length == 0 {
		return
	}
	angle := float32(math.Atan2(float64(d.Y), float64(d.X)))
	r.quad(dst, length, 1, transform{pos: p, rot: angle})
}

// drawCircle draws a 1-pixel circle outline of radius it.Size.X around t.
func (r *Renderer) drawCircle(dst *ebiten.Image, it *Item, t transform) {
	if sh := r.shader(it.Shader); sh != nil {
		// The pivot is the center; one more pixel for the outline's width.
		reach := it.Size.X + 1
		lo := gfx.Vec2{X: -reach, Y: -reach}
		if region, at, ok := r.beginScratch(lo, gfx.Vec2{X: 2 * reach, Y: 2 * reach}); ok {
			r.circleOutline(region, it, at)
			r.endScratch(dst, region, sh, it, t, lo)
			return
		}
	}
	r.setStyle(it.Color, it.Blend)
	r.circleOutline(dst, it, t)
}

// circleOutline draws the segments of drawCircle in the current style.
func (r *Renderer) circleOutline(dst *ebiten.Image, it *Item, t transform) {
	radius := it.Size.X * t.scale.X
	prev := gfx.Vec2{X: t.pos.X + radius, Y: t.pos.Y}
	for i := 1; i <= circleSegments; i++ {
		sin, cos := xmath.Sincos(2 * math.Pi * float32(i) / circleSegments)
		next := gfx.Vec2{X: t.pos.X + radius*cos, Y: t.pos.Y + radius*sin}
		r.segment(dst, prev, next.Sub(prev))
		prev = next
	}
}

// drawText draws it.Text with its top-left corner at t. Through a shader
// the text is drawn at scale 1 first and the shader sees all of it.
func (r *Renderer) drawText(dst *ebiten.Image, it *Item, t transform) {
	font := r.stores.Fonts.Get(it.Font)
	// Without a line spacing, Ebiten draws every line of a multi-line text
	// on the same baseline.
	r.topts.LineSpacing = font.LineHeight
	if sh := r.shader(it.Shader); sh != nil {
		w, h := text.Measure(it.Text, font.Face, font.LineHeight)
		if region, at, ok := r.beginScratch(gfx.Vec2{}, gfx.Vec2{X: float32(w), Y: float32(h)}); ok {
			r.textAt(region, it, font.Face, at)
			r.endScratch(dst, region, sh, it, t, gfx.Vec2{})
			return
		}
	}
	r.setStyle(it.Color, it.Blend)
	r.textAt(dst, it, font.Face, t)
}

// textAt draws it.Text at t in the current style.
func (r *Renderer) textAt(dst *ebiten.Image, it *Item, face text.Face, t transform) {
	geoM(&r.topts.GeoM, gfx.Vec2{}, false, false, t)
	r.flush() // text is not batched; keep draw order
	text.Draw(dst, it.Text, face, &r.topts)
	r.stats.DrawCalls++
}
