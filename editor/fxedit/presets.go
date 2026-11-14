package fxedit

import (
	"fmt"
	"math"
	"strings"

	"github.com/weakpixel/engine/gfx"
	"github.com/weakpixel/engine/particles/fxfile"
)

// Preset is a named starting point for an effect: one emitter, or a
// complete effect of several. The values are tuned by eye for a game of a
// few hundred pixels across; a preset only ever changes with the editor,
// never the engine.
type Preset struct {
	Name     string
	Emitters []Emitter
}

// Multi reports whether the preset is a whole effect of several emitters.
// Applying it replaces the effect, not the selected emitter.
func (p Preset) Multi() bool { return len(p.Emitters) > 1 }

// DefaultPreset is the preset a new effect starts from.
const DefaultPreset = 0

const (
	up    = -math.Pi / 2 // screen up: Y grows downward and Angle turns clockwise
	down  = math.Pi / 2
	round = 2 * math.Pi // a full circle of Spread
)

// one is a preset of a single emitter.
func one(name string, e Emitter) Preset { return Preset{name, []Emitter{e}} }

// named returns e with its Name set, for the emitters of a multi preset.
func named(name string, e Emitter) Emitter {
	e.Name = name
	return e
}

// Presets returns the presets: smoke first, then the single emitters, then
// the effects of several emitters.
func Presets() []Preset {
	return []Preset{
		one("Smoke", Emitter{Sprite: SoftCircle,
			Color: 0xa0a0aeb0, EndColor: 0x5a5a6600, Scale: 0.25, EndScale: 0.7,
			FadeIn: 0.2, FadeOut: 0.6, Rate: 18, Max: 128,
			Shape: gfx.EmitCircle, ShapeSize: gfx.Vec2{X: 4},
			LifeMin: 1.2, LifeMax: 2.4, Angle: up, Spread: 0.5, SpeedMin: 14, SpeedMax: 28,
			Gravity: gfx.Vec2{Y: -6}, Drag: 0.8, SpinMin: -0.5, SpinMax: 0.5, RandomRotation: true,
		}),
		one("Sparks", Emitter{Sprite: Spark,
			Blend: gfx.BlendAdditive, Color: 0xffe08aff, EndColor: 0xff6a0000, Scale: 0.14, EndScale: 0.02,
			Burst: 30, Duration: 1, Loop: true, Max: 64,
			LifeMin: 0.3, LifeMax: 0.7, Angle: up, Spread: 2.6, SpeedMin: 60, SpeedMax: 140,
			Gravity: gfx.Vec2{Y: 220}, Drag: 1, RandomRotation: true,
		}),
		one("Dust", Emitter{Sprite: SoftCircle,
			Color: 0xc8b48c70, EndColor: 0xc8b48c00, Scale: 0.18, EndScale: 0.45,
			FadeIn: 0.3, FadeOut: 0.5, Rate: 12, Max: 64,
			Shape: gfx.EmitRect, ShapeSize: gfx.Vec2{X: 16, Y: 2},
			LifeMin: 0.6, LifeMax: 1.2, Angle: up, Spread: math.Pi, SpeedMin: 4, SpeedMax: 12, Drag: 2,
		}),
		one("Embers", Emitter{Sprite: Dot,
			Blend: gfx.BlendAdditive, Color: 0xffb347ff, EndColor: 0xff3c0000, Scale: 0.1, EndScale: 0.04,
			FadeIn: 0.1, FadeOut: 0.5, Rate: 14, Max: 64,
			Shape: gfx.EmitCircle, ShapeSize: gfx.Vec2{X: 6},
			LifeMin: 1, LifeMax: 2.2, Angle: up, Spread: 0.8, SpeedMin: 10, SpeedMax: 26,
			Gravity: gfx.Vec2{Y: -8}, Drag: 0.3,
		}),
		one("Explosion", Emitter{Sprite: SoftCircle,
			Blend: gfx.BlendAdditive, Color: 0xffd070ff, EndColor: 0xff400000, Scale: 0.3, EndScale: 0.9,
			FadeOut: 0.5, Burst: 40, Duration: 0.6, Loop: true, Max: 128,
			Shape: gfx.EmitCircle, ShapeSize: gfx.Vec2{X: 3},
			LifeMin: 0.3, LifeMax: 0.8, Spread: round, SpeedMin: 40, SpeedMax: 120, Drag: 3,
		}),
		one("Fire", fire),
		one("Rain", rain),
		one("Snow", Emitter{Sprite: Flake,
			Color: 0xffffffe8, Scale: 0.16, EndScale: 0.11, FadeIn: 0.1, FadeOut: 0.25, Rate: 22, Max: 192,
			Shape: gfx.EmitRect, ShapeSize: gfx.Vec2{X: 280, Y: 4},
			LifeMin: 6, LifeMax: 9, Angle: down, Spread: 0.7, SpeedMin: 20, SpeedMax: 36,
			Gravity: gfx.Vec2{X: 4, Y: 8}, Drag: 0.3, SpinMin: -1.2, SpinMax: 1.2, RandomRotation: true,
		}),
		one("Fireflies", Emitter{Sprite: Dot,
			Blend: gfx.BlendAdditive, Color: 0xdcff70ff, EndColor: 0x80ff5000, Scale: 0.08, EndScale: 0.05,
			FadeIn: 0.4, FadeOut: 0.4, Rate: 7, Max: 48,
			Shape: gfx.EmitCircle, ShapeSize: gfx.Vec2{X: 70},
			LifeMin: 2, LifeMax: 4, Spread: round, SpeedMin: 4, SpeedMax: 14,
			Gravity: gfx.Vec2{Y: -2}, Drag: 0.2,
		}),
		one("Bubbles", Emitter{Sprite: Ring,
			Color: 0xc8ecffe0, Scale: 0.1, EndScale: 0.24, FadeIn: 0.15, FadeOut: 0.15, Rate: 5, Max: 48,
			Shape: gfx.EmitRect, ShapeSize: gfx.Vec2{X: 48, Y: 4},
			LifeMin: 2, LifeMax: 3.5, Angle: up, Spread: 0.35, SpeedMin: 16, SpeedMax: 32,
			Gravity: gfx.Vec2{Y: -12}, Drag: 0.7,
		}),
		one("Magic", Emitter{Sprite: Star,
			Blend: gfx.BlendAdditive, Color: 0xfff3a8ff, EndColor: 0xff70e000, Scale: 0.16, EndScale: 0.03,
			FadeIn: 0.2, Rate: 18, Max: 64,
			Shape: gfx.EmitCircle, ShapeSize: gfx.Vec2{X: 22},
			LifeMin: 0.6, LifeMax: 1.2, Spread: round, SpeedMin: 6, SpeedMax: 20,
			Gravity: gfx.Vec2{Y: -8}, SpinMin: -4, SpinMax: 4, RandomRotation: true,
		}),
		one("Hearts", Emitter{Sprite: Heart,
			Color: 0xff4d8cff, EndColor: 0xffa0c800, Scale: 0.14, EndScale: 0.22, FadeIn: 0.15, Rate: 4, Max: 32,
			Shape: gfx.EmitCircle, ShapeSize: gfx.Vec2{X: 6},
			LifeMin: 1.5, LifeMax: 2.2, Angle: up, Spread: 0.6, SpeedMin: 18, SpeedMax: 30,
			Gravity: gfx.Vec2{Y: -12}, Drag: 0.8, SpinMin: -0.5, SpinMax: 0.5,
		}),
		one("Heal", Emitter{Sprite: Plus,
			Blend: gfx.BlendAdditive, Color: 0x7dff9aff, EndColor: 0x2adf8000, Scale: 0.13, EndScale: 0.07,
			FadeIn: 0.2, Rate: 10, Max: 32,
			Shape: gfx.EmitRect, ShapeSize: gfx.Vec2{X: 28, Y: 8},
			LifeMin: 0.8, LifeMax: 1.4, Angle: up, Spread: 0.15, SpeedMin: 20, SpeedMax: 36, Drag: 0.5,
		}),
		one("Shockwave", Emitter{Sprite: Ring,
			Blend: gfx.BlendAdditive, Color: 0xa8ecffff, EndColor: 0x3a8cff00, Scale: 0.2, EndScale: 2.6,
			Burst: 2, Duration: 1.2, Loop: true, Max: 4,
			LifeMin: 0.5, LifeMax: 0.8,
		}),
		one("Splash", Emitter{Sprite: Dot,
			Color: 0x6ab8ffff, EndColor: 0xc0e6ff00, Scale: 0.09, EndScale: 0.04,
			Burst: 28, Duration: 1.4, Loop: true, Max: 64,
			Shape: gfx.EmitRect, ShapeSize: gfx.Vec2{X: 10, Y: 1},
			LifeMin: 0.5, LifeMax: 0.9, Angle: up, Spread: 1.6, SpeedMin: 70, SpeedMax: 140,
			Gravity: gfx.Vec2{Y: 380}, Drag: 0.4,
		}),
		one("Pixels", Emitter{Sprite: Square,
			Color: 0x6af0ffff, EndColor: 0xb44dff00, Scale: 0.07, EndScale: 0.05,
			Burst: 40, Duration: 1, Loop: true, Max: 64,
			LifeMin: 0.6, LifeMax: 1, Spread: round, SpeedMin: 30, SpeedMax: 110, Drag: 2.5,
		}),
		{"Campfire", campfire()},
		{"Fireworks", fireworks()},
		{"Thunderstorm", thunderstorm()},
		{"Confetti", confetti()},
		{"Magic portal", portal()},
		{"Dragon breath", dragonBreath()},
		{"Burning scraps", burningScraps()},
		{"Supernova", supernova()},
	}
}

