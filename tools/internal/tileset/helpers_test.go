package tileset

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

// sampleSubTextures is a small packed atlas drawn in code: sub-textures of
// several sizes, the largest 64x64, so the cell size is 64x64.
var sampleSubTextures = []struct {
	name string
	rect image.Rectangle
}{
	{"block.png", image.Rect(0, 0, 64, 64)},
	{"coin.png", image.Rect(64, 0, 96, 32)},
	{"flag.png", image.Rect(96, 0, 128, 48)},
	{"plank.png", image.Rect(64, 48, 128, 64)},
	{"gem.png", image.Rect(128, 0, 144, 16)},
}

// writeSampleAtlas writes the sample atlas as sheet.xml (Starling/ShoeBox
// TextureAtlas) and sheet.png into a temp dir and returns the XML path.
func writeSampleAtlas(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	img := image.NewNRGBA(image.Rect(0, 0, 144, 64))
	var xml bytes.Buffer
	xml.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n" + `<TextureAtlas imagePath="sheet.png">` + "\n")
	for i, s := range sampleSubTextures {
		c := color.NRGBA{uint8(40 * (i + 1)), uint8(200 - 30*i), 128, 255}
		for y := s.rect.Min.Y; y < s.rect.Max.Y; y++ {
			for x := s.rect.Min.X; x < s.rect.Max.X; x++ {
				img.SetNRGBA(x, y, c)
			}
		}
		fmt.Fprintf(&xml, "\t<SubTexture name=%q x=\"%d\" y=\"%d\" width=\"%d\" height=\"%d\"/>\n",
			s.name, s.rect.Min.X, s.rect.Min.Y, s.rect.Dx(), s.rect.Dy())
	}
	xml.WriteString("</TextureAtlas>\n")
	var pngBuf bytes.Buffer
	if err := png.Encode(&pngBuf, img); err != nil {
		t.Fatal(err)
	}
	xmlPath := filepath.Join(dir, "sheet.xml")
	if err := os.WriteFile(xmlPath, xml.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "sheet.png"), pngBuf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	return xmlPath
}
