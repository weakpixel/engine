package xmath

import (
	"encoding/json"
	"testing"
)

func TestVec2Arithmetic(t *testing.T) {
	a, b := V(3, 4), V(1, 2)
	checkV(t, "Add", a.Add(b), V(4, 6))
	checkV(t, "Sub", a.Sub(b), V(2, 2))
	checkV(t, "Mul", a.Mul(2), V(6, 8))
	checkV(t, "Div", a.Div(2), V(1.5, 2))
	checkV(t, "MulV", a.MulV(b), V(3, 8))
	checkV(t, "DivV", a.DivV(b), V(3, 2))
	checkV(t, "Neg", a.Neg(), V(-3, -4))
	checkF(t, "Dot", a.Dot(b), 11)
	checkF(t, "Cross", a.Cross(b), 2)
	checkF(t, "Len", a.Len(), 5)
	checkF(t, "LenSq", a.LenSq(), 25)
	checkF(t, "Dist", a.Dist(V(0, 0)), 5)
	checkF(t, "DistSq", a.DistSq(V(0, 0)), 25)
	checkV(t, "Splat", Splat(2), V(2, 2))
	checkV(t, "Abs", V(-1, 2).Abs(), V(1, 2))
	checkV(t, "Min", a.Min(b), V(1, 2))
	checkV(t, "Max", a.Max(b), V(3, 4))
	checkV(t, "Clamp", V(5, -5).Clamp(V(0, 0), V(2, 2)), V(2, 0))
	checkV(t, "Floor", V(1.7, -1.2).Floor(), V(1, -2))
	checkV(t, "Round", V(1.5, -1.5).Round(), V(2, -2))
	checkV(t, "Snap", V(13, 5).Snap(V(8, 16)), V(16, 0))
	if x, y := a.XY(); x != 3 || y != 4 {
		t.Error("XY")
	}
	if !Zero.IsZero() || a.IsZero() {
		t.Error("IsZero")
	}
	if !a.NearlyEqual(V(3.01, 4), 0.1) || a.NearlyEqual(V(3.5, 4), 0.1) {
		t.Error("NearlyEqual")
	}
}

func TestVec2Direction(t *testing.T) {
	checkV(t, "Norm", V(0, 5).Norm(), V(0, 1))
	checkV(t, "Norm zero", Zero.Norm(), Zero)
	if _, ok := V(1e-9, 0).NormOk(); ok {
		t.Error("NormOk accepted a degenerate vector")
	}
	if n, ok := V(3, 4).NormOk(); !ok || !nearV(n, V(0.6, 0.8), 1e-6) {
		t.Errorf("NormOk = %v %v", n, ok)
	}
	checkV(t, "WithLen", V(3, 4).WithLen(10), V(6, 8))
	checkV(t, "WithLen zero", Zero.WithLen(10), Zero)
	checkV(t, "ClampLen long", V(30, 40).ClampLen(5), V(3, 4))
	checkV(t, "ClampLen short", V(3, 4).ClampLen(10), V(3, 4))
	checkV(t, "ClampLen negative limit", V(3, 4).ClampLen(-1), Zero)
	checkF(t, "Angle down", Down.Angle(), HalfPi)
	checkF(t, "AngleTo", Right.AngleTo(Down), HalfPi)
	checkF(t, "AngleTo back", Down.AngleTo(Right), -HalfPi)
	checkV(t, "FromAngle", FromAngle(HalfPi), Down)
	checkV(t, "FromPolar", FromPolar(Pi, 3), V(-3, 0))
}

func TestVec2Rotate(t *testing.T) {
	// Clockwise on screen: a quarter turn takes right to down, down to left.
	checkV(t, "Right", Right.Rotate(HalfPi), Down)
	checkV(t, "Down", Down.Rotate(HalfPi), Left)
	checkV(t, "Perp", Right.Perp(), Down)
	checkV(t, "Perp is Rotate(HalfPi)", V(2, 3).Perp(), V(2, 3).Rotate(HalfPi))
	checkF(t, "Rotate keeps length", V(3, 4).Rotate(1.234).Len(), 5)
}

func TestVec2Projection(t *testing.T) {
	checkV(t, "Project", V(3, 4).Project(Right), V(3, 0))
	checkV(t, "Project onto zero", V(3, 4).Project(Zero), Zero)
	checkV(t, "Reject", V(3, 4).Reject(Right), V(0, 4))
	checkV(t, "Reflect off floor", V(3, 4).Reflect(Up), V(3, -4))
}

func TestVec2Motion(t *testing.T) {
	checkV(t, "Lerp", V(0, 10).Lerp(V(10, 20), 0.5), V(5, 15))
	checkV(t, "Lerp end exact", V(0.1, 0).Lerp(V(0.7, 1), 1), V(0.7, 1))
	checkV(t, "Damp", V(0, 0).Damp(V(8, 4), 1, 1), V(4, 2))
	checkV(t, "Damp composition", V(0, 0).Damp(V(8, 4), 0.3, 0.2),
		V(0, 0).Damp(V(8, 4), 0.3, 0.1).Damp(V(8, 4), 0.3, 0.1))
	checkV(t, "Approach", V(0, 0).Approach(V(10, 0), 3), V(3, 0))
	checkV(t, "Approach arrives", V(9, 0).Approach(V(10, 0), 3), V(10, 0))

	var vel Vec2
	pos := V(0, 0)
	for range 600 {
		pos = pos.Spring(&vel, V(100, -50), 1.0/60, 2, 1)
	}
	if !nearV(pos, V(100, -50), 1e-3) {
		t.Errorf("Spring settles at %v", pos)
	}
	if vel.Len() > 0.01 {
		t.Errorf("Spring still moving at %v", vel)
	}
}

// TestVec2JSONShape pins the serialized form: GOD schemas and level files
// rely on a plain {"X":..,"Y":..} object.
func TestVec2JSONShape(t *testing.T) {
	b, err := json.Marshal(struct {
		P Vec2
		R Rect
	}{V(1, 2), RectXYWH(0, 0, 3, 4)})
	if err != nil {
		t.Fatal(err)
	}
	const want = `{"P":{"X":1,"Y":2},"R":{"Min":{"X":0,"Y":0},"Max":{"X":3,"Y":4}}}`
	if string(b) != want {
		t.Errorf("JSON = %s, want %s", b, want)
	}
}