// fire and rain are presets of their own and parts of bigger effects.
var (
	fire = Emitter{Sprite: SoftCircle,
		Blend: gfx.BlendAdditive, Color: 0xffc04aff, EndColor: 0xc01e0000, Scale: 0.36, EndScale: 0.08,
		FadeIn: 0.1, FadeOut: 0.3, Rate: 40, Max: 96,
		Shape: gfx.EmitRect, ShapeSize: gfx.Vec2{X: 14, Y: 3},
		LifeMin: 0.5, LifeMax: 1, Angle: up, Spread: 0.35, SpeedMin: 25, SpeedMax: 50,
		Gravity: gfx.Vec2{Y: -30}, Drag: 0.5,
	}
	rain = Emitter{Sprite: Streak,
		Color: 0xa8c8ffb0, Scale: 0.25, FadeOut: 0.15, Rate: 80, Max: 256,
		Shape: gfx.EmitRect, ShapeSize: gfx.Vec2{X: 280, Y: 4},
		LifeMin: 0.8, LifeMax: 1, Angle: down, Spread: 0.04, SpeedMin: 260, SpeedMax: 320,
		Gravity: gfx.Vec2{Y: 200},
	}
)

func campfire() []Emitter {
	return []Emitter{
		named("Smoke", Emitter{Sprite: SoftCircle,
			Color: 0x6a6a74a0, EndColor: 0x3a3a4200, Scale: 0.3, EndScale: 1.2,
			FadeIn: 0.35, Rate: 8, Max: 48,
			Shape: gfx.EmitCircle, ShapeSize: gfx.Vec2{X: 6},
			LifeMin: 2.5, LifeMax: 4, Angle: up, Spread: 0.3, SpeedMin: 18, SpeedMax: 30,
			Gravity: gfx.Vec2{X: 5, Y: -4}, Drag: 0.3, SpinMin: -0.4, SpinMax: 0.4, RandomRotation: true,
		}),
		named("Flames", Emitter{Sprite: SoftCircle,
			Blend: gfx.BlendAdditive, Color: 0xff7a24ff, EndColor: 0x9a120000, Scale: 0.42, EndScale: 0.1,
			FadeIn: 0.1, FadeOut: 0.3, Rate: 45, Max: 64,
			Shape: gfx.EmitRect, ShapeSize: gfx.Vec2{X: 20, Y: 4},
			LifeMin: 0.6, LifeMax: 1, Angle: up, Spread: 0.3, SpeedMin: 30, SpeedMax: 60,
			Gravity: gfx.Vec2{Y: -50}, Drag: 0.6,
		}),
		named("Core", Emitter{Sprite: SoftCircle,
			Blend: gfx.BlendAdditive, Color: 0xfff0b0ff, EndColor: 0xffa03000, Scale: 0.3, EndScale: 0.06,
			Rate: 30, Max: 32,
			Shape: gfx.EmitRect, ShapeSize: gfx.Vec2{X: 10, Y: 3},
			LifeMin: 0.25, LifeMax: 0.45, Angle: up, Spread: 0.2, SpeedMin: 20, SpeedMax: 40,
		}),
		named("Embers", Emitter{Sprite: Dot,
			Blend: gfx.BlendAdditive, Color: 0xffc860ff, EndColor: 0xff400000, Scale: 0.05, EndScale: 0.02,
			FadeOut: 0.3, Rate: 8, Max: 32,
			Shape: gfx.EmitCircle, ShapeSize: gfx.Vec2{X: 6},
			LifeMin: 1.2, LifeMax: 2.4, Angle: up, Spread: 0.9, SpeedMin: 30, SpeedMax: 70,
			Gravity: gfx.Vec2{X: 6, Y: -10}, Drag: 0.4,
		}),
		named("Pops", Emitter{Sprite: Spark,
			Blend: gfx.BlendAdditive, Color: 0xffe9a0ff, EndColor: 0xff600000, Scale: 0.1, EndScale: 0.02,
			Burst: 6, Duration: 0.9, Loop: true, Max: 16,
			LifeMin: 0.3, LifeMax: 0.6, Angle: up, Spread: 1.4, SpeedMin: 60, SpeedMax: 120,
			Gravity: gfx.Vec2{Y: 160}, RandomRotation: true,
		}),
	}
}

