package tileset

import (
	"bytes"
	"errors"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/weakpixel/engine/tiled"
	"github.com/weakpixel/engine/tools/internal/cli"
)

func TestFromAtlasExample(t *testing.T) {
	xmlPath := writeSampleAtlas(t)
	if err := FromAtlas(FromAtlasOptions{Atlases: []string{xmlPath}, Columns: 2}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	out := strings.TrimSuffix(xmlPath, ".xml") + ".tiles.png"
	f, err := os.Open(out)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		t.Fatal(err)
	}
	// 5 tiles of 64x64 in 2 columns: 3 rows.
	if got, want := img.Bounds().Size(), image.Pt(2*64, 3*64); got != want {
		t.Errorf("image size = %v, want %v", got, want)
	}
	data, err := os.ReadFile(strings.TrimSuffix(out, ".png") + ".tsj")
	if err != nil {
		t.Fatal(err)
	}
	ts, err := tiled.DecodeTileset(data)
	if err != nil {
		t.Fatal(err)
	}
	if ts.TileCount != 5 || ts.Columns != 2 || ts.TileWidth != 64 || ts.Image != "sheet.tiles.png" {
		t.Errorf("tileset = %+v", ts)
	}
}

func TestFromAtlasFailures(t *testing.T) {
	dir := t.TempDir()
	write := func(name, content string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	img := image.NewNRGBA(image.Rect(0, 0, 8, 8))
	var pngBuf bytes.Buffer
	if err := png.Encode(&pngBuf, img); err != nil {
		t.Fatal(err)
	}
	write("ok.png", pngBuf.String())
	big := write("ok.xml", `<TextureAtlas><SubTexture name="a" x="0" y="0" width="8" height="8"/></TextureAtlas>`)
	noImg := write("none.xml", `<TextureAtlas><SubTexture name="a" x="0" y="0" width="4" height="4"/></TextureAtlas>`)
	outside := write("out.xml", `<TextureAtlas imagePath="ok.png"><SubTexture name="o" x="6" y="6" width="4" height="4"/></TextureAtlas>`)

	tests := []struct {
		name     string
		opts     FromAtlasOptions
		wantCode int
		wantErr  string
	}{
		{"no atlas", FromAtlasOptions{}, 2, "one or more <atlas.xml>"},
		{"bad tile", FromAtlasOptions{Atlases: []string{big}, Tile: "7"}, 2, "not WxH"},
		{"bad align", FromAtlasOptions{Atlases: []string{big}, Align: "up"}, 2, "unknown alignment"},
		{"bad out", FromAtlasOptions{Atlases: []string{big}, Out: "x.jpg"}, 2, "--out must end in .png"},
		{"oversize", FromAtlasOptions{Atlases: []string{big}, Tile: "4x4"}, 1, `SubTexture "a": frame 8x8 is larger than the 4x4 tile`},
		{"missing image", FromAtlasOptions{Atlases: []string{noImg}}, 1, "atlas image not found"},
		{"outside image", FromAtlasOptions{Atlases: []string{outside}}, 1, `SubTexture "o"`},
		{"missing xml", FromAtlasOptions{Atlases: []string{filepath.Join(dir, "gone.xml")}}, 1, "gone.xml"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := FromAtlas(tc.opts, &bytes.Buffer{})
			if err == nil {
				t.Fatal("no error")
			}
			var u *cli.UsageError
			if got := errors.As(err, &u); got != (tc.wantCode == 2) {
				t.Errorf("usage error = %v, want exit code %d (%v)", got, tc.wantCode, err)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("error = %q, want it to contain %q", err, tc.wantErr)
			}
		})
	}
}

// The default alignment centers a frame smaller than the cell.
func TestFromAtlasDefaultCenters(t *testing.T) {
	dir := t.TempDir()
	src := image.NewNRGBA(image.Rect(0, 0, 4, 4))
	for i := range src.Pix {
		src.Pix[i] = 255
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, src); err != nil {
		t.Fatal(err)
	}
	xmlPath := filepath.Join(dir, "a.xml")
	xml := `<TextureAtlas imagePath="a.png"><SubTexture name="s" x="0" y="0" width="2" height="2"/></TextureAtlas>`
	for p, c := range map[string][]byte{xmlPath: []byte(xml), filepath.Join(dir, "a.png"): buf.Bytes()} {
		if err := os.WriteFile(p, c, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := FromAtlas(FromAtlasOptions{Atlases: []string{xmlPath}, Tile: "4x4"}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(filepath.Join(dir, "a.tiles.png"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, _, a := img.At(1, 1).RGBA(); a == 0 {
		t.Error("pixel (1,1) is transparent, want the centered sprite")
	}
	if _, _, _, a := img.At(0, 0).RGBA(); a != 0 {
		t.Error("pixel (0,0) is opaque, want padding")
	}
}

func TestFromAtlasMultiple(t *testing.T) {
	first := writeSampleAtlas(t)
	dir := filepath.Dir(first)
	second := filepath.Join(dir, "other.xml")
	if err := os.WriteFile(second, []byte(`<TextureAtlas imagePath="sheet.png"><SubTexture name="x" x="0" y="0" width="999999" height="2"/></TextureAtlas>`), 0o644); err != nil {
		t.Fatal(err)
	}
	// Quoted wildcard: expanded by the tool. other.xml fails, sheet.xml still converts.
	err := FromAtlas(FromAtlasOptions{Atlases: []string{filepath.Join(dir, "*.xml")}}, &bytes.Buffer{})
	var u *cli.UsageError
	if err == nil || errors.As(err, &u) || !strings.Contains(err.Error(), `SubTexture "x"`) {
		t.Errorf("error %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "sheet.tiles.tsj")); err != nil {
		t.Errorf("sheet.xml was not converted: %v", err)
	}
	if err := FromAtlas(FromAtlasOptions{Atlases: []string{first, second}, Out: "x.png"}, &bytes.Buffer{}); !errors.As(err, &u) {
		t.Errorf("--out with two atlases: %v", err)
	}
	if err := FromAtlas(FromAtlasOptions{Atlases: []string{filepath.Join(dir, "*.nope")}}, &bytes.Buffer{}); !errors.As(err, &u) {
		t.Errorf("empty pattern: %v", err)
	}
}
