package animedit

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/weakpixel/engine/anim/animfile"
)

// heroDef is a definition of the fixture sheet: idle (frames 0..1) and run
// (2..5) tags, a pivot, a body, and a slash shape on run.
const heroDef = `{
  "version": 1,
  "sheet": "hero.aseprite.json",
  "pivot": {"x": 2, "y": 3},
  "defaultClip": "idle",
  "body": [{"x": -1, "y": -3, "w": 2, "h": 3}],
  "clips": {
    "run": {"loop": true, "fps": 10,
      "shapes": [
        {"kind": "hit", "id": "slash", "frames": {"from": 1, "to": 2}, "rect": {"x": 1, "y": -3, "w": 4, "h": 2}},
        {"kind": "hurt", "frames": {"from": 3}, "rect": {"x": -1, "y": -2, "w": 2, "h": 2}}
      ]}
  }
}`

const heroSheetJSON = `{"frames":{` +
	`"h 0":{"frame":{"x":0,"y":0,"w":4,"h":4},"duration":100},` +
	`"h 1":{"frame":{"x":4,"y":0,"w":4,"h":4},"duration":100},` +
	`"h 2":{"frame":{"x":8,"y":0,"w":4,"h":4},"duration":100},` +
	`"h 3":{"frame":{"x":12,"y":0,"w":4,"h":4},"duration":100},` +
	`"h 4":{"frame":{"x":16,"y":0,"w":4,"h":4},"duration":100},` +
	`"h 5":{"frame":{"x":20,"y":0,"w":4,"h":4},"duration":100}},` +
	`"meta":{"image":"hero.png","frameTags":[` +
	`{"name":"idle","from":0,"to":1,"direction":"forward"},` +
	`{"name":"run","from":2,"to":5,"direction":"forward"}]}}`

func heroFile(t testing.TB) *animfile.File {
	t.Helper()
	f, err := animfile.Decode([]byte(heroDef))
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func heroSheet(t testing.TB) *animfile.Sheet {
	t.Helper()
	s, err := animfile.DecodeSheet([]byte(heroSheetJSON))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// heroFS holds the fixture: a 6-frame sheet with its PNG and the definition.
func heroFS(t testing.TB) fstest.MapFS {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 24, 4))
	for y := range 4 {
		for x := range 24 {
			img.SetRGBA(x, y, color.RGBA{R: 255, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return fstest.MapFS{
		"chars/hero.aseprite.json": {Data: []byte(heroSheetJSON)},
		"chars/hero.ase.json":      {Data: []byte(heroSheetJSON)},
		"chars/hero.png":           {Data: buf.Bytes()},
		"chars/hero.anim.json":     {Data: []byte(strings.ReplaceAll(heroDef, `"hero.aseprite.json"`, `"hero.aseprite.json"`))},
	}
}

func encode(t testing.TB, f *animfile.File) string {
	t.Helper()
	b, err := animfile.Encode(f)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func itoa(n int) string { return strconv.Itoa(n) }

func ip(n int) *int { return &n }
