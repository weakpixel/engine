package anim

import (
	"time"

	"github.com/weakpixel/engine/gfx"
	"github.com/weakpixel/engine/render"
)

// Frame is one step of a [Clip].
type Frame struct {
	// Sprite is the image shown during the frame.
	Sprite gfx.SpriteHandle
	// Duration is how long the frame is shown. It is positive in clips that
	// come from a loader.
	Duration time.Duration
	// Events is an optional bitmask for gameplay (footstep, hitbox on, ...).
	// The Aseprite loader leaves it zero.
	Events uint32
	// Origin is the pivot written to Sprite.Origin while the frame shows, as
	// a fraction of the sprite's untrimmed size. It is computed per frame from
	// the pivot of an *.anim.json file so that the pivot stays put even when
	// trimming differs between frames. Zero means the clip has no pivot and
	// leaves Sprite.Origin alone.
	Origin gfx.Vec2
	// Boxes are the hit and hurt boxes active during the frame, relative to
	// the pivot. It is a sub-slice of one array per clip; do not modify or
	// append to it. Nil for clips without an *.anim.json file.
	Boxes []gfx.Box
}

// LoopMode says what a [Clip] does when it reaches its last frame.
type LoopMode uint8

// Loop modes.
const (
	// Loop restarts from the first frame.
	Loop LoopMode = iota
	// Once stops on the last frame.
	Once
	// PingPong plays forward, then backward, without repeating the end
	// frames, and so on.
	PingPong
)

// Clip is a sequence of frames. Load one with [LoadClip] and a
// "#tag" key ("chars/hero.aseprite.json#run"). A Clip is immutable once
// loaded.
type Clip struct {
	// Frames are the steps in play order (a reversed Aseprite tag is already
	// reversed here).
	Frames []Frame
	// Loop is the end-of-clip behavior.
	Loop LoopMode
	// Total is the sum of the frame durations: one forward pass.
	Total time.Duration
	// Body is the union of the "body" rects of the clip's *.anim.json
	// definition, relative to the pivot: the fixed body, not the per-frame
	// hurt boxes. animbody.System copies it into a collision.ShapeAnimBody
	// collider. Zero without a definition or body.
	Body gfx.IRect
}

// ClipSet is the set of clips of one Aseprite file, one per tag. Load it with
// [LoadClipSet] and the file's path: either the Aseprite export itself
// or an *.anim.json animation definition that refers to it and adds loop and
// timing overrides, a pivot and hit/hurt boxes. Each clip is also loadable on
// its own with a "#tag" key, which loads the set too; a held clip keeps the
// set (and so the atlas and texture) alive.
type ClipSet struct {
	// Atlas is the sheet the clips' sprites come from.
	Atlas render.AtlasHandle
	// Clips maps tag name to clip. Read it at load or
	// spawn time, not per frame.
	Clips map[string]gfx.ClipHandle
	// Names lists the tag names in file order.
	Names []string
	// Default is the clip to start with, from the "defaultClip" of an
	// *.anim.json file; empty for a plain Aseprite export. It is informational:
	// spawn code picks Clips[Default].
	Default string
}

// ShapesAt returns the boxes of frame (an index into Frames, as in
// Animator.Frame), or nil if frame is out of range or has none. The slice
// points into the clip: do not modify it. It does not allocate.
func (c *Clip) ShapesAt(frame int) []gfx.Box {
	if frame < 0 || frame >= len(c.Frames) {
		return nil
	}
	return c.Frames[frame].Boxes
}

// FrameAt returns the index of the frame shown t into the clip and whether a
// non-looping clip has finished. t is the playback time already scaled by
// speed; a negative t counts as zero. It does not allocate.
//
// An empty clip returns (0, true).
func (c *Clip) FrameAt(t time.Duration) (frame int, done bool) {
	n := len(c.Frames)
	if n == 0 || c.Total <= 0 {
		return 0, true
	}
	t = max(t, 0)
	switch c.Loop {
	case Once:
		if t >= c.Total {
			return n - 1, true
		}
		return c.scan(t, 0, n, 1), false
	case PingPong:
		if n == 1 {
			return 0, false
		}
		// One period is the forward pass plus the way back through the
		// inner frames: the end frames are not shown twice in a row.
		period := 2*c.Total - c.Frames[0].Duration - c.Frames[n-1].Duration
		t %= period
		if t < c.Total {
			return c.scan(t, 0, n, 1), false
		}
		return c.scan(t-c.Total, n-2, 0, -1), false
	default:
		return c.scan(t%c.Total, 0, n, 1), false
	}
}

// scan walks frames from `from` in steps of `step` until `end` (exclusive)
// and returns the frame that t falls in. A linear scan beats a prefix-sum
// table for the short clips animation uses and needs no extra storage, so
// the Clip stays a plain value. t is always inside the walked range; if
// rounding ever pushes it past, the last walked frame is returned.
func (c *Clip) scan(t time.Duration, from, end, step int) int {
	last := from
	for i := from; i != end; i += step {
		d := c.Frames[i].Duration
		if t < d {
			return i
		}
		t -= d
		last = i
	}
	return last
}
