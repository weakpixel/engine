package anim

import (
	"slices"
	"testing"
	"time"

	"github.com/weakpixel/engine/gfx"
)

func TestShapesOf(t *testing.T) {
	e, set := mustLoadDef(t, defJSON(`"body":[{"x":-2,"y":-4,"w":4,"h":4}],
		"clips":{"idle":{"shapes":[{"kind":"hit","id":"jab","frames":{"from":1},"rect":{"x":1,"y":-3,"w":5,"h":2}}]}}`))
	clips := ClipStoreOf(e.srv)
	body := gfx.Box{Kind: gfx.BoxHurt, ID: "body", Rect: gfx.IRect{X: -2, Y: -4, W: 4, H: 4}}
	jab := gfx.Box{Kind: gfx.BoxHit, ID: "jab", Rect: gfx.IRect{X: 1, Y: -3, W: 5, H: 2}}
	idle := set.Clips["idle"]
	missing := LoadClip(e.srv.NewScope("m"), defKey+"#nope") // never Ready
	e.settle(e.srv.Global())
	plain, err := LoadClipSync(e.srv.Global(), "chars/hero.aseprite.json#idle")
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name string
		an   gfx.Animator
		want []gfx.Box
	}{
		{"frame 0", gfx.Animator{Clip: idle}, []gfx.Box{body}},
		{"frame 1", gfx.Animator{Clip: idle, Frame: 1}, []gfx.Box{body, jab}},
		{"frame out of range", gfx.Animator{Clip: idle, Frame: 5}, nil},
		{"zero handle", gfx.Animator{}, nil},
		{"clip not ready", gfx.Animator{Clip: missing}, nil},
		{"plain Aseprite clip", gfx.Animator{Clip: plain}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ShapesOf(clips, &tt.an); !slices.Equal(got, tt.want) {
				t.Fatalf("ShapesOf = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestShapesOfNoAllocs(t *testing.T) {
	e, set := mustLoadDef(t, defJSON(`"body":[{"x":-2,"y":-4,"w":4,"h":4}]`))
	clips := ClipStoreOf(e.srv)
	an := gfx.Animator{Clip: set.Clips["run"], Frame: 2, Time: 130 * time.Millisecond}
	var sink int
	if n := testing.AllocsPerRun(100, func() { sink += len(ShapesOf(clips, &an)) }); n != 0 {
		t.Fatalf("ShapesOf allocs/op = %v, want 0", n)
	}
	if sink == 0 {
		t.Fatal("no boxes returned")
	}
}