// fireworks bursts every 2.2 s: every emitter shares the cycle, so the
// flash, the ring and the two shells of stars go off together.
func fireworks() []Emitter {
	shell := Emitter{Sprite: Spark,
		Blend: gfx.BlendAdditive, Scale: 0.2, EndScale: 0.04, FadeOut: 0.3,
		Duration: 2.2, Loop: true, Max: 128,
		Spread: round, Gravity: gfx.Vec2{Y: 40}, Drag: 1.4, RandomRotation: true,
	}
	pink, gold := shell, shell
	// A narrow speed range keeps a shell a crisp sphere.
	pink.Color, pink.EndColor, pink.Burst = 0xff6ad5ff, 0x8a2be200, 70
	pink.LifeMin, pink.LifeMax, pink.SpeedMin, pink.SpeedMax = 1.2, 1.8, 150, 170
	gold.Color, gold.EndColor, gold.Burst = 0xffe680ff, 0xff802000, 48
	gold.LifeMin, gold.LifeMax, gold.SpeedMin, gold.SpeedMax = 1.4, 2, 80, 92
	return []Emitter{
		named("Smoke", Emitter{Sprite: SoftCircle,
			Color: 0x6a607850, EndColor: 0x30283800, Scale: 0.4, EndScale: 1.4,
			FadeIn: 0.2, Burst: 16, Duration: 2.2, Loop: true, Max: 32,
			Shape: gfx.EmitCircle, ShapeSize: gfx.Vec2{X: 10},
			LifeMin: 1.8, LifeMax: 2.1, Spread: round, SpeedMin: 10, SpeedMax: 40,
			Gravity: gfx.Vec2{Y: -4}, Drag: 1, RandomRotation: true,
		}),
		named("Flash", Emitter{Sprite: SoftCircle,
			Blend: gfx.BlendAdditive, Color: 0xfff6d8ff, EndColor: 0xffb04000, Scale: 1.2, EndScale: 2.5,
			Burst: 1, Duration: 2.2, Loop: true, Max: 2,
			LifeMin: 0.25,
		}),
		named("Ring", Emitter{Sprite: Ring,
			Blend: gfx.BlendAdditive, Color: 0xffd8f0ff, EndColor: 0xff60c000, Scale: 0.3, EndScale: 3.5,
			Burst: 1, Duration: 2.2, Loop: true, Max: 2,
			LifeMin: 0.5,
		}),
		named("Pink shell", pink),
		named("Gold shell", gold),
		named("Glitter", Emitter{Sprite: Dot,
			Blend: gfx.BlendAdditive, Color: 0xffffffff, EndColor: 0xfff0b000, Scale: 0.04,
			FadeIn: 0.3, Burst: 60, Duration: 2.2, Loop: true, Max: 64,
			LifeMin: 1.6, LifeMax: 2.1, Spread: round, SpeedMin: 20, SpeedMax: 160,
			Gravity: gfx.Vec2{Y: 50}, Drag: 1.2,
		}),
	}
}

