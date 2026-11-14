package sheetfile

import (
	"image"
	"strings"
	"testing"
)

func TestDecodeCollectsProblems(t *testing.T) {
	_, err := Decode([]byte(`{"frameW":0,"frameH":-1,"margin":-2}`))
	if err == nil {
		t.Fatal("want an error")
	}
	for _, want := range []string{`"image" is required`, "must be positive", "must not be negative"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q lacks %q", err, want)
		}
	}
	if _, err := Decode([]byte(`{`)); err == nil {
		t.Error("broken JSON decoded")
	}
}

func TestEncodeRoundTrip(t *testing.T) {
	in := `{"image":"a.png","frameW":8,"frameH":4,"margin":1,"spacing":2,"pivot":{"x":0.5,"y":1}}`
	f, err := Decode([]byte(in))
	if err != nil {
		t.Fatal(err)
	}
	out, err := Encode(f)
	if err != nil {
		t.Fatal(err)
	}
	g, err := Decode(out)
	if err != nil || !f.Equal(g) {
		t.Fatalf("round trip: %v %s", err, out)
	}
	min, _ := Encode(&File{Image: "a.png", FrameW: 8, FrameH: 8})
	if strings.Contains(string(min), "margin") || !strings.HasSuffix(string(min), "}\n") {
		t.Errorf("zero margin written or no newline: %q", min)
	}
}

func TestLayout(t *testing.T) {
	tests := []struct {
		name       string
		f          File
		size       image.Point
		cols, rows int
		used       image.Rectangle
	}{
		{"plain", File{FrameW: 16, FrameH: 16}, image.Pt(64, 32), 4, 2, image.Rect(0, 0, 64, 32)},
		{"leftover", File{FrameW: 16, FrameH: 16}, image.Pt(70, 40), 4, 2, image.Rect(0, 0, 64, 32)},
		{"margin and spacing", File{FrameW: 8, FrameH: 8, Margin: 1, Spacing: 2}, image.Pt(32, 22), 3, 2, image.Rect(1, 1, 29, 19)},
		{"does not fit", File{FrameW: 16, FrameH: 16}, image.Pt(10, 40), 0, 0, image.Rectangle{}},
		{"margin eats it", File{FrameW: 8, FrameH: 8, Margin: 4}, image.Pt(12, 12), 0, 0, image.Rectangle{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := tt.f.Layout(tt.size)
			if g.Cols != tt.cols || g.Rows != tt.rows || g.Count() != tt.cols*tt.rows {
				t.Fatalf("grid %dx%d (%d), want %dx%d", g.Cols, g.Rows, g.Count(), tt.cols, tt.rows)
			}
			if got := g.Used(); got != tt.used {
				t.Errorf("used %v, want %v", got, tt.used)
			}
		})
	}
}

func TestAt(t *testing.T) {
	f := File{FrameW: 8, FrameH: 8, Margin: 1, Spacing: 2}
	g := f.Layout(image.Pt(32, 22))
	for i := range g.Count() {
		r := g.Rect(i)
		for _, p := range []image.Point{r.Min, r.Max.Sub(image.Pt(1, 1))} {
			if got, ok := g.At(p); !ok || got != i {
				t.Errorf("At(%v) = %d, %v; want %d", p, got, ok, i)
			}
		}
	}
	for _, p := range []image.Point{{0, 0}, {9, 1}, {10, 1}, {1, 9}, {31, 5}, {5, 21}, {-1, 3}} {
		if i, ok := g.At(p); ok {
			t.Errorf("At(%v) = %d, want no frame", p, i)
		}
	}
}

func TestStarter(t *testing.T) {
	if f := Starter("a.png", image.Pt(64, 8)); f.FrameW != 16 || f.FrameH != 8 {
		t.Errorf("starter %dx%d", f.FrameW, f.FrameH)
	}
	if f := Starter("a.png", image.Point{}); f.Validate() != nil {
		t.Error("starter of an unknown size is invalid")
	}
}
