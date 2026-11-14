//go:build ebitentest

package render

import (
	"testing"
	"testing/fstest"
)

// TestHotReloadShaderDraws checks that a reloaded shader is the one the next
// Draw runs, for a sprite, a layer group and a post pass, and that a broken
// save keeps drawing with the old one.
func TestHotReloadShaderDraws(t *testing.T) {
	files := fstest.MapFS{}
	save(files, "red.kage", textFile(kageSolid("1, 0, 0")), 1)
	e := newWatchEnv(t, files)
	sprite := e.loadSheet(files, "s", 1)[0]
	sc := e.srv.NewScope("fx")
	sh := LoadShader(sc, "red.kage")
	e.settle(sc)

	tests := []struct {
		name  string
		setup func(r *Renderer, q *Queue)
	}{
		{"sprite", func(r *Renderer, q *Queue) { put(q, sprite, 0, 0, 0).Shader = sh }},
		{"group", func(r *Renderer, q *Queue) {
			put(q, sprite, 0, 0, 0)
			var g GroupConfig
			g.Add(GroupOptions{From: 0, To: 0, Shader: sh})
			r.SetGroups(&g)
		}},
		{"post pass", func(r *Renderer, q *Queue) {
			put(q, sprite, 0, 0, 0)
			r.SetPost(&PostChain{Passes: []Pass{{Shader: sh}}})
		}},
	}
	n := 1
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := NewRenderer(StoresOf(e.srv))
			q := &Queue{}
			tc.setup(r, q)
			q.Sort(nil, r.stores.Sprites)
			draw := func() (red, green uint8) {
				dst := newTarget(16, 16)
				r.Draw(dst, q, &Camera{}, 1)
				p := pixel(dst, 1, 1)
				return p.R, p.G
			}
			// Each case starts from the previous case's source, so reset it.
			n++
			save(files, "red.kage", textFile(kageSolid("1, 0, 0")), n)
			e.srv.Update()
			if r, g := draw(); r != 255 || g != 0 {
				t.Fatalf("before: R=%d G=%d, want red", r, g)
			}
			n++
			save(files, "red.kage", textFile(kageSolid("0, 1, 0")), n)
			e.srv.Update()
			if r, g := draw(); r != 0 || g != 255 {
				t.Fatalf("after reload: R=%d G=%d, want green", r, g)
			}
			n++
			save(files, "red.kage", textFile("package main\nfunc Fragment( {"), n)
			e.srv.Update()
			if r, g := draw(); r != 0 || g != 255 {
				t.Fatalf("after broken save: R=%d G=%d, want green kept", r, g)
			}
		})
	}
}
