package render

import (
	"fmt"
	"image"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/weakpixel/engine/gfx"
)

// TargetID names an image a [Pass] reads or writes: one of the special
// values [TargetChain], [TargetFrame] and [TargetPrevious], or a target
// declared with [PostChain.AddTarget]. The zero value is [TargetChain].
type TargetID int

const (
	// TargetChain is the chain so far. As a pass's Src it is the output of
	// the pass before (the frame for the first pass); as its Out it is the
	// next chain image, or the screen for the last pass that writes it. The
	// zero value of Src and Out, so a pass that sets neither works as it
	// always did. As an Extra entry zero means "unused".
	TargetChain TargetID = 0
	// TargetFrame is the frame Draw produced, as the first pass sees it. It
	// is an input only.
	TargetFrame TargetID = -1
	// TargetPrevious is what the chain showed on the previous frame (at
	// the first frame, and after a resize, the current frame). It is an
	// input only. A chain with a running pass that reads it costs two more
	// screen-sized images and one more copy per frame.
	TargetPrevious TargetID = -2
)

// firstGroupTarget is the TargetID of group 0; group g is firstGroupTarget-g.
const firstGroupTarget TargetID = -3

// TargetGroup is the target that names the image of layer group id (see
// [GroupConfig]): the group's items drawn over its Clear color, before its
// shader and blend, as the logical screen size. It is an input only (Src or
// an Extra entry); a pass cannot write a group. A group no item fell into
// reads as its Clear color, never as last frame's pixels. A group that does
// not exist makes Draw panic with "unknown post target".
func TargetGroup(id GroupID) TargetID { return firstGroupTarget - TargetID(id) }

// groupOf decodes a [TargetGroup] target.
func groupOf(id TargetID) (GroupID, bool) {
	if id > firstGroupTarget {
		return 0, false
	}
	return GroupID(firstGroupTarget - id), true
}

// Target is an offscreen image a [PostChain] pass can write and later passes
// can read, declared with [PostChain.AddTarget].
type Target struct {
	// Scale divides the screen size: 1 (and 0) is the screen size, 2 half,
	// 4 a quarter. A size that does not divide evenly is rounded up. The
	// image follows screen resizes. It is allocated the first frame a
	// running pass uses it, and starts transparent.
	Scale int
}

// Pass is one full-screen shader pass of a [PostChain].
type Pass struct {
	// Shader runs once per pixel of Out. It gets Src as imageSrc0, the
	// Extra images as imageSrc1 to imageSrc3 and Params as its "Params" vec4
	// uniform. The zero handle, and a shader that is loading or failed, skip
	// the pass.
	//
	// The shader's src is in Src's pixels: the pass draws one quad over Out,
	// with source coordinates spanning all of Src, so a pass that writes a
	// smaller image than it reads runs once per small pixel and sees src at
	// the center of the block of Src pixels it covers. To read an input of
	// another size, map src by the sizes and add imageSrc0Origin(), because
	// Kage takes the position of imageSrcNAt in imageSrc0's texture:
	// imageSrc1At(imageSrc0Origin() +
	// (src-imageSrc0Origin())/imageSrc0Size()*imageSrc1Size()).
	Shader gfx.ShaderHandle
	// Params are passed to Shader as its "Params" vec4 uniform.
	Params [4]float32
	// Off skips the pass (a settings toggle) without removing it.
	Off bool

	// Src is the image the shader reads as imageSrc0: the chain so far
	// (zero), TargetFrame, TargetPrevious or a declared target.
	Src TargetID
	// Extra are the images read as imageSrc1 to imageSrc3; zero is unused.
	// Any input may have any size.
	Extra [3]TargetID
	// Out is the image the pass writes, replacing its pixels: the chain
	// (zero) or a declared target. A pass that writes a target leaves the
	// chain as it was. If no running pass writes the chain, the frame is
	// shown as it is. Out may not also be read by the pass.
	//
	// A skipped pass (Off, shader not loaded) writes nothing: its Out keeps
	// what it held last frame, or is transparent if it was never written.
	Out TargetID

	// builtin is a shader of the engine that stands in for Shader (see
	// builtinShader); the zero value means Shader is used.
	builtin builtinShader
}

