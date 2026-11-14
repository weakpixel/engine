package xmath

import "testing"

func TestScalarHelpers(t *testing.T) {
	checkF(t, "Sqrt(9)", Sqrt(9), 3)
	checkF(t, "Hypot(3,4)", Hypot(3, 4), 5)
	checkF(t, "Round(-2.5)", Round(-2.5), -3)
	checkF(t, "Round(2.5)", Round(2.5), 3)
	checkF(t, "Trunc(-1.7)", Trunc(-1.7), -1)
	checkF(t, "Mod(-1,4)", Mod(-1, 4), 3)
	checkF(t, "Mod(5,4)", Mod(5, 4), 1)
	checkF(t, "Mod(x,0)", Mod(5, 0), 0)
	checkF(t, "Sq(3)", Sq(3), 9)
	if s, c := Sincos(0); s != 0 || c != 1 {
		t.Errorf("Sincos(0) = %v, %v", s, c)
	}
}

func TestIntHelpers(t *testing.T) {
	tests := []struct {
		name string
		got  int
		want int
	}{
		{"FloorDiv(-1,16)", FloorDiv(-1, 16), -1},
		{"FloorDiv(-16,16)", FloorDiv(-16, 16), -1},
		{"FloorDiv(17,16)", FloorDiv(17, 16), 1},
		{"FloorDiv(1,-2)", FloorDiv(1, -2), -1},
		{"ModInt(-1,3)", ModInt(-1, 3), 2},
		{"ModInt(7,3)", ModInt(7, 3), 1},
		{"Abs(-3)", Abs(-3), 3},
		{"Sign(-9)", Sign(-9), -1},
		{"Sign(0)", Sign(0), 0},
		{"Clamp(15,0,10)", Clamp(15, 0, 10), 10},
		{"Approach(1,5,2)", Approach(1, 5, 2), 3},
		{"Approach(4,5,2)", Approach(4, 5, 2), 5},
		{"Approach(5,1,2)", Approach(5, 1, 2), 3},
	}
	for _, tt := range tests {
		if tt.got != tt.want {
			t.Errorf("%s = %d, want %d", tt.name, tt.got, tt.want)
		}
	}
}

func TestSignEpsAndFinite(t *testing.T) {
	if SignEps(0.1, 0.2) != 0 || SignEps(0.3, 0.2) != 1 || SignEps(-0.3, 0.2) != -1 {
		t.Error("SignEps dead zone")
	}
	var zero float32
	if IsFinite(1/zero) || IsFinite(zero/zero) || !IsFinite(1) {
		t.Error("IsFinite")
	}
	if !NearlyEqual(1, 1.05, 0.1) || NearlyEqual(1, 1.2, 0.1) {
		t.Error("NearlyEqual")
	}
}
