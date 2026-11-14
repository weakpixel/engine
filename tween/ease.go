package tween

import "github.com/weakpixel/engine/xmath"

// Ease shapes the progress of a tween: it maps the linear time fraction t
// (0 at the start of a cycle, 1 at its end) to the fraction of the way from
// the start value to the end value. The zero Ease is [Linear].
//
// Every Ease returns exactly 0 at 0 and exactly 1 at 1. The Back and Elastic
// curves overshoot in between (values below 0 or above 1), so lerp functions
// must extrapolate.
type Ease uint8

// The easing curves. In eases start slowly, Out eases end slowly, InOut
// eases do both. The formulas are Robert Penner's.
const (
	Linear Ease = iota
	InQuad
	OutQuad
	InOutQuad
	InCubic
	OutCubic
	InOutCubic
	InSine
	OutSine
	InOutSine
	InExpo
	OutExpo
	InOutExpo
	InBack
	OutBack
	InOutBack
	InElastic
	OutElastic
	InBounce
	OutBounce
	// Step stays at 0 until the end of the cycle, then jumps to 1.
	Step

	numEases
)

var easeNames = [numEases]string{
	"Linear", "InQuad", "OutQuad", "InOutQuad", "InCubic", "OutCubic", "InOutCubic",
	"InSine", "OutSine", "InOutSine", "InExpo", "OutExpo", "InOutExpo",
	"InBack", "OutBack", "InOutBack", "InElastic", "OutElastic", "InBounce", "OutBounce", "Step",
}

// String returns the constant's name, such as "OutBack".
func (e Ease) String() string {
	if e >= numEases {
		return "invalid"
	}
	return easeNames[e]
}

// easeFuncs is indexed by Ease and holds the xmath curves. A table of plain
// functions (no closures) keeps Ease.At free of allocations and branches on
// the enum.
var easeFuncs = [numEases]func(float32) float32{
	Linear:     xmath.EaseLinear,
	InQuad:     xmath.EaseInQuad,
	OutQuad:    xmath.EaseOutQuad,
	InOutQuad:  xmath.EaseInOutQuad,
	InCubic:    xmath.EaseInCubic,
	OutCubic:   xmath.EaseOutCubic,
	InOutCubic: xmath.EaseInOutCubic,
	InSine:     xmath.EaseInSine,
	OutSine:    xmath.EaseOutSine,
	InOutSine:  xmath.EaseInOutSine,
	InExpo:     xmath.EaseInExpo,
	OutExpo:    xmath.EaseOutExpo,
	InOutExpo:  xmath.EaseInOutExpo,
	InBack:     xmath.EaseInBack,
	OutBack:    xmath.EaseOutBack,
	InOutBack:  xmath.EaseInOutBack,
	InElastic:  xmath.EaseInElastic,
	OutElastic: xmath.EaseOutElastic,
	InBounce:   xmath.EaseInBounce,
	OutBounce:  xmath.EaseOutBounce,
	Step: func(t float32) float32 {
		if t >= 1 {
			return 1
		}
		return 0
	},
}

// At returns the eased fraction for t. t is clamped to [0, 1]; an invalid
// Ease behaves as Linear.
func (e Ease) At(t float32) float32 {
	switch {
	case t <= 0:
		return 0
	case t >= 1:
		return 1
	case e >= numEases:
		return t
	}
	return easeFuncs[e](t)
}
