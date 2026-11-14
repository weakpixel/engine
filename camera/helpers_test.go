package camera

import (
	"math"
	"strings"
	"testing"

	"github.com/weakpixel/engine/gfx"
	"github.com/weakpixel/engine/xmath"
)

// mustPanic fails t unless f panics with a message containing want.
func mustPanic(t *testing.T, want string, f func()) {
	t.Helper()
	defer func() {
		t.Helper()
		r := recover()
		if r == nil {
			t.Fatalf("no panic, want %q", want)
		}
		msg, _ := r.(string)
		if err, ok := r.(error); ok {
			msg = err.Error()
		}
		if !strings.Contains(msg, want) {
			t.Fatalf("panic %q, want it to contain %q", msg, want)
		}
	}()
	f()
}

// near reports whether a and b differ by at most eps on both axes.
func near(a, b gfx.Vec2, eps float32) bool {
	return math.Abs(float64(a.X-b.X)) <= float64(eps) && math.Abs(float64(a.Y-b.Y)) <= float64(eps)
}

func vec(x, y float32) gfx.Vec2 { return gfx.Vec2{X: x, Y: y} }

// smoothStep is the exponential follow of the smooth mode.
func smoothStep(pos, goal gfx.Vec2, dt, tau float32) gfx.Vec2 {
	return pos.Lerp(goal, xmath.ExpFactor(dt, tau))
}

// springStep is the spring follow of the spring mode.
func springStep(pos, vel, goal gfx.Vec2, dt, freq, zeta float32) (gfx.Vec2, gfx.Vec2) {
	pos = pos.Spring(&vel, goal, dt, freq, zeta)
	return pos, vel
}