// on reports whether the pass runs with shader sh (nil: not loaded).
func (p *Pass) on(sh *ebiten.Shader) bool { return !p.Off && sh != nil }

// passShader returns the compiled shader of p: the engine's built-in one if
// it has one, else the loaded asset (nil: not loaded).
func (r *Renderer) passShader(p *Pass) *ebiten.Shader {
	if p.builtin != 0 {
		return r.builtinShader(p.builtin)
	}
	return r.shader(p.Shader)
}

// PostChain is the post-processing of a Renderer (see [Renderer.SetPost]):
// full-screen shader passes over the finished frame, at logical resolution,
// and an optional final pass at window resolution. The zero value does
// nothing and is ready to use.
//
// Game code owns the chain: set Passes up once and change a pass's Params or
// Off between frames (from a system: it is a World resource in game.Engine).
// The renderer reads it in every Draw and never copies or keeps it.
type PostChain struct {
	// Passes run in order on the frame Draw produced. Each one reads the
	// output of the previous one; the last one writes the screen. With no
	// pass that runs (none, all Off, or shaders not loaded), Draw draws
	// straight to the screen and allocates nothing.
	Passes []Pass
	// Targets are the offscreen images the passes can write and read; the
	// target at index i has the ID i+1. Declare them with [PostChain.AddTarget].
	Targets []Target
	// Final, when set, is drawn by [Renderer.DrawFinal] at window (device)
	// resolution: the shader gets the logical frame as imageSrc0 and runs
	// once per window pixel, for effects that need real pixels (CRT
	// scanlines and masks, smooth upscaling). nil, Off or a shader that is
	// not loaded draw the frame with Ebiten's default scaling.
	Final *Pass
	// Fade covers the frame with a color after Passes and before Final: a
	// screen fade or flash without a shader. Its alpha says how much (255:
	// fully covered). Unlike a Sprite tint, a zero alpha byte means no fade,
	// so the zero value does nothing; the fade costs one draw while it is on.
	Fade gfx.Color
}

// AddTarget declares an offscreen image for passes to write and read and
// returns its ID. Add targets before the first frame that uses them; do not
// remove or reorder them afterwards, as passes refer to them by ID.
func (p *PostChain) AddTarget(t Target) TargetID {
	p.Targets = append(p.Targets, t)
	return TargetID(len(p.Targets))
}

// fadeOn reports whether the chain's Fade covers any of the frame.
func (p *PostChain) fadeOn() bool { return p != nil && p.Fade&0xff != 0 }

// drawFade fills dst's frame-sized area with PostChain.Fade (see fadeOn),
// as a stretched copy of the white pixel, which allocates nothing.
func (r *Renderer) drawFade(dst *ebiten.Image) {
	o := &r.fadeopts
	o.GeoM.Reset()
	o.GeoM.Scale(float64(r.screen.X), float64(r.screen.Y))
	o.ColorScale.Reset()
	cr, cg, cb, ca := r.post.Fade.Floats()
	// ColorScale is premultiplied.
	o.ColorScale.Scale(cr*ca, cg*ca, cb*ca, ca)
	r.stats.DrawCalls++
	dst.DrawImage(r.white, o)
}

// postOn reports whether any pass of the chain runs this frame.
func (r *Renderer) postOn() bool {
	if r.post == nil {
		return false
	}
	for i := range r.post.Passes {
		p := &r.post.Passes[i]
		if p.on(r.passShader(p)) {
			return true
		}
	}
	return false
}

// beginPost returns the image the frame is drawn into instead of dst: a
// copy of dst, so what the caller drew before Draw (the background fill)
// is part of the frame the passes see.
func (r *Renderer) beginPost(dst *ebiten.Image) *ebiten.Image {
	scene := screenImage(&r.postImgs[0], r.screen)
	r.popts.Blend = ebiten.BlendCopy
	scene.DrawImage(dst, &r.popts)
	return scene
}

// postPlan is what runPost learns about the running passes before drawing.
type postPlan struct {
	last      int  // the last running pass that writes the chain, -1 if none
	keepFrame bool // a pass reads TargetFrame after the chain was written
	usePrev   bool // a pass reads TargetPrevious
}