func thunderstorm() []Emitter {
	drizzle := rain
	drizzle.Color, drizzle.Scale, drizzle.Rate, drizzle.Max = 0x8aa8e060, 0.16, 60, 192
	drizzle.SpeedMin, drizzle.SpeedMax, drizzle.LifeMin, drizzle.LifeMax = 170, 210, 1.2, 1.5
	return []Emitter{
		named("Clouds", Emitter{Sprite: SoftCircle,
			Color: 0x4a4e5ce0, EndColor: 0x2a2e3a00, Scale: 1.1, EndScale: 1.6,
			FadeIn: 0.3, FadeOut: 0.3, Rate: 10, Max: 64,
			Shape: gfx.EmitRect, ShapeSize: gfx.Vec2{X: 280, Y: 20},
			LifeMin: 3, LifeMax: 5, Spread: 0.2, SpeedMin: 4, SpeedMax: 10, RandomRotation: true,
		}),
		named("Drizzle", drizzle),
		named("Rain", rain),
		// Two flashes with different lives flicker.
		named("Lightning", Emitter{Sprite: SoftCircle,
			Blend: gfx.BlendAdditive, Color: 0xd8e8ffff, EndColor: 0x8ab0ff00, Scale: 4, EndScale: 4.5,
			Burst: 2, Duration: 2.6, Loop: true, Max: 4,
			Shape: gfx.EmitRect, ShapeSize: gfx.Vec2{X: 120, Y: 10},
			LifeMin: 0.08, LifeMax: 0.25,
		}),
	}
}

