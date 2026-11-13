//go:build ebitentest

package render

import (
	"testing"
	"testing/fstest"
	"time"
)

// kageTimeRed writes Time/4 into red, keeping green and blue of the source:
// the drawn red channel shows which Time the shader saw.
const kageTimeRed = `//kage:unit pixels

package main

var Time float

func Fragment(dst vec4, src vec2, color vec4) vec4 {
	c := imageSrc0At(src)
	c.r = Time / 4 * c.a
	return c
}
`

// TestDrawShaderTime checks that sprite shaders, group composites and post
// passes all see the Time set with SetTime, and that it is 0 before.
func TestDrawShaderTime(t *testing.T) {
	files := fstest.MapFS{"time.kage": textFile(kageTimeRed)}
	e := newAssetEnv(t, files, 0)
	sprite := e.loadSheet(files, "s", 1)[0]
	sc := e.srv.NewScope("fx")
	sh := LoadShader(sc, "time.kage")
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
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := NewRenderer(StoresOf(e.srv))
			q := &Queue{}
			tc.setup(r, q)
			q.Sort(nil, r.stores.Sprites)
			for _, step := range []struct {
				set  time.Duration
				want int
			}{{-1, 0}, {2 * time.Second, 128}, {4 * time.Second, 255}} {
				if step.set >= 0 {
					r.SetTime(step.set)
				}
				dst := newTarget(16, 16)
				r.Draw(dst, q, &Camera{}, 1)
				got := pixel(dst, 1, 1)
				if d := int(got.R) - step.want; d < -1 || d > 1 || got.A != 255 {
					t.Fatalf("Time %v: pixel = %v, want red about %d", step.set, got, step.want)
				}
			}
		})
	}
}
