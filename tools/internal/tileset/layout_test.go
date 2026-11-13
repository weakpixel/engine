package tileset

import (
	"image"
	"strings"
	"testing"
)

func mustAtlas(t *testing.T, xml string) *Atlas {
	t.Helper()
	a, err := ParseAtlas(strings.NewReader("<TextureAtlas>" + xml + "</TextureAtlas>"))
	if err != nil {
		t.Fatal(err)
	}
	return a
}

const twoSizes = `<SubTexture name="a" x="0" y="0" width="4" height="4"/>
	<SubTexture name="b" x="4" y="0" width="2" height="2"/>
	<SubTexture name="c" x="0" y="4" width="2" height="2"/>`

func TestLayout(t *testing.T) {
	a := mustAtlas(t, twoSizes)
	l, err := NewLayout(a, image.Point{}, 2, 1, 2)
	if err != nil {
		t.Fatal(err)
	}
	if l.Tile != image.Pt(4, 4) || l.Columns != 2 || l.Rows() != 2 {
		t.Fatalf("layout = %+v", l)
	}
	if got, want := l.Size(), image.Pt(1+4+2+4+1, 1+4+2+4+1); got != want {
		t.Errorf("size = %v, want %v", got, want)
	}
	if got, want := l.Origin(2), image.Pt(1, 7); got != want {
		t.Errorf("origin(2) = %v, want %v", got, want)
	}
	if l, _ := NewLayout(a, image.Point{}, 16, 0, 0); l.Columns != 3 {
		t.Errorf("columns = %d, want capped at 3", l.Columns)
	}
}

func TestLayoutOversize(t *testing.T) {
	a := mustAtlas(t, twoSizes+`<SubTexture name="d" x="0" y="0" width="9" height="1"/>`)
	_, err := NewLayout(a, image.Pt(3, 3), 4, 0, 0)
	if err == nil {
		t.Fatal("no error")
	}
	for _, n := range []string{`"a"`, `"d"`} {
		if !strings.Contains(err.Error(), n) {
			t.Errorf("error %q misses %s", err, n)
		}
	}
	if strings.Contains(err.Error(), `"b"`) {
		t.Errorf("error %q lists b, which fits", err)
	}
}

func TestLayoutOffset(t *testing.T) {
	l := Layout{Tile: image.Pt(10, 8), Columns: 1, Count: 1}
	s := SubTexture{Frame: image.Pt(4, 2), Trim: image.Pt(1, 0)}
	tests := []struct {
		align Align
		want  image.Point
	}{
		{TopLeft, image.Pt(1, 0)},
		{Center, image.Pt(4, 3)},
		{BottomLeft, image.Pt(1, 6)},
	}
	for _, tc := range tests {
		if got := l.Offset(s, tc.align); got != tc.want {
			t.Errorf("align %d: offset = %v, want %v", tc.align, got, tc.want)
		}
	}
}
