package fxedit

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"path"
	"strings"

	"github.com/weakpixel/engine/gfx"
	"github.com/weakpixel/engine/internal/shaderparam"
	"github.com/weakpixel/engine/particles/fxfile"
)

// The ids of the panel's fields. Set receives them; the edit functions
// below are keyed by them.
const (
	IDPreset  = "fx.preset"
	IDApply   = "fx.apply"
	IDKeep    = "fx.keep"
	IDRestart = "fx.restart"

	IDImage       = "look.image"
	IDShader      = "look.shader"
	IDParamPrefix = "look.param."  // + marker index
	IDRawPrefix   = "look.params." // + 0..3, without markers
	IDBlend       = "look.blend"
	IDColor       = "look.color"
	IDChange      = "look.change"
	IDEndColor    = "look.endcolor"
	IDEndScale    = "look.endscale"
	IDScale       = "look.scale"
	IDFadeIn      = "look.fadein"
	IDFadeOut     = "look.fadeout"

	IDRate     = "emit.rate"
	IDBurst    = "emit.burst"
	IDDuration = "emit.duration"
	IDLoop     = "emit.loop"
	IDMax      = "emit.max"

	IDShape  = "shape.kind"
	IDRadius = "shape.radius"
	IDWidth  = "shape.width"
	IDHeight = "shape.height"

	IDLife   = "motion.life"
	IDAngle  = "motion.angle"
	IDSpread = "motion.spread"
	IDSpeed  = "motion.speed"
	IDGravX  = "motion.gx"
	IDGravY  = "motion.gy"
	IDDrag   = "motion.drag"
	IDSpin   = "motion.spin"
	IDRotate = "motion.rot"

	IDLocal = "adv.local"
)

// Emitter is the effect's emitter the panel edits: package particles' type,
// in the units of the simulation (radians, world units).
type Emitter = fxfile.Emitter

// BlendNames are the entries of the Blend dropdown, indexed by gfx.Blend.
var BlendNames = []string{"Normal", "Additive", "Multiply", "Screen", "Subtract", "Lighten", "Erase"}

// ShapeNames are the entries of the Shape dropdown, indexed by gfx.EmitShape.
var ShapeNames = []string{"Point", "Circle", "Rect"}

// Context is what an edit needs besides the emitter: the parameters the
// emitter's shader declares (none: the four plain Params numbers).
type Context struct {
	Params []shaderparam.Param
}

// Apply applies the change of one panel field to e. It returns an error for
// a value that is nonsense for the field (a negative rate, a crossed range, a
// blend mode that does not exist); e is then unchanged. Buttons and the
// preset widgets are not fields of the emitter and are handled by the
// panel.
func Apply(e *Emitter, ctx Context, id string, v json.RawMessage) (err error) {
	next := *e
	switch {
	case strings.HasPrefix(id, IDParamPrefix):
		err = applyParam(&next, ctx, strings.TrimPrefix(id, IDParamPrefix), v)
	case strings.HasPrefix(id, IDRawPrefix):
		var i int
		if i, err = indexIn(strings.TrimPrefix(id, IDRawPrefix), 4); err == nil {
			err = setFloat(v, math.Inf(-1), math.Inf(1), func(f float64) { next.Params[i] = float32(f) })
		}
	default:
		err = applyField(&next, id, v)
	}
	if err != nil {
		return fmt.Errorf("fx: %s: %w", id, err)
	}
	*e = next
	return nil
}

