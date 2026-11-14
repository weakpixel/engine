package tileset

import (
	"image"
	"os"
	"strings"
	"testing"
)

func TestAtlasExample(t *testing.T) {
	f, err := os.Open(writeSampleAtlas(t))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	a, err := ParseAtlas(f)
	if err != nil {
		t.Fatal(err)
	}
	if got := len(a.SubTextures); got != len(sampleSubTextures) {
		t.Errorf("sub-textures = %d, want %d", got, len(sampleSubTextures))
	}
	if got := CellSize(a); got != image.Pt(64, 64) {
		t.Errorf("cell size = %v, want 64x64", got)
	}
	if a.SubTextures[1].Name != "coin.png" || a.SubTextures[1].Rect != image.Rect(64, 0, 96, 32) {
		t.Errorf("second sub-texture = %+v", a.SubTextures[1])
	}
}

func TestAtlasErrors(t *testing.T) {
	tests := []struct {
		name, xml string
		want      []string
	}{
		{"bad xml", `<TextureAtlas`, []string{"invalid atlas XML"}},
		{"empty", `<TextureAtlas/>`, []string{"no SubTexture"}},
		{"size", `<TextureAtlas><SubTexture name="a" x="0" y="0" width="0" height="4"/></TextureAtlas>`, []string{`SubTexture "a": size 0x4`}},
		{"frame too small", `<TextureAtlas><SubTexture name="a" x="0" y="0" width="4" height="4" frameX="-1" frameY="0" frameWidth="4" frameHeight="4"/></TextureAtlas>`, []string{`SubTexture "a": frame 4x4`}},
		{"collects all", `<TextureAtlas>
			<SubTexture name="a" x="0" y="0" width="4" height="4"/>
			<SubTexture name="a" x="0" y="0" width="4" height="4"/>
			<SubTexture name="b" x="0" y="0" width="-1" height="4"/></TextureAtlas>`,
			[]string{`SubTexture "a": duplicate`, `SubTexture "b": size`}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseAtlas(strings.NewReader(tc.xml))
			if err == nil {
				t.Fatal("no error")
			}
			for _, w := range tc.want {
				if !strings.Contains(err.Error(), w) {
					t.Errorf("error %q does not contain %q", err, w)
				}
			}
		})
	}
}

func TestAtlasCheckBounds(t *testing.T) {
	a, err := ParseAtlas(strings.NewReader(`<TextureAtlas>
		<SubTexture name="in" x="0" y="0" width="4" height="4"/>
		<SubTexture name="out" x="6" y="0" width="4" height="4"/></TextureAtlas>`))
	if err != nil {
		t.Fatal(err)
	}
	err = a.CheckBounds(image.Rect(0, 0, 8, 8))
	if err == nil || !strings.Contains(err.Error(), `"out"`) || strings.Contains(err.Error(), `"in"`) {
		t.Errorf("err = %v, want only out", err)
	}
}

// ShoeBox leaves rotated off but swaps the frame size: the sub-texture is
// rotated, and its upright size is the frame.
func TestAtlasSwappedFrame(t *testing.T) {
	a, err := ParseAtlas(strings.NewReader(
		`<TextureAtlas><SubTexture name="v" x="0" y="0" width="64" height="10" frameX="-0" frameY="-0" frameWidth="10" frameHeight="64"/>
		<SubTexture name="r" x="0" y="0" width="6" height="4" rotated="true"/></TextureAtlas>`))
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range a.SubTextures {
		if !s.Rotated {
			t.Errorf("%s: not rotated", s.Name)
		}
	}
	if got := a.SubTextures[0].Frame; got != image.Pt(10, 64) {
		t.Errorf("frame = %v", got)
	}
	if got := a.SubTextures[1].Frame; got != image.Pt(4, 6) {
		t.Errorf("frame of r = %v, want the swapped rect", got)
	}
}
