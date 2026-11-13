package render

import (
	"bytes"
	"errors"
	"fmt"
	"math"
	"path"
	"strconv"

	"github.com/hajimehoshi/ebiten/v2/text/v2"

	"github.com/weakpixel/asset"
)

// FontSource is a parsed TTF or OTF file, shared by every size of that font.
// Load it with asset.Load[FontSource]; you usually load a [Font] instead.
type FontSource struct {
	Source *text.GoTextFaceSource
}

// FontSourceID is a handle to a [FontSource].
type FontSourceID = asset.Handle[FontSource]

// Font is a face of a [FontSource] at one size. The key carries the size in
// pixels after '#': [LoadFont](sc, "fonts/ui.ttf#16").
//
// A Font is cheap: it depends on the FontSource of the same file, so every
// size of a font parses the file once. (It is a dependent asset and not an
// asset child, because a FontSource cannot know which sizes will be asked
// for.)
type Font struct {
	// Face draws and measures text. It is a *text.GoTextFace.
	Face text.Face
	// LineHeight is the distance between two baselines, in pixels.
	LineHeight float64
}

// fontSourceLoader parses the font on a worker: it is CPU work only and
// creates no GPU objects.
type fontSourceLoader struct{}

func (fontSourceLoader) Decode(_ *asset.DecodeContext, data []byte) (*text.GoTextFaceSource, error) {
	return text.NewGoTextFaceSource(bytes.NewReader(data))
}

func (fontSourceLoader) Finish(_ *asset.FinishContext, src *text.GoTextFaceSource) (FontSource, error) {
	return FontSource{Source: src}, nil
}

func (fontSourceLoader) Unload(*FontSource) {}

// fontLoader builds a face from the font source of the same file. Decode
// ignores the file bytes: the source dependency has them.
type fontLoader struct {
	sources *asset.Store[FontSource]
}

type fontSpec struct {
	source asset.DepRef[FontSource]
	size   float64
}

func (fontLoader) Decode(ctx *asset.DecodeContext, _ []byte) (fontSpec, error) {
	size, err := parseFontSize(ctx.Key.Sub)
	if err != nil {
		return fontSpec{}, err
	}
	return fontSpec{
		source: asset.Dep[FontSource](ctx, path.Base(ctx.Key.Path)),
		size:   size,
	}, nil
}

func (l fontLoader) Finish(ctx *asset.FinishContext, spec fontSpec) (Font, error) {
	src := l.sources.Get(asset.Resolve(ctx, spec.source))
	return newFont(src.Source, spec.size), nil
}

func (fontLoader) Unload(*Font) {}

func newFont(src *text.GoTextFaceSource, size float64) Font {
	face := &text.GoTextFace{Source: src, Size: size}
	m := face.Metrics()
	return Font{Face: face, LineHeight: m.HAscent + m.HDescent + m.HLineGap}
}

// parseFontSize reads the size after '#' in a Font key.
func parseFontSize(sub string) (float64, error) {
	if sub == "" {
		return 0, errors.New(`font key needs a size, like "fonts/ui.ttf#16"`)
	}
	size, err := strconv.ParseFloat(sub, 64)
	if err != nil || math.IsNaN(size) || math.IsInf(size, 0) || size <= 0 {
		return 0, fmt.Errorf("font size %q must be a positive number of pixels", sub)
	}
	return size, nil
}
