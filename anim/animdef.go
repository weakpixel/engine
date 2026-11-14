package anim

import (
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/weakpixel/asset"
	"github.com/weakpixel/engine/anim/animfile"
	"github.com/weakpixel/engine/gfx"
)

// An animation definition ("hero.anim.json", animfile.File) layers gameplay
// data over an Aseprite export, its "sheet": loop and timing overrides, a
// pivot and hit/hurt boxes. The result is a ClipSet with the sheet's clips
// under the definition's own key ("hero.anim.json#slash"). Package animfile
// parses and checks the file; this adapter loads the sheet, converts the
// pivot to Frame.Origin (which needs the atlas sprites) and publishes the
// clips. doc/guide/engine/animation.md is the format reference.

// animDefSpec is decodeAnimDef's result: the parsed file and its sheet
// dependency. Everything that needs the sheet is checked in Finish
// (animfile.Resolve), which reports every problem of the file at once.
type animDefSpec struct {
	file  *animfile.File
	sheet asset.DepRef[ClipSet]
}

func decodeAnimDef(ctx *asset.DecodeContext, data []byte) (*animDefSpec, error) {
	f, err := animfile.Decode(data)
	if err != nil {
		return nil, fmt.Errorf("anim json: %w", err)
	}
	// Relative to the definition's directory, like every Dep path.
	return &animDefSpec{file: f, sheet: asset.Dep[ClipSet](ctx, f.Sheet)}, nil
}

// finishAnimDef applies the definition to the sheet's clips and publishes the
// results as children of the definition. Every clip is built and checked
// before any is published, so a bad file leaves no half set behind.
func (l *clipSetLoader) finishAnimDef(ctx *asset.FinishContext, spec *animDefSpec) (ClipSet, error) {
	baseH := asset.Resolve(ctx, spec.sheet)
	base := l.sets.Get(baseH)

	baseClips := make([]*Clip, len(base.Names))
	tags := make([]animfile.Tag, len(base.Names))
	for i, name := range base.Names {
		c := l.clips.Get(base.Clips[name])
		baseClips[i] = c
		tags[i] = animfile.Tag{Name: name, Loop: fileLoopMode(c.Loop), Durations: make([]time.Duration, len(c.Frames))}
		for j, f := range c.Frames {
			tags[i].Durations[j] = f.Duration
		}
	}
	res, err := animfile.Resolve(spec.file, tags)
	if err != nil {
		return ClipSet{}, fmt.Errorf("anim json: %w", err)
	}

	var errs []error
	built := make([]Clip, len(res.Clips))
	for i, rc := range res.Clips {
		c, clipErrs := l.buildClip(baseClips[i], &rc)
		c.Body = gfx.IRect{X: res.Body.X, Y: res.Body.Y, W: res.Body.W, H: res.Body.H}
		for _, e := range clipErrs {
			errs = append(errs, fmt.Errorf("clip %q: %w", rc.Name, e))
		}
		built[i] = c
	}
	if err := errors.Join(errs...); err != nil {
		return ClipSet{}, fmt.Errorf("anim json: %w", err)
	}

	set := ClipSet{
		Atlas:   base.Atlas,
		Clips:   make(map[string]gfx.ClipHandle, len(base.Names)),
		Names:   slices.Clone(base.Names),
		Default: res.Default,
	}
	for i, name := range set.Names {
		set.Clips[name] = gfx.ClipHandle{Ref: asset.AddChild(ctx, name, built[i]).Ref()}
	}
	return set, nil
}

// buildClip copies base (sprites, events) and applies the resolved clip:
// loop mode, durations, the pivot as Frame.Origin and the boxes.
func (l *clipSetLoader) buildClip(base *Clip, rc *animfile.ResolvedClip) (Clip, []error) {
	c := Clip{Loop: loopModeOf(rc.Loop), Frames: slices.Clone(base.Frames)}
	var errs []error
	for i := range c.Frames {
		f := &c.Frames[i]
		f.Duration = rc.Durations[i]
		c.Total += f.Duration
		if rc.Pivot != nil {
			o, err := l.origin(f.Sprite, *rc.Pivot)
			if err != nil {
				errs = append(errs, fmt.Errorf("frame %d: %w", i, err))
			}
			f.Origin = o
		}
	}
	setBoxes(c.Frames, rc.Boxes)
	return c, errs
}

// origin is [PivotOrigin] with the error the loader reports.
func (l *clipSetLoader) origin(h gfx.SpriteHandle, p animfile.Point) (gfx.Vec2, error) {
	sp := l.sprites.Get(h)
	if sp.Size.X <= 0 || sp.Size.Y <= 0 {
		return gfx.Vec2{}, fmt.Errorf("sprite has no size")
	}
	return PivotOrigin(sp, p), nil
}

// setBoxes converts the resolved boxes (per frame) to Frame.Boxes. All boxes
// of the clip share one backing array, and each frame gets a sub-slice capped
// at its own length, so a clip costs one allocation and an append to one
// frame's slice can never overwrite the next frame's boxes.
func setBoxes(frames []Frame, boxes [][]animfile.Box) {
	total := 0
	for _, fb := range boxes {
		total += len(fb)
	}
	if total == 0 {
		return
	}
	all := make([]gfx.Box, 0, total)
	for i, fb := range boxes {
		start := len(all)
		for _, b := range fb {
			all = append(all, gfx.Box{Kind: boxKindOf(b.Kind), ID: b.ID, Rect: gfx.IRect(b.Rect)})
		}
		if len(all) > start {
			frames[i].Boxes = all[start:len(all):len(all)]
		}
	}
}

// boxKindOf converts animfile's box kind, which mirrors gfx.BoxKind.
func boxKindOf(k animfile.BoxKind) gfx.BoxKind {
	if k == animfile.BoxHit {
		return gfx.BoxHit
	}
	return gfx.BoxHurt
}