func confetti() []Emitter {
	piece := Emitter{Sprite: Square,
		Scale: 0.08, FadeOut: 0.2, Burst: 30, Duration: 3, Loop: true, Max: 32,
		Shape: gfx.EmitRect, ShapeSize: gfx.Vec2{X: 12, Y: 2},
		LifeMin: 2.4, LifeMax: 3, Angle: up, Spread: 1, SpeedMin: 200, SpeedMax: 340,
		Gravity: gfx.Vec2{Y: 120}, Drag: 1.6, SpinMin: -8, SpinMax: 8, RandomRotation: true,
	}
	colored := func(name string, c gfx.Color) Emitter {
		e := piece
		e.Name, e.Color = name, c
		return e
	}
	streamers := piece
	streamers.Name, streamers.Sprite, streamers.Color, streamers.Scale = "Streamers", Streak, 0xc77dffff, 0.18
	streamers.Burst, streamers.SpinMin, streamers.SpinMax = 16, -10, 10
	return []Emitter{
		colored("Red", 0xff4d6dff),
		colored("Yellow", 0xffd23fff),
		colored("Green", 0x3ddc97ff),
		colored("Blue", 0x4d9bffff),
		streamers,
	}
}

func portal() []Emitter {
	return []Emitter{
		// Multiply darkens whatever is behind the portal.
		named("Void", Emitter{Sprite: SoftCircle,
			Blend: gfx.BlendMultiply, Color: 0x200040ff, Scale: 1.6,
			FadeIn: 0.5, FadeOut: 0.5, Rate: 6, Max: 16, LifeMin: 1,
		}),
		named("Glow", Emitter{Sprite: SoftCircle,
			Blend: gfx.BlendAdditive, Color: 0x8a3cffa0, EndColor: 0x3c1cff00, Scale: 1.2, EndScale: 1.6,
			FadeIn: 0.4, Rate: 4, Max: 8, LifeMin: 1.2,
		}),
		// Shrinking rings look like they fall into the portal.
		named("Rings", Emitter{Sprite: Ring,
			Blend: gfx.BlendAdditive, Color: 0xc9a0ffff, EndColor: 0x6a3cffff, Scale: 1.6, EndScale: 0.1,
			FadeIn: 0.3, Rate: 2.5, Max: 8, LifeMin: 1.4,
		}),
		named("Runes", Emitter{Sprite: Star,
			Blend: gfx.BlendAdditive, Color: 0xe0c0ffff, EndColor: 0x8050ff00, Scale: 0.1, EndScale: 0.05,
			FadeIn: 0.3, FadeOut: 0.4, Rate: 14, Max: 32,
			Shape: gfx.EmitCircle, ShapeSize: gfx.Vec2{X: 46},
			LifeMin: 1, LifeMax: 2, Spread: round, SpeedMin: 2, SpeedMax: 6,
			SpinMin: 3, SpinMax: 6, RandomRotation: true,
		}),
		named("Motes", Emitter{Sprite: Dot,
			Blend: gfx.BlendAdditive, Color: 0xffffffff, EndColor: 0xa070ff00, Scale: 0.05,
			Rate: 30, Max: 64,
			Shape: gfx.EmitCircle, ShapeSize: gfx.Vec2{X: 40},
			LifeMin: 0.8, LifeMax: 1.6, Angle: up, Spread: 0.6, SpeedMin: 10, SpeedMax: 30,
			Gravity: gfx.Vec2{Y: -20},
		}),
		named("Sparks", Emitter{Sprite: Spark,
			Blend: gfx.BlendAdditive, Color: 0xf0e0ffff, EndColor: 0x9050ff00, Scale: 0.12, EndScale: 0.02,
			Burst: 12, Duration: 0.7, Loop: true, Max: 32,
			LifeMin: 0.4, LifeMax: 0.7, Spread: round, SpeedMin: 40, SpeedMax: 90, Drag: 2, RandomRotation: true,
		}),
	}
}

