package anim

import (
	"fmt"
	"path"
	"strings"

	"github.com/weakpixel/asset"
	"github.com/weakpixel/engine/anim/animfile"
	"github.com/weakpixel/engine/gfx"
	"github.com/weakpixel/engine/render"
)

// RegisterLoaders registers [ClipSet] and [Clip] on srv. Call it after
// render.RegisterLoaders: a ClipSet depends on a render.Atlas for its sprites.
//
// It panics with "asset: type not registered" if the render loaders are
// missing, and like asset.Register if the types are already registered.
func RegisterLoaders(srv *asset.Server) {
	l := &clipSetLoader{
		atlases: render.AtlasStoreOf(srv), // panics if the render loaders are missing
		sprites: render.SpriteStoreOf(srv),
	}
	// The loader is a pointer so that it can hold the stores Register creates:
	// the Finish of an *.anim.json file reads its base set and that set's clips.
	l.sets = asset.Register[ClipSet, clipSetSpec](srv, l, ClipSet{})
	l.clips = ClipStore{asset.Register[Clip, struct{}](srv, clipLoader{}, Clip{})}
	asset.RegisterChild[ClipSet, Clip](srv)
}

// clipSetLoader loads a ClipSet from an Aseprite export or, for keys ending
// in animfile.Suffix (".anim.json"), from an animation definition
// (animdef.go). Parsing and checking both formats is package animfile's job;
// the loader resolves dependencies and builds the clips.
type clipSetLoader struct {
	atlases render.AtlasStore
	sprites render.SpriteStore
	sets    *asset.Store[ClipSet]
	clips   ClipStore
}

// clipSetSpec is Decode's result for either file kind: exactly one field is
// set.
type clipSetSpec struct {
	aseprite *asepriteSpec
	def      *animDefSpec
}

func (l *clipSetLoader) Decode(ctx *asset.DecodeContext, data []byte) (clipSetSpec, error) {
	// The suffix routes the key, as ".sheet.json" routes an atlas key to the
	// grid sheet loader.
	if strings.HasSuffix(ctx.Key.Path, animfile.Suffix) {
		def, err := decodeAnimDef(ctx, data)
		return clipSetSpec{def: def}, err
	}
	spec, err := decodeAseprite(ctx, data)
	return clipSetSpec{aseprite: spec}, err
}

func (l *clipSetLoader) Finish(ctx *asset.FinishContext, spec clipSetSpec) (ClipSet, error) {
	if spec.def != nil {
		return l.finishAnimDef(ctx, spec.def)
	}
	return l.finishAseprite(ctx, spec.aseprite)
}

func (*clipSetLoader) Unload(*ClipSet) {}

// asepriteSpec is decodeAseprite's result: the checked sheet plus the atlas
// dependency, so Finish only has to look up sprites.
type asepriteSpec struct {
	atlas asset.DepRef[render.Atlas]
	sheet *animfile.Sheet
}

// decodeAseprite reads an Aseprite export (animfile.Sheet). Frame i of the
// file is Atlas.Order[i]: the atlas loader reads the same file and promises
// file order.
func decodeAseprite(ctx *asset.DecodeContext, data []byte) (*asepriteSpec, error) {
	sheet, err := animfile.DecodeSheet(data)
	if err != nil {
		return nil, fmt.Errorf("aseprite json: %w", err)
	}
	return &asepriteSpec{
		// The same file, loaded as an atlas; the path is relative to its own dir.
		atlas: asset.Dep[render.Atlas](ctx, path.Base(ctx.Key.Path)),
		sheet: sheet,
	}, nil
}

func (l *clipSetLoader) finishAseprite(ctx *asset.FinishContext, spec *asepriteSpec) (ClipSet, error) {
	atlasH := render.AtlasHandle{Ref: asset.Resolve(ctx, spec.atlas).Ref()}
	atlas := l.atlases.Get(atlasH)
	frames := spec.sheet.Frames
	if len(atlas.Order) != len(frames) {
		return ClipSet{}, fmt.Errorf("aseprite json: %d frames have durations but the atlas has %d", len(frames), len(atlas.Order))
	}

	tags := spec.sheet.Tags
	set := ClipSet{
		Atlas: atlasH,
		Clips: make(map[string]gfx.ClipHandle, len(tags)),
		Names: make([]string, 0, len(tags)),
	}
	for _, t := range tags {
		clip := Clip{Loop: loopModeOf(t.Loop), Frames: make([]Frame, 0, t.To-t.From+1)}
		for _, i := range t.FrameIndices() { // play order: reversed for reverse tags
			clip.Frames = append(clip.Frames, Frame{
				Sprite:   atlas.Order[i],
				Duration: frames[i].Duration,
			})
			clip.Total += frames[i].Duration
		}
		set.Clips[t.Name] = gfx.ClipHandle{Ref: asset.AddChild(ctx, t.Name, clip).Ref()}
		set.Names = append(set.Names, t.Name)
	}
	return set, nil
}

// loopModeOf converts animfile's loop mode, which mirrors LoopMode.
func loopModeOf(m animfile.LoopMode) LoopMode {
	switch m {
	case animfile.Once:
		return Once
	case animfile.PingPong:
		return PingPong
	default:
		return Loop
	}
}

// fileLoopMode is the inverse of loopModeOf.
func fileLoopMode(m LoopMode) animfile.LoopMode {
	switch m {
	case Once:
		return animfile.Once
	case PingPong:
		return animfile.PingPong
	default:
		return animfile.Loop
	}
}

// clipLoader handles a Clip key without "#tag", which names no clip.
type clipLoader struct{}

func (clipLoader) Decode(ctx *asset.DecodeContext, _ []byte) (struct{}, error) {
	return struct{}{}, fmt.Errorf("anim: a Clip needs a #tag key, as in %q", ctx.Key.Path+"#run")
}

func (clipLoader) Finish(*asset.FinishContext, struct{}) (Clip, error) { return Clip{}, nil }
func (clipLoader) Unload(*Clip)                                        {}
