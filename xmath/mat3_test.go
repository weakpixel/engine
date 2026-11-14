package xmath

import "testing"

func TestMat3(t *testing.T) {
	t.Run("identity and basics", func(t *testing.T) {
		p := V(3, 4)
		checkV(t, "Identity", Identity().Apply(p), p)
		checkV(t, "Translation", Translation(V(1, 2)).Apply(p), V(4, 6))
		checkV(t, "Scaling", Scaling(V(-1, 2)).Apply(p), V(-3, 8))
		checkV(t, "Rotation", Rotation(HalfPi).Apply(V(1, 0)), V(0, 1)) // clockwise on screen: +X turns to +Y
		checkV(t, "ApplyDir", Translation(V(9, 9)).ApplyDir(p), p)
	})
	t.Run("TRS matches the renderer order and Rotate", func(t *testing.T) {
		pos, rot, scale := V(10, -5), float32(0.7), V(2, -3)
		m := TRS(pos, rot, scale)
		for _, p := range []Vec2{{0, 0}, {1, 0}, {0, 1}, {4, -2}} {
			// Ebiten GeoM order: scale, rotate, translate.
			want := p.MulV(scale).Rotate(rot).Add(pos)
			checkV(t, "TRS", m.Apply(p), want)
		}
		chain := Translation(pos).Mul(Rotation(rot)).Mul(Scaling(scale))
		checkV(t, "chain", chain.Apply(V(4, -2)), m.Apply(V(4, -2)))
	})
	t.Run("Mul applies n first", func(t *testing.T) {
		m := Translation(V(10, 0)).Mul(Scaling(Splat(2)))
		checkV(t, "scale then move", m.Apply(V(1, 1)), V(12, 2))
		m = Scaling(Splat(2)).Mul(Translation(V(10, 0)))
		checkV(t, "move then scale", m.Apply(V(1, 1)), V(22, 2))
	})
	t.Run("Inverse round trip", func(t *testing.T) {
		m := TRS(V(7, -3), 1.2, V(2, 0.5))
		inv, ok := m.Inverse()
		if !ok {
			t.Fatal("not invertible")
		}
		for _, p := range []Vec2{{0, 0}, {5, 6}, {-3, 2}} {
			checkV(t, "round trip", inv.Apply(m.Apply(p)), p)
		}
		id := inv.Mul(m)
		if !near(id.A, 1, 1e-5) || !near(id.D, 1, 1e-5) || !near(id.TX, 0, 1e-4) || !near(id.TY, 0, 1e-4) {
			t.Errorf("inv*m = %+v, want identity", id)
		}
	})
	t.Run("singular", func(t *testing.T) {
		inv, ok := TRS(V(1, 2), 0.3, V(0, 1)).Inverse()
		if ok || inv != Identity() {
			t.Errorf("Inverse of scale 0 = %+v, %v; want Identity, false", inv, ok)
		}
		if _, ok := (Mat3{}).Inverse(); ok {
			t.Error("zero matrix is invertible")
		}
	})
	t.Run("ApplyRect", func(t *testing.T) {
		r := RectXYWH(0, 0, 4, 2)
		checkV(t, "translate min", Translation(V(1, 1)).ApplyRect(r).Min, V(1, 1))
		checkV(t, "translate max", Translation(V(1, 1)).ApplyRect(r).Max, V(5, 3))
		got := Rotation(HalfPi).ApplyRect(r) // 4x2 turned a quarter: 2x4
		checkV(t, "rot size", got.Size(), V(2, 4))
		checkV(t, "rot min", got.Min, V(-2, 0))
	})
}
