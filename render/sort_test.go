package render

import (
	"slices"
	"testing"
	"testing/fstest"

	"github.com/weakpixel/engine/gfx"
)

// order returns the Sprite handles of q in draw order.
func order(q *Queue) []gfx.SpriteHandle {
	var out []gfx.SpriteHandle
	for i := range q.Len() {
		out = append(out, q.At(i).Sprite)
	}
	return out
}

func TestQueueSort(t *testing.T) {
	ySorted := &LayerConfig{}
	ySorted.Set(1, LayerOptions{YSort: true})
	e, s := testSprites(t, 4) // s[0..3] share one texture
	h := func(i int) gfx.SpriteHandle { return s[i] }

	tests := []struct {
		name  string
		cfg   *LayerConfig
		items []Item
		want  []gfx.SpriteHandle
	}{
		{
			name:  "layer order",
			items: []Item{{Layer: 2, Sprite: h(0)}, {Layer: -1, Sprite: h(1)}, {Layer: 0, Sprite: h(2)}},
			want:  []gfx.SpriteHandle{h(1), h(2), h(0)},
		},
		{
			name:  "stable for equal keys",
			items: []Item{{Sprite: h(0)}, {Sprite: h(1)}, {Sprite: h(2)}, {Sprite: h(3)}},
			want:  []gfx.SpriteHandle{h(0), h(1), h(2), h(3)},
		},
		{
			name: "SortY only inside a YSort layer",
			cfg:  ySorted,
			items: []Item{
				{Layer: 1, SortY: 9, Sprite: h(0)}, {Layer: 1, SortY: 3, Sprite: h(1)},
				{Layer: 0, SortY: 9, Sprite: h(2)}, {Layer: 0, SortY: 3, Sprite: h(3)},
			},
			want: []gfx.SpriteHandle{h(2), h(3), h(1), h(0)},
		},
		{
			name: "nil config never Y-sorts",
			items: []Item{
				{Layer: 1, SortY: 9, Sprite: h(0)}, {Layer: 1, SortY: 3, Sprite: h(1)},
			},
			want: []gfx.SpriteHandle{h(0), h(1)},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var q Queue
			for _, it := range tc.items {
				q.Push(it)
			}
			q.Sort(tc.cfg, StoresOf(e.srv).Sprites)
			if got := order(&q); !slices.Equal(got, tc.want) {
				t.Fatalf("order = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestQueueSortTextureKey covers the batching key: sprites of one texture
// end up together, layers beat textures, and placeholders (zero, failed) share
// one key that sorts before every real texture.
func TestQueueSortTextureKey(t *testing.T) {
	files := fstest.MapFS{}
	e := newAssetEnv(t, files, 0)
	a := e.loadSheet(files, "a", 2)
	b := e.loadSheet(files, "b", 2)
	sc := e.srv.NewScope("bad")
	failed := LoadSprite(sc, "missing.png")
	e.settle(sc)
	st := StoresOf(e.srv).Sprites
	if ka, kb := st.Get(a[0]).Texture.Index(), st.Get(b[0]).Texture.Index(); ka == kb || ka == 0 || kb == 0 {
		t.Fatalf("texture slots %d, %d: want two distinct real textures", ka, kb)
	}
	first, second := a, b // texture slot order decides which group comes first
	if st.Get(b[0]).Texture.Index() < st.Get(a[0]).Texture.Index() {
		first, second = b, a
	}

	t.Run("texture groups a layer", func(t *testing.T) {
		var q Queue
		for _, h := range []gfx.SpriteHandle{second[0], first[0], second[1], first[1]} {
			q.Push(Item{Sprite: h})
		}
		q.Sort(nil, st)
		want := []gfx.SpriteHandle{first[0], first[1], second[0], second[1]}
		if got := order(&q); !slices.Equal(got, want) {
			t.Fatalf("order = %v, want %v", got, want)
		}
	})
	t.Run("layer beats texture", func(t *testing.T) {
		var q Queue
		q.Push(Item{Layer: 1, Sprite: first[0]})
		q.Push(Item{Layer: 0, Sprite: second[0]})
		q.Sort(nil, st)
		if got, want := order(&q), []gfx.SpriteHandle{second[0], first[0]}; !slices.Equal(got, want) {
			t.Fatalf("order = %v, want %v", got, want)
		}
	})
	t.Run("placeholders batch", func(t *testing.T) {
		var q Queue
		for _, h := range []gfx.SpriteHandle{first[0], {}, second[0], failed, first[1]} {
			q.Push(Item{Sprite: h})
		}
		q.Sort(nil, st)
		want := []gfx.SpriteHandle{{}, failed, first[0], first[1], second[0]}
		if got := order(&q); !slices.Equal(got, want) {
			t.Fatalf("order = %v, want %v", got, want)
		}
	})
	t.Run("nine-slices batch with sprites", func(t *testing.T) {
		var q Queue
		q.Push(Item{Sprite: second[0]})
		q.Push(Item{Kind: KindNineSlice, Sprite: first[0]})
		q.Push(Item{Sprite: first[1]})
		q.Sort(nil, st)
		if got, want := order(&q), []gfx.SpriteHandle{first[0], first[1], second[0]}; !slices.Equal(got, want) {
			t.Fatalf("order = %v, want %v", got, want)
		}
	})
	t.Run("nil store skips the texture key", func(t *testing.T) {
		var q Queue
		for _, h := range []gfx.SpriteHandle{second[0], first[0]} {
			q.Push(Item{Sprite: h})
		}
		q.Sort(nil, SpriteStore{})
		if got, want := order(&q), []gfx.SpriteHandle{second[0], first[0]}; !slices.Equal(got, want) {
			t.Fatalf("order = %v, want %v", got, want)
		}
	})
	t.Run("non-sprite items keep their order", func(t *testing.T) {
		var q Queue
		q.Push(Item{Kind: KindRect})
		q.Push(Item{Sprite: first[0]})
		q.Push(Item{Kind: KindText})
		q.Sort(nil, st)
		if q.At(0).Kind != KindRect || q.At(1).Kind != KindSprite || q.At(2).Kind != KindText {
			t.Fatal("non-sprite items moved")
		}
	})
}

func TestLayerConfig(t *testing.T) {
	var c LayerConfig
	for _, l := range []int16{-32768, -1, 0, 1, 32767} {
		if c.Options(l).YSort {
			t.Fatalf("layer %d YSort before Set", l)
		}
		c.Set(l, LayerOptions{YSort: true})
		if !c.Options(l).YSort {
			t.Fatalf("layer %d YSort lost", l)
		}
	}
	c.Set(0, LayerOptions{})
	if c.Options(0).YSort || !c.Options(1).YSort {
		t.Fatal("Set(false) must clear only that layer")
	}
}

// TestQueueSortShaderKey covers the shader part of the batching key: inside
// a layer, sprites group by shader before texture, unshaded sprites come
// first, and Y-sorting still beats the shader.
func TestQueueSortShaderKey(t *testing.T) {
	files := fstest.MapFS{"a.kage": textFile(kageOK), "b.kage": textFile(kageOK)}
	e := newAssetEnv(t, files, 0)
	s := e.loadSheet(files, "s", 2)
	sc := e.srv.NewScope("fx")
	a, b := LoadShader(sc, "a.kage"), LoadShader(sc, "b.kage")
	e.settle(sc)
	if a.Index() > b.Index() {
		a, b = b, a
	}
	st := StoresOf(e.srv).Sprites

	type key struct {
		sh gfx.ShaderHandle
		sp gfx.SpriteHandle
	}
	keys := func(q *Queue) []key {
		var out []key
		for i := range q.Len() {
			out = append(out, key{q.At(i).Shader, q.At(i).Sprite})
		}
		return out
	}

	t.Run("shader groups a layer", func(t *testing.T) {
		var q Queue
		for _, k := range []key{{b, s[0]}, {a, s[1]}, {gfx.ShaderHandle{}, s[0]}, {b, s[1]}, {a, s[0]}} {
			q.Push(Item{Shader: k.sh, Sprite: k.sp})
		}
		q.Sort(nil, st)
		want := []key{{gfx.ShaderHandle{}, s[0]}, {a, s[1]}, {a, s[0]}, {b, s[0]}, {b, s[1]}}
		if got := keys(&q); !slices.Equal(got, want) {
			t.Fatalf("order = %v, want %v", got, want)
		}
	})
	t.Run("Y-sort beats shader", func(t *testing.T) {
		cfg := &LayerConfig{}
		cfg.Set(0, LayerOptions{YSort: true})
		var q Queue
		q.Push(Item{SortY: 2, Shader: a, Sprite: s[0]})
		q.Push(Item{SortY: 1, Shader: b, Sprite: s[0]})
		q.Sort(cfg, st)
		if got, want := keys(&q), []key{{b, s[0]}, {a, s[0]}}; !slices.Equal(got, want) {
			t.Fatalf("order = %v, want %v", got, want)
		}
	})
}