// dragonBreath blows to the right; turn the entity to aim it.
func dragonBreath() []Emitter {
	return []Emitter{
		named("Smoke", Emitter{Sprite: SoftCircle,
			Color: 0x585058a0, EndColor: 0x30303000, Scale: 0.3, EndScale: 1.8,
			FadeIn: 0.4, Rate: 14, Max: 48,
			LifeMin: 1.2, LifeMax: 1.8, Spread: 0.25, SpeedMin: 120, SpeedMax: 180,
			Gravity: gfx.Vec2{Y: -30}, Drag: 1.6, RandomRotation: true,
		}),
		named("Outer flame", Emitter{Sprite: SoftCircle,
			Blend: gfx.BlendAdditive, Color: 0xff5a1aff, EndColor: 0x80100000, Scale: 0.2, EndScale: 1.4,
			Rate: 70, Max: 64,
			LifeMin: 0.5, LifeMax: 0.8, Spread: 0.3, SpeedMin: 180, SpeedMax: 240,
			Gravity: gfx.Vec2{Y: -40}, Drag: 1.8,
		}),
		named("Flame", Emitter{Sprite: SoftCircle,
			Blend: gfx.BlendAdditive, Color: 0xffb030ff, EndColor: 0xff300000, Scale: 0.15, EndScale: 0.6,
			Rate: 60, Max: 48,
			LifeMin: 0.3, LifeMax: 0.5, Spread: 0.14, SpeedMin: 200, SpeedMax: 260, Drag: 1.6,
		}),
		named("Core", Emitter{Sprite: SoftCircle,
			Blend: gfx.BlendAdditive, Color: 0xfffbe0ff, EndColor: 0xffc06000, Scale: 0.12, EndScale: 0.3,
			Rate: 50, Max: 16,
			LifeMin: 0.15, LifeMax: 0.25, Spread: 0.06, SpeedMin: 220, SpeedMax: 280,
		}),
		named("Embers", Emitter{Sprite: Dot,
			Blend: gfx.BlendAdditive, Color: 0xffd070ff, EndColor: 0xff400000, Scale: 0.05, EndScale: 0.02,
			Rate: 25, Max: 48,
			LifeMin: 0.6, LifeMax: 1.2, Spread: 0.5, SpeedMin: 100, SpeedMax: 260,
			Gravity: gfx.Vec2{Y: -20}, Drag: 1.2,
		}),
		named("Sparks", Emitter{Sprite: Spark,
			Blend: gfx.BlendAdditive, Color: 0xfff0c0ff, EndColor: 0xff802000, Scale: 0.08, EndScale: 0.02,
			Rate: 12, Max: 16,
			LifeMin: 0.3, LifeMax: 0.6, Spread: 0.7, SpeedMin: 150, SpeedMax: 300, Drag: 2, RandomRotation: true,
		}),
	}
}