func applyField(e *Emitter, id string, v json.RawMessage) error {
	em := e
	f32 := func(dst *float32, lo, hi float64) error {
		return setFloat(v, lo, hi, func(f float64) { *dst = float32(f) })
	}
	deg := func(dst *float32, lo, hi float64) error {
		return setFloat(v, lo, hi, func(f float64) { *dst = float32(f * math.Pi / 180) })
	}
	inf := math.Inf(1)
	switch id {
	case IDImage:
		return setKey(v, &e.Sprite)
	case IDShader:
		if err := setKey(v, &e.Shader); err != nil {
			return err
		}
		if e.Shader == "" {
			em.Params = [4]float32{}
		}
		return nil
	case IDBlend:
		return setIndex(v, len(BlendNames), func(i int) { em.Blend = gfx.Blend(i) })
	case IDColor:
		return setColor(v, &em.Color)
	case IDEndColor:
		return setColor(v, &em.EndColor)
	case IDChange:
		return setChange(e, v)
	case IDEndScale:
		return f32(&em.EndScale, 0, inf)
	case IDScale:
		return f32(&em.Scale, 0, inf)
	case IDFadeIn:
		return f32(&em.FadeIn, 0, 1)
	case IDFadeOut:
		return f32(&em.FadeOut, 0, 1)
	case IDRate:
		return f32(&em.Rate, 0, inf)
	case IDBurst:
		return setFloat(v, 0, math.MaxUint16, func(f float64) { em.Burst = uint16(f) })
	case IDDuration:
		return f32(&em.Duration, 0, inf)
	case IDLoop:
		return setBool(v, &em.Loop)
	case IDMax:
		return setFloat(v, 0, math.MaxUint16, func(f float64) { em.Max = uint16(f) })
	case IDShape:
		return setIndex(v, len(ShapeNames), func(i int) { em.Shape = gfx.EmitShape(i) })
	case IDRadius, IDWidth:
		return f32(&em.ShapeSize.X, 0, inf)
	case IDHeight:
		return f32(&em.ShapeSize.Y, 0, inf)
	case IDLife:
		return setRange(v, 0, inf, 1, &em.LifeMin, &em.LifeMax)
	case IDAngle:
		return deg(&em.Angle, math.Inf(-1), inf)
	case IDSpread:
		return deg(&em.Spread, 0, 360)
	case IDSpeed:
		return setRange(v, 0, inf, 1, &em.SpeedMin, &em.SpeedMax)
	case IDGravX:
		return f32(&em.Gravity.X, math.Inf(-1), inf)
	case IDGravY:
		return f32(&em.Gravity.Y, math.Inf(-1), inf)
	case IDDrag:
		return f32(&em.Drag, 0, inf)
	case IDSpin:
		return setRange(v, math.Inf(-1), inf, math.Pi/180, &em.SpinMin, &em.SpinMax)
	case IDRotate:
		return setBool(v, &em.RandomRotation)
	case IDLocal:
		return setBool(v, &em.Local)
	}
	return errors.New("unknown field")
}

// applyParam applies a shader-declared parameter: n is the marker index.
func applyParam(e *Emitter, ctx Context, n string, v json.RawMessage) error {
	i, err := indexIn(n, len(ctx.Params))
	if err != nil {
		return err
	}
	p := ctx.Params[i]
	set := func(slot int, f float64) { e.Params[p.Slots[slot]] = float32(f) }
	switch p.Widget {
	case "color":
		var c gfx.Color
		if err := setColor(v, &c); err != nil {
			return err
		}
		ch := [4]float64{float64(c >> 24), float64(c >> 16 & 255), float64(c >> 8 & 255), float64(c & 255)}
		for s := range p.Slots {
			set(s, ch[s]/255)
		}
		return nil
	case "toggle":
		var on bool
		if err := setBool(v, &on); err != nil {
			return err
		}
		set(0, map[bool]float64{false: 0, true: 1}[on])
		return nil
	case "angle":
		return setFloat(v, math.Inf(-1), math.Inf(1), func(f float64) { set(0, f*math.Pi/180) })
	}
	lo, hi := math.Inf(-1), math.Inf(1)
	if p.HasRange {
		lo, hi = p.Min, p.Max
	}
	return setFloat(v, lo, hi, func(f float64) { set(0, f) })
}

// setChange turns "change over life" on or off. Off clears EndColor and
// EndScale (zero means no change). On starts a fade to transparent and a
// shrink to half, which differ from the start values, so the fields appear.
func setChange(e *Emitter, v json.RawMessage) error {
	var on bool
	if err := setBool(v, &on); err != nil {
		return err
	}
	if !on {
		e.EndColor, e.EndScale = 0, 0
		return nil
	}
	if ChangesOverLife(*e) {
		return nil
	}
	start := e.Color
	if start == 0 {
		start = gfx.White
	}
	e.EndColor = start &^ 0xff
	e.EndScale = EffectiveScale(*e) / 2
	return nil
}

// EffectiveScale is the particle size at birth: Scale, 1 when zero.
func EffectiveScale(e Emitter) float32 {
	if e.Scale == 0 {
		return 1
	}
	return e.Scale
}

// ChangesOverLife reports whether the particle's color or size differs
// between birth and death.
func ChangesOverLife(e Emitter) bool {
	return (e.EndColor != 0 && e.EndColor != e.Color) ||
		(e.EndScale != 0 && e.EndScale != EffectiveScale(e))
}

