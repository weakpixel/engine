package particles

import (
	"github.com/weakpixel/engine/gfx"
	"github.com/weakpixel/engine/particles/fxfile"
)

// Resolve returns the emitter that c plays: eff's, with every
// non-nil override field of c applied. It copies the emitter and does not
// allocate. The Sprite and Shader keys stay the emitter's; the System and
// [Extract] draw with [SpriteOf] and [ShaderOf].
//
// The System resolves every tick instead of caching, so a value c does not
// override follows the effect file at once, hot reloads included.
func Resolve(eff *EffectEmitter, c *gfx.Particles) fxfile.Emitter {
	e := eff.Emitter
	set(&e.Params, c.Params)
	set(&e.Blend, c.Blend)
	set(&e.Color, c.Color)
	set(&e.EndColor, c.EndColor)
	set(&e.Scale, c.Scale)
	set(&e.EndScale, c.EndScale)
	set(&e.FadeIn, c.FadeIn)
	set(&e.FadeOut, c.FadeOut)
	set(&e.Rate, c.Rate)
	set(&e.Burst, c.Burst)
	set(&e.Duration, c.Duration)
	set(&e.Loop, c.Loop)
	set(&e.Max, c.Max)
	set(&e.Shape, c.Shape)
	set(&e.ShapeSize, c.ShapeSize)
	set(&e.LifeMin, c.LifeMin)
	set(&e.LifeMax, c.LifeMax)
	set(&e.Angle, c.Angle)
	set(&e.Spread, c.Spread)
	set(&e.SpeedMin, c.SpeedMin)
	set(&e.SpeedMax, c.SpeedMax)
	set(&e.Gravity, c.Gravity)
	set(&e.Drag, c.Drag)
	set(&e.SpinMin, c.SpinMin)
	set(&e.SpinMax, c.SpinMax)
	set(&e.RandomRotation, c.RandomRotation)
	set(&e.Local, c.Local)
	return e
}

// set stores *o in *dst when o is not nil.
func set[T any](dst, o *T) {
	if o != nil {
		*dst = *o
	}
}

// SpriteOf returns the particle image of c: its Sprite override when set,
// else the effect's.
func SpriteOf(eff *EffectEmitter, c *gfx.Particles) gfx.SpriteHandle {
	if c.Sprite != (gfx.SpriteHandle{}) {
		return c.Sprite
	}
	return eff.SpriteRef
}

// ShaderOf returns the shader of c: its Shader override when set, else the
// effect's.
func ShaderOf(eff *EffectEmitter, c *gfx.Particles) gfx.ShaderHandle {
	if c.Shader != (gfx.ShaderHandle{}) {
		return c.Shader
	}
	return eff.ShaderRef
}
