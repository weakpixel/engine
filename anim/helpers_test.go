package anim

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/weakpixel/asset"
	"github.com/weakpixel/engine/render"
)

// pngFile returns a w x h red PNG as a MapFS entry.
func pngFile(t testing.TB, w, h int) *fstest.MapFile {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.SetRGBA(x, y, color.RGBA{R: 255, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return &fstest.MapFile{Data: buf.Bytes()}
}

// asepriteHash is a hash-form export of n 4x4 frames with the given durations
// (ms, cycled) and the tags JSON ("" for none).
func asepriteHash(n int, durs []int, tags string) string {
	var sb strings.Builder
	sb.WriteString(`{"frames":{`)
	for i := range n {
		if i > 0 {
			sb.WriteString(",")
		}
		sb.WriteString(`"hero ` + itoa(i) + `.aseprite":{"frame":{"x":` + itoa(4*i) + `,"y":0,"w":4,"h":4},"duration":` + itoa(durs[i%len(durs)]) + `}`)
	}
	sb.WriteString(`},"meta":{"image":"hero.png"`)
	if tags != "" {
		sb.WriteString(`,"frameTags":` + tags)
	}
	sb.WriteString(`}}`)
	return sb.String()
}

func itoa(n int) string {
	var b strings.Builder
	if n >= 10 {
		b.WriteString(itoa(n / 10))
	}
	b.WriteByte(byte('0' + n%10))
	return b.String()
}

// env is a server with the render and anim loaders registered and a "hero"
// export of 6 frames in its filesystem (replace files["hero.aseprite.json"]
// before the first load to test other exports).
type env struct {
	t     testing.TB
	srv   *asset.Server
	files fstest.MapFS
}

func newEnv(t testing.TB, sheetJSON string) *env {
	t.Helper()
	files := fstest.MapFS{
		"chars/hero.aseprite.json": {Data: []byte(sheetJSON)},
		"chars/hero.png":           pngFile(t, 24, 4),
	}
	srv := asset.NewServer(asset.Options{FS: files})
	render.RegisterLoaders(srv, render.LoaderOptions{})
	RegisterLoaders(srv)
	t.Cleanup(srv.Close)
	return &env{t: t, srv: srv, files: files}
}

// settle runs Update until sc is done.
func (e *env) settle(sc *asset.Scope) {
	e.t.Helper()
	for range 5000 {
		e.srv.Update()
		if sc.Done() {
			return
		}
	}
	e.t.Fatal("scope never became Done")
}

func mustContain(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("error %v does not contain %q", err, want)
	}
}

func textFile(s string) *fstest.MapFile { return &fstest.MapFile{Data: []byte(s)} }