// ApplyPreset returns the emitter with the values of the preset's first
// emitter. With keep a shader (and its Params) and a sprite from the project
// stay: the user picked them. A built-in sprite or shader is the preset's
// own and is replaced.
func ApplyPreset(cur Emitter, p Preset, keep bool) Emitter {
	out := p.Emitters[0]
	if keep {
		if cur.Sprite != "" && !IsBuiltin(cur.Sprite) {
			out.Sprite = cur.Sprite
		}
		if cur.Shader != "" && !IsBuiltin(cur.Shader) {
			out.Shader, out.Params = cur.Shader, cur.Params
		}
	}
	return out
}

// ColorHex formats c as "#rrggbbaa"; the zero color is white.
func ColorHex(c gfx.Color) string {
	if c == 0 {
		c = gfx.White
	}
	return fmt.Sprintf("#%08x", uint32(c))
}

// ParseColor reads "#rrggbbaa".
func ParseColor(s string) (gfx.Color, error) {
	var c uint32
	if len(s) != 9 || s[0] != '#' {
		return 0, fmt.Errorf("color %q is not #rrggbbaa", s)
	}
	if _, err := fmt.Sscanf(s[1:], "%08x", &c); err != nil {
		return 0, fmt.Errorf("color %q is not #rrggbbaa", s)
	}
	return gfx.Color(c), nil
}

// Degrees converts radians to degrees for the panel, rounded to hundredths
// so a stored -1.5708 shows as -90.
func Degrees(rad float32) float64 {
	return math.Round(float64(rad)*180/math.Pi*100) / 100
}

func setFloat(v json.RawMessage, lo, hi float64, set func(float64)) error {
	var f float64
	if err := json.Unmarshal(v, &f); err != nil {
		return fmt.Errorf("want a number: %w", err)
	}
	if math.IsNaN(f) || f < lo || f > hi {
		return fmt.Errorf("%v is outside %v..%v", f, lo, hi)
	}
	set(f)
	return nil
}

func setBool(v json.RawMessage, dst *bool) error {
	if err := json.Unmarshal(v, dst); err != nil {
		return fmt.Errorf("want true or false: %w", err)
	}
	return nil
}

func setIndex(v json.RawMessage, n int, set func(int)) error {
	var i int
	if err := json.Unmarshal(v, &i); err != nil {
		return fmt.Errorf("want an option index: %w", err)
	}
	if i < 0 || i >= n {
		return fmt.Errorf("option %d does not exist (%d options)", i, n)
	}
	set(i)
	return nil
}

func setColor(v json.RawMessage, dst *gfx.Color) error {
	var s string
	if err := json.Unmarshal(v, &s); err != nil {
		return fmt.Errorf("want a color string: %w", err)
	}
	c, err := ParseColor(s)
	if err != nil {
		return err
	}
	*dst = c
	return nil
}

// setKey takes an asset key: relative, slash-separated, without "..".
func setKey(v json.RawMessage, dst *string) error {
	var s string
	if err := json.Unmarshal(v, &s); err != nil {
		return fmt.Errorf("want an asset key: %w", err)
	}
	if s != "" && (path.IsAbs(s) || path.Clean(s) != s || strings.HasPrefix(s, "../") || strings.Contains(s, "\\")) {
		return fmt.Errorf("%q is not a relative asset key", s)
	}
	*dst = s
	return nil
}

// setRange reads [lo, hi], which must not cross, scales both by k and stores
// them.
func setRange(v json.RawMessage, min, max, k float64, lo, hi *float32) error {
	var r []float64
	if err := json.Unmarshal(v, &r); err != nil || len(r) != 2 {
		return fmt.Errorf("want [low, high]")
	}
	if r[0] > r[1] {
		return fmt.Errorf("low %v is above high %v", r[0], r[1])
	}
	for _, f := range r {
		if math.IsNaN(f) || f < min || f > max {
			return fmt.Errorf("%v is outside %v..%v", f, min, max)
		}
	}
	*lo, *hi = float32(r[0]*k), float32(r[1]*k)
	return nil
}

func indexIn(s string, n int) (int, error) {
	var i int
	if _, err := fmt.Sscanf(s, "%d", &i); err != nil || i < 0 || i >= n {
		return 0, fmt.Errorf("no parameter %q", s)
	}
	return i, nil
}
