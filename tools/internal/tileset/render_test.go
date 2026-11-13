package tileset

import (
	"image"
	"image/color"
	"image/draw"
	"testing"
)

func TestRender(t *testing.T) {
	// Atlas: a red 4x4 at (0,0) and a green 2x2 at (4,0).
	src := image.NewNRGBA(image.Rect(0, 0, 8, 4))
	red, green := color.NRGBA{255, 0, 0, 255}, color.NRGBA{0, 255, 0, 255}
	draw.Draw(src, image.Rect(0, 0, 4, 4), image.NewUniform(red), image.Point{}, draw.Src)
	draw.Draw(src, image.Rect(4, 0, 6, 2), image.NewUniform(green), image.Point{}, draw.Src)
	a := mustAtlas(t, `<SubTexture name="r" x="0" y="0" width="4" height="4"/>
		<SubTexture name="g" x="4" y="0" width="2" height="2"/>`)
	l, err := NewLayout(a, image.Point{}, 2, 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	dst := Render(src, a, l, Center)
	if got, want := dst.Bounds().Size(), image.Pt(11, 6); got != want {
		t.Fatalf("size = %v, want %v", got, want)
	}
	tests := []struct {
		at   image.Point
		want color.NRGBA
	}{
		{image.Pt(1, 1), red},           // first cell, top-left pixel
		{image.Pt(4, 4), red},           // first cell, bottom-right
		{image.Pt(0, 0), color.NRGBA{}}, // margin
		{image.Pt(5, 1), color.NRGBA{}}, // spacing
		{image.Pt(6, 1), color.NRGBA{}}, // second cell padding (centered 2x2 in 4x4)
		{image.Pt(7, 2), green},         // second cell, centered at (2,2)
		{image.Pt(8, 3), green},
		{image.Pt(9, 4), color.NRGBA{}}, // padding right/bottom
	}
	for _, tc := range tests {
		if got := dst.NRGBAAt(tc.at.X, tc.at.Y); got != tc.want {
			t.Errorf("pixel %v = %v, want %v", tc.at, got, tc.want)
		}
	}
}

func TestRenderPaletted(t *testing.T) {
	pal := image.NewPaletted(image.Rect(0, 0, 2, 2), color.Palette{color.NRGBA{}, color.NRGBA{0, 0, 255, 255}})
	pal.SetColorIndex(0, 0, 1)
	a := mustAtlas(t, `<SubTexture name="p" x="0" y="0" width="2" height="2"/>`)
	l, _ := NewLayout(a, image.Point{}, 1, 0, 0)
	dst := Render(pal, a, l, TopLeft)
	if got := dst.NRGBAAt(0, 0); got != (color.NRGBA{0, 0, 255, 255}) {
		t.Errorf("pixel = %v", got)
	}
	if got := dst.NRGBAAt(1, 1); got.A != 0 {
		t.Errorf("transparent pixel = %v", got)
	}
}

func TestRenderRotated(t *testing.T) {
	// Stored rotated clockwise, a 2 wide, 3 high rect upright is 3x2: the
	// top-left stored pixel (red) ends up bottom-left.
	src := image.NewNRGBA(image.Rect(0, 0, 2, 3))
	red := color.NRGBA{255, 0, 0, 255}
	src.SetNRGBA(0, 0, red)
	a := mustAtlas(t, `<SubTexture name="r" x="0" y="0" width="2" height="3" rotated="true"/>`)
	l, err := NewLayout(a, image.Point{}, 1, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	dst := Render(src, a, l, TopLeft)
	if got := dst.Bounds().Size(); got != image.Pt(3, 2) {
		t.Fatalf("size = %v", got)
	}
	if got := dst.NRGBAAt(0, 1); got != red {
		t.Errorf("pixel (0,1) = %v, want red", got)
	}
	if got := dst.NRGBAAt(0, 0); got.A != 0 {
		t.Errorf("pixel (0,0) = %v, want transparent", got)
	}
}
