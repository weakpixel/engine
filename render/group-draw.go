package render

import (
	"image"
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
)

// This file is the Renderer's side of layer groups: switching the draw
// target while walking the sorted queue, and compositing group images.
//
// Draw visits items in layer order, so every group's items are contiguous.
// The Renderer walks groups.order (sorted by From) alongside the queue:
// next points at the first group not reached yet and active is the group
// being drawn into. Groups the queue skips over are composited empty if
// their Clear is visible (an ambient light map), so they apply even on a
// frame without items in their layers.

// targetFor returns the image items of layer draw into: dst, or the image
// of the group that contains layer. Layers arrive in ascending order.
func (r *Renderer) targetFor(dst *ebiten.Image, layer int16) *ebiten.Image {
	g := r.groups
	if g == nil || len(g.order) == 0 {
		return dst
	}
	if r.active >= 0 {
		if layer <= g.groups[r.active].opts.To {
			return r.targets[r.active]
		}
		r.composite(dst, r.active)
		r.active = -1
	}
	for r.next < len(g.order) {
		id := g.order[r.next]
		o := &g.groups[id].opts
		if layer < o.From {
			break
		}
		r.next++
		if layer <= o.To {
			r.begin(id)
			r.active = id
			return r.targets[id]
		}
		r.compositeEmpty(dst, id)
	}
	return dst
}

// finishGroups composites the group still being drawn into and the groups
// above the last item.
func (r *Renderer) finishGroups(dst *ebiten.Image) {
	g := r.groups
	if g == nil {
		return
	}
	if r.active >= 0 {
		r.composite(dst, r.active)
		r.active = -1
	}
	for ; r.next < len(g.order); r.next++ {
		r.compositeEmpty(dst, g.order[r.next])
	}
}

// compositeEmpty composites a group that no item fell into, if its Clear
// color shows at all.
func (r *Renderer) compositeEmpty(dst *ebiten.Image, id GroupID) {
	o := &r.groups.groups[id].opts
	if o.NoComposite || o.Clear.A == 0 {
		return
	}
	r.begin(id)
	r.composite(dst, id)
}

// begin prepares the image of group id for this frame: screen-sized and
// cleared to the group's Clear color.
func (r *Renderer) begin(id GroupID) {
	r.flush()
	if len(r.targets) < r.groups.Len() {
		r.targets = append(r.targets, make([]*ebiten.Image, r.groups.Len()-len(r.targets))...)
		r.begun = append(r.begun, make([]uint64, r.groups.Len()-len(r.begun))...)
	}
	r.begun[id] = r.frame
	img := screenImage(&r.targets[id], r.screen)
	e := &r.groups.groups[id]
	if e.opts.Clear == (color.RGBA{}) {
		img.Clear()
		return
	}
	// e.clear is the color.Color boxed in Add or Set: passing opts.Clear
	// here would box it again and allocate every frame.
	img.Fill(e.clear)
}

// composite draws the image of group id onto dst: through the group's
// shader if it is loaded, else unchanged, with the group's blend mode.
func (r *Renderer) composite(dst *ebiten.Image, id GroupID) {
	r.flush() // the group's last quads, or dst's quads below the group
	o := &r.groups.groups[id].opts
	if o.NoComposite {
		return
	}
	img := r.targets[id]
	r.stats.Groups++
	r.stats.DrawCalls++
	sh := r.shader(o.Shader)
	if sh == nil {
		r.gopts.Blend = ebitenBlend(o.Blend)
		dst.DrawImage(img, &r.gopts)
		return
	}
	so := &r.gsopts
	so.Blend = ebitenBlend(o.Blend)
	so.Images[0], so.Images[1] = img, nil
	if o.SampleBelow {
		so.Images[1] = r.copyBelow(dst)
	}
	// In place, never assign to the map: see newUniforms.
	copy(r.gparams, o.Params[:])
	r.stats.ShaderDraws++
	r.drawShaderQuad(dst, sh, so, nil, float32(r.screen.X), float32(r.screen.Y), img.Bounds())
}

// copyBelow copies dst into the SampleBelow image and returns it.
func (r *Renderer) copyBelow(dst *ebiten.Image) *ebiten.Image {
	below := screenImage(&r.below, r.screen)
	r.gopts.Blend = ebiten.BlendCopy
	below.DrawImage(dst, &r.gopts)
	return below
}

// screenImage returns *img, (re)allocated when it is missing or not of the
// given size. Allocation happens on the first frame and after a resize only.
func screenImage(img **ebiten.Image, size image.Point) *ebiten.Image {
	if *img != nil {
		if (*img).Bounds().Size() == size {
			return *img
		}
		(*img).Deallocate()
	}
	*img = ebiten.NewImage(size.X, size.Y)
	return *img
}