// burningScraps shows the built-in Burn shader: paper scraps and smoke
// burn away along ragged edges instead of fading. With Burn the tint's
// alpha is the unburned part: Color is opaque and the alpha runs out by the
// end of the life.
func burningScraps() []Emitter {
	return []Emitter{
		named("Smoke", Emitter{Sprite: SoftCircle, Shader: Burn, Params: [4]float32{0.1, 0.2, 0.2, 0.24},
			Color: 0x55555ff0, EndColor: 0x30303800, Scale: 0.4, EndScale: 1.1,
			Rate: 6, Max: 24,
			Shape: gfx.EmitRect, ShapeSize: gfx.Vec2{X: 16, Y: 4},
			LifeMin: 2, LifeMax: 3, Angle: up, Spread: 0.4, SpeedMin: 16, SpeedMax: 28,
			Gravity: gfx.Vec2{Y: -6}, Drag: 0.4, SpinMin: -0.4, SpinMax: 0.4, RandomRotation: true,
		}),
		// The scrap chars from paper to brown while it burns.
		named("Scraps", Emitter{Sprite: Square, Shader: Burn, Params: [4]float32{0.2, 1, 0.54, 0.125},
			Color: 0xf0e4c8ff, EndColor: 0x5a463000, Scale: 0.3, EndScale: 0.24,
			Rate: 5, Max: 24,
			Shape: gfx.EmitRect, ShapeSize: gfx.Vec2{X: 24, Y: 4},
			LifeMin: 1.6, LifeMax: 2.4, Angle: up, Spread: 0.8, SpeedMin: 30, SpeedMax: 60,
			Gravity: gfx.Vec2{Y: -15}, Drag: 0.5, SpinMin: -2, SpinMax: 2, RandomRotation: true,
		}),
		named("Embers", Emitter{Sprite: Dot,
			Blend: gfx.BlendAdditive, Color: 0xffb050ff, EndColor: 0xff400000, Scale: 0.05, EndScale: 0.02,
			FadeOut: 0.3, Rate: 10, Max: 32,
			Shape: gfx.EmitRect, ShapeSize: gfx.Vec2{X: 24, Y: 4},
			LifeMin: 1, LifeMax: 2, Angle: up, Spread: 1, SpeedMin: 25, SpeedMax: 60,
			Gravity: gfx.Vec2{X: 4, Y: -10}, Drag: 0.4,
		}),
	}
}