// planPost validates the running passes of the chain and finds the facts
// runPost needs. It panics on a pass that refers to an unknown target, reads
// and writes the same target, or writes TargetFrame or TargetPrevious.
func (r *Renderer) planPost() postPlan {
	pl := postPlan{last: -1}
	n, ng := len(r.post.Targets), r.groups.Len()
	writes := 0
	for i := range r.post.Passes {
		p := &r.post.Passes[i]
		if !p.on(r.passShader(p)) {
			continue
		}
		checkTarget(i, p.Src, n, ng)
		if p.Out == TargetFrame || p.Out == TargetPrevious {
			panic(fmt.Sprintf("render: post pass %d writes %s", i, targetName(p.Out)))
		}
		checkTarget(i, p.Out, n, ng)
		if _, ok := groupOf(p.Out); ok {
			panic(fmt.Sprintf("render: post pass %d writes %s", i, targetName(p.Out)))
		}
		reads := func(id TargetID) {
			if id > 0 && id == p.Out {
				panic(fmt.Sprintf("render: post pass %d reads and writes target %d", i, id))
			}
			switch id {
			case TargetFrame:
				pl.keepFrame = pl.keepFrame || writes > 0
			case TargetPrevious:
				pl.usePrev = true
			}
		}
		reads(p.Src)
		for _, id := range p.Extra {
			if id != 0 {
				checkTarget(i, id, n, ng)
				reads(id)
			}
		}
		if p.Out == TargetChain {
			pl.last = i
			writes++
		}
	}
	return pl
}

// checkTarget panics if id is not a special target, one of the n declared
// or one of the ng groups.
func checkTarget(pass int, id TargetID, n, ng int) {
	if g, ok := groupOf(id); ok {
		if int(g) < ng {
			return
		}
	} else if id >= TargetPrevious && int(id) <= n {
		return
	}
	panic(fmt.Sprintf("render: unknown post target %d in pass %d", id, pass))
}

func targetName(id TargetID) string {
	if g, ok := groupOf(id); ok {
		return fmt.Sprintf("group %d", g)
	}
	if id == TargetFrame {
		return "TargetFrame"
	}
	return "TargetPrevious"
}

// postTarget returns the image of declared target id, allocated or resized
// to Scale on first use.
func (r *Renderer) postTarget(id TargetID) *ebiten.Image {
	i := int(id) - 1
	s := max(r.post.Targets[i].Scale, 1)
	size := image.Pt((r.screen.X+s-1)/s, (r.screen.Y+s-1)/s)
	return screenImage(&r.ptargets[i], size)
}

// syncTargets makes r.ptargets as long as the chain's Targets, freeing the
// images of removed ones. It allocates only when the chain gained a target.
func (r *Renderer) syncTargets() {
	n := len(r.post.Targets)
	for _, img := range r.ptargets[min(n, len(r.ptargets)):] {
		if img != nil {
			img.Deallocate()
		}
	}
	if n <= cap(r.ptargets) {
		r.ptargets = r.ptargets[:n]
		return
	}
	r.ptargets = append(r.ptargets[:cap(r.ptargets)], make([]*ebiten.Image, n-cap(r.ptargets))...)
}

// postInput returns the image id names for a pass: chain is the chain so
// far, scene the frame and prev the previous frame.
func (r *Renderer) postInput(id TargetID, chain, scene, prev *ebiten.Image) *ebiten.Image {
	switch id {
	case TargetChain:
		return chain
	case TargetFrame:
		return scene
	case TargetPrevious:
		return prev
	}
	if g, ok := groupOf(id); ok {
		// A group nobody drew into this frame still holds last frame's
		// pixels (or none yet): begin it so the pass sees its Clear color.
		if int(g) >= len(r.begun) || r.begun[g] != r.frame {
			r.begin(g)
		}
		return r.targets[g]
	}
	return r.postTarget(id)
}

