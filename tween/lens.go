package tween

import (
	"fmt"
	"sync/atomic"

	"github.com/weakpixel/engine/gfx"
)

// Lens selects the field a tween writes: a value of type V inside a
// component C. Create lenses once, as package-level variables, with
// [NewLens]; each lens gets its own pool in every [Tweens], and the overwrite
// rule (see [To]) is per entity and lens.
//
// A game's own field is one line:
//
//	var HPBar = tween.NewLens(func(h *Health) *float32 { return &h.Shown }, tween.LerpFloat)
type Lens[C, V any] struct {
	id   uint32
	get  func(*C) *V
	lerp func(a, b V, t float32) V
	// norm, when set, maps a value read from the field to the value it
	// means (a zero gfx.Transform.Scale means (1, 1)).
	norm func(V) V
}

// lensIDs numbers lenses process-wide, so a Tweens can index its pools by
// lens without a map. Lenses are usually created in package init, possibly
// in parallel tests, hence the atomic.
var lensIDs atomic.Uint32

// NewLens returns the lens for the field get points to. get must return a
// pointer into the component it is given (never nil). lerp returns the value
// t of the way from a to b; it must give a at 0 and b at 1 and extrapolate
// for t outside [0, 1] (overshooting eases, and [Builder.Relative] and
// [Builder.Additive], rely on it). The Lerp functions of this package do.
//
// Panics with "tween: nil lens function" if get or lerp is nil.
func NewLens[C, V any](get func(*C) *V, lerp func(a, b V, t float32) V) *Lens[C, V] {
	if get == nil || lerp == nil {
		panic("tween: nil lens function")
	}
	return &Lens[C, V]{id: lensIDs.Add(1), get: get, lerp: lerp}
}

// add returns a + b - c. Lenses carry only a lerp, but for the vector-like
// values tweens animate a + b - c is an affine combination (its weights sum
// to 1), so two lerps compute it: m = (a+b)/2, then c + 2(m-c). Relative
// targets (start + offset - zero) and additive steps (field + now - prev)
// both use it. It is exact up to float rounding for float32, gfx.Vec2 and
// [4]float32; for gfx.Color it rounds each channel to a byte.
func (l *Lens[C, V]) add(a, b, c V) V {
	return l.lerp(c, l.lerp(a, b, 0.5), 2)
}

// Built-in lenses for the engine's components.
var (
	// Pos is gfx.Transform.Pos.
	Pos = NewLens(func(t *gfx.Transform) *gfx.Vec2 { return &t.Pos }, LerpVec2)
	// Scale is gfx.Transform.Scale. A zero Scale counts as (1, 1), as when
	// drawing, so tweening an unscaled entity starts from its real size.
	Scale = withNorm(NewLens(func(t *gfx.Transform) *gfx.Vec2 { return &t.Scale }, LerpVec2), normScale)
	// Rot is gfx.Transform.Rot, in radians.
	Rot = NewLens(func(t *gfx.Transform) *float32 { return &t.Rot }, LerpFloat)
	// Tint is gfx.Sprite.Tint. Fade a sprite out by tweening it to a
	// color with zero alpha, such as gfx.RGBA(255, 255, 255, 0).
	Tint = NewLens(func(s *gfx.Sprite) *gfx.Color { return &s.Tint }, LerpColor)
	// Params is gfx.Sprite.Params, the shader's Params uniform.
	Params = NewLens(func(s *gfx.Sprite) *[4]float32 { return &s.Params }, LerpFloat4)
)

// paramLenses are the lenses Param returns. Building them once means Param
// never allocates, and the same index always gives the same lens, which the
// overwrite rule needs.
var paramLenses = [4]*Lens[gfx.Sprite, float32]{
	NewLens(func(s *gfx.Sprite) *float32 { return &s.Params[0] }, LerpFloat),
	NewLens(func(s *gfx.Sprite) *float32 { return &s.Params[1] }, LerpFloat),
	NewLens(func(s *gfx.Sprite) *float32 { return &s.Params[2] }, LerpFloat),
	NewLens(func(s *gfx.Sprite) *float32 { return &s.Params[3] }, LerpFloat),
}

// Param returns the lens for one shader parameter, gfx.Sprite.Params[i].
// Tweens on different parameters of one sprite do not overwrite each other.
//
// Panics with "tween: Param index out of range: <i>" unless 0 <= i < 4.
func Param(i int) *Lens[gfx.Sprite, float32] {
	if i < 0 || i >= len(paramLenses) {
		panic(fmt.Sprintf("tween: Param index out of range: %d", i))
	}
	return paramLenses[i]
}

func withNorm[C, V any](l *Lens[C, V], norm func(V) V) *Lens[C, V] {
	l.norm = norm
	return l
}

func normScale(v gfx.Vec2) gfx.Vec2 {
	if v == (gfx.Vec2{}) {
		return gfx.Vec2{X: 1, Y: 1}
	}
	return v
}
