package anim

import (
	"github.com/weakpixel/asset"
	"github.com/weakpixel/engine/gfx"
)

// ShapesOf returns the hit and hurt boxes of the frame an is showing
// (Animator.Frame of its clip), relative to the pivot. Place them in the world
// with gfx.Box.World and the entity's Transform and Sprite flips.
//
// It returns nil for a zero clip handle, a clip that is not Ready, a frame out
// of range, and clips without boxes. The slice points into the clip: do not
// modify it, and do not keep it past an unload of the clip. Cache the store
// ([ClipStoreOf]) once; ShapesOf itself does not allocate.
func ShapesOf(clips ClipStore, an *gfx.Animator) []gfx.Box {
	// Lookup, unlike Get, tells "not loaded" from "loaded" without handing out
	// the placeholder clip.
	clip, state := clips.Lookup(an.Clip)
	if state != asset.Ready {
		return nil
	}
	return clip.ShapesAt(int(an.Frame))
}