// runPost runs the passes of the chain on scene (r.postImgs[0]). Passes that
// write the chain alternate between two chain images (r.postImgs[1] and the
// frame image, or a third one when a later pass still reads the frame); the
// last one writes dst. With a pass that reads TargetPrevious that last pass
// writes the history image instead, which is then copied to dst.
func (r *Renderer) runPost(dst, scene *ebiten.Image) {
	pl := r.planPost()
	r.syncTargets()

	var prev, final *ebiten.Image
	final = dst
	if pl.usePrev {
		prev, final = r.beginHistory(scene)
	} else {
		r.dropHistory()
	}
	pingpong := [2]*ebiten.Image{}
	n := 0 // chain images used so far
	chain := scene
	ins := &r.psopts.Images
	for i := range r.post.Passes {
		p := &r.post.Passes[i]
		sh := r.passShader(p)
		if !p.on(sh) {
			continue
		}
		var out *ebiten.Image
		switch {
		case p.Out != TargetChain:
			out = r.postTarget(p.Out)
		case i == pl.last:
			out = final
		default:
			// The chain images are allocated only for chains that need
			// them.
			k := n % 2
			n++
			switch {
			case k == 0:
				pingpong[0] = screenImage(&r.postImgs[1], r.screen)
			case pl.keepFrame:
				pingpong[1] = screenImage(&r.postAlt, r.screen)
			default:
				pingpong[1] = scene
			}
			out = pingpong[k]
		}
		ins[0] = r.postInput(p.Src, chain, scene, prev)
		for k, id := range p.Extra {
			ins[k+1] = nil
			if id != 0 {
				ins[k+1] = r.postInput(id, chain, scene, prev)
			}
		}
		r.shade(out, sh, p)
		if p.Out == TargetChain {
			chain = out
		}
	}
	if pl.last < 0 {
		// No pass writes the chain: the frame still has to reach the screen.
		r.copyImage(final, scene)
	}
	if pl.usePrev {
		r.copyImage(dst, final)
		r.histIdx = 1 - r.histIdx
	}
}

// beginHistory allocates the two history images of a chain that reads
// TargetPrevious, seeds the previous one with the frame when it is new, and
// returns the previous frame and the image the chain's last pass writes.
func (r *Renderer) beginHistory(scene *ebiten.Image) (prev, next *ebiten.Image) {
	fresh := !r.histOK
	for k := range r.hist {
		if r.hist[k] == nil || r.hist[k].Bounds().Size() != r.screen {
			fresh = true
		}
		screenImage(&r.hist[k], r.screen)
	}
	prev, next = r.hist[r.histIdx], r.hist[1-r.histIdx]
	if fresh {
		r.copyImage(prev, scene)
		r.histOK = true
	}
	return prev, next
}

// dropHistory frees the history images once no running pass reads
// TargetPrevious, so the next use starts from the then-current frame.
func (r *Renderer) dropHistory() {
	if !r.histOK {
		return
	}
	for k := range r.hist {
		r.hist[k].Deallocate()
		r.hist[k] = nil
	}
	r.histOK = false
}

// copyImage replaces dst with src (same size).
func (r *Renderer) copyImage(dst, src *ebiten.Image) {
	r.popts.Blend = ebiten.BlendCopy
	r.stats.DrawCalls++
	dst.DrawImage(src, &r.popts)
}

// quadIndices are the two triangles of a shader quad (see drawShaderQuad).
var quadIndices = [6]uint32{0, 1, 2, 1, 2, 3}

// shaderDst is what drawShaderQuad draws onto: an *ebiten.Image or the
// ebiten.FinalScreen of DrawFinal.
type shaderDst interface {
	DrawTrianglesShader32(vertices []ebiten.Vertex, indices []uint32, shader *ebiten.Shader, options *ebiten.DrawTrianglesShaderOptions)
}