// supernova uses every emitter an effect can hold: a star blows up every
// 4 s in front of a twinkling starfield and leaves a pulsar with two jets.
func supernova() []Emitter {
	jet := Emitter{Sprite: Streak,
		Blend: gfx.BlendAdditive, Color: 0xa0d8ffff, EndColor: 0x5a7aff00, Scale: 0.36, EndScale: 0.16,
		FadeIn: 0.1, Rate: 60, Max: 64,
		LifeMin: 0.5, LifeMax: 0.9, Angle: up, Spread: 0.06, SpeedMin: 140, SpeedMax: 200,
	}
	jetDown := jet
	jetDown.Angle = down
	return []Emitter{
		named("Starfield", Emitter{Layer: -2, Sprite: Dot,
			Blend: gfx.BlendAdditive, Color: 0xbfd8ffff, Scale: 0.04,
			FadeIn: 0.5, FadeOut: 0.5, Rate: 25, Max: 96,
			Shape: gfx.EmitCircle, ShapeSize: gfx.Vec2{X: 140},
			LifeMin: 1, LifeMax: 2.5, Spread: round, SpeedMax: 3,
		}),
		named("Nebula", Emitter{Layer: -1, Sprite: SoftCircle,
			Blend: gfx.BlendScreen, Color: 0x9a3cff40, EndColor: 0xff3c9a00, Scale: 0.6, EndScale: 2.4,
			FadeIn: 0.2, Burst: 36, Duration: 4, Loop: true, Max: 48,
			LifeMin: 2.5, LifeMax: 3.8, Spread: round, SpeedMin: 15, SpeedMax: 60,
			Drag: 0.6, SpinMin: -0.4, SpinMax: 0.4, RandomRotation: true,
		}),
		named("Flash", Emitter{Sprite: SoftCircle,
			Blend: gfx.BlendAdditive, Color: 0xffffffff, EndColor: 0x9fd8ff00, Scale: 1.5, EndScale: 7,
			Burst: 1, Duration: 4, Loop: true, Max: 2, LifeMin: 0.45,
		}),
		// Rings with different lives grow at different speeds: three nested
		// shockwaves from one burst.
		named("Shockwaves", Emitter{Sprite: Ring,
			Blend: gfx.BlendAdditive, Color: 0xbff0ffff, EndColor: 0x3c78ff00, Scale: 0.2, EndScale: 5,
			Burst: 3, Duration: 4, Loop: true, Max: 4, LifeMin: 0.9, LifeMax: 2.2,
		}),
		named("Ejecta", Emitter{Sprite: Spark,
			Blend: gfx.BlendAdditive, Color: 0xffe9b0ff, EndColor: 0xff402000, Scale: 0.22, EndScale: 0.04,
			Burst: 140, Duration: 4, Loop: true, Max: 160,
			LifeMin: 0.8, LifeMax: 1.6, Spread: round, SpeedMin: 120, SpeedMax: 280, Drag: 1.4, RandomRotation: true,
		}),
		named("Core", Emitter{Sprite: SoftCircle,
			Blend: gfx.BlendAdditive, Color: 0xd8f0ffff, EndColor: 0x6aa8ff00, Scale: 0.5, EndScale: 0.9,
			FadeIn: 0.3, Rate: 12, Max: 16, LifeMin: 0.5, LifeMax: 0.8,
		}),
		named("Jet up", jet),
		named("Jet down", jetDown),
	}
}

// PresetNames returns the names of Presets, for a dropdown.
func PresetNames() []string {
	ps := Presets()
	names := make([]string, len(ps))
	for i, p := range ps {
		names[i] = p.Name
		if p.Multi() {
			names[i] = fmt.Sprintf("%s (%d emitters)", p.Name, len(p.Emitters))
		}
	}
	return names
}

// presetFile is the file name Save as proposes for preset i.
func presetFile(i int) string {
	return strings.ReplaceAll(strings.ToLower(Presets()[i].Name), " ", "-") + fxfile.Suffix
}