// drawShaderQuad draws one quad through sh with o's images, blend and
// uniforms: the dw x dh rectangle placed by g (nil: as is) shows src, a
// rectangle in imageSrc0's bounds coordinates, whatever the sizes
// (DrawTrianglesShader accepts inputs of different sizes in pixel units,
// DrawRectShader does not). The corners are snapped like DrawRectShader
// snaps them (see snapPixel), so with src of the quad's size it covers the
// same pixels a DrawRectShader call with g would. The color argument is
// white. It reuses r.pverts and allocates nothing.
func (r *Renderer) drawShaderQuad(dst shaderDst, sh *ebiten.Shader, o *ebiten.DrawTrianglesShaderOptions, g *ebiten.GeoM, dw, dh float32, src image.Rectangle) {
	// The same float32 arithmetic as Ebiten's QuadVerticesFromSrcAndMatrix
	// (and addQuad), so the corners match DrawRectShader's to the bit.
	a, b, c, d, tx, ty := float32(1), float32(0), float32(0), float32(1), float32(0), float32(0)
	if g != nil {
		a, b, c, d = float32(g.Element(0, 0)), float32(g.Element(0, 1)), float32(g.Element(1, 0)), float32(g.Element(1, 1))
		tx, ty = float32(g.Element(0, 2)), float32(g.Element(1, 2))
	}
	ax, by, cx, dy := a*dw, b*dh, c*dw, d*dh
	sx0, sy0, sx1, sy1 := float32(src.Min.X), float32(src.Min.Y), float32(src.Max.X), float32(src.Max.Y)
	v := &r.pverts
	white := ebiten.Vertex{ColorR: 1, ColorG: 1, ColorB: 1, ColorA: 1}
	for i, p := range [4][4]float32{
		{tx, ty, sx0, sy0},
		{ax + tx, cx + ty, sx1, sy0},
		{by + tx, dy + ty, sx0, sy1},
		{ax + by + tx, cx + dy + ty, sx1, sy1},
	} {
		v[i] = white
		v[i].DstX, v[i].DstY, v[i].SrcX, v[i].SrcY = snapPixel(p[0]), snapPixel(p[1]), p[2], p[3]
	}
	dst.DrawTrianglesShader32(v[:], quadIndices[:], sh, o)
}

// shade draws the images r.psopts.Images through sh over all of out,
// replacing what was there (BlendCopy: a pass decides the whole pixel, so the
// output image needs no clear). The quad spans out, and its source
// coordinates span all of imageSrc0, whatever the sizes.
func (r *Renderer) shade(out *ebiten.Image, sh *ebiten.Shader, p *Pass) {
	so := &r.psopts
	so.Blend = ebiten.BlendCopy
	// In place, never assign to the map: see newUniforms.
	copy(r.pparams, p.Params[:])
	r.stats.PostPasses++
	r.stats.ShaderDraws++
	r.stats.DrawCalls++
	d := out.Bounds().Size()
	r.drawShaderQuad(out, sh, so, nil, float32(d.X), float32(d.Y), so.Images[0].Bounds())
}

// DrawFinal draws the logical frame offscreen onto the window screen, at
// window resolution: through PostChain.Final if it is set, not Off and
// loaded, else with ebiten.DefaultDrawFinalScreen. Its arguments are those
// of ebiten.FinalScreenDrawer, whose DrawFinalScreen should call it;
// game.Engine does. geoM scales and centers the frame in the window and is
// applied to the shader's rectangle, so imageSrc0At(src) samples the frame
// in logical pixels while the shader runs per window pixel (dst).
//
// A final pass adds one to Stats.PostPasses of the frame.
func (r *Renderer) DrawFinal(screen ebiten.FinalScreen, offscreen *ebiten.Image, geoM ebiten.GeoM) {
	var p *Pass
	if r.post != nil {
		p = r.post.Final
	}
	var sh *ebiten.Shader
	if p != nil {
		sh = r.shader(p.Shader)
	}
	if p == nil || !p.on(sh) {
		ebiten.DefaultDrawFinalScreen(screen, offscreen, geoM)
		return
	}
	so := &r.fsopts
	so.Blend = ebiten.BlendCopy
	copy(r.fparams, p.Params[:])
	r.stats.PostPasses++
	r.stats.ShaderDraws++
	r.stats.DrawCalls++
	so.Images[0] = offscreen
	b := offscreen.Bounds()
	r.drawShaderQuad(screen, sh, so, &geoM, float32(b.Dx()), float32(b.Dy()), b)
	so.Images[0] = nil
}
