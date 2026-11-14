package render

import (
	"image/color"
	"slices"
	"testing"
	"testing/fstest"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/weakpixel/engine/gfx"
)

func TestGroupConfig(t *testing.T) {
	var c GroupConfig
	a := c.Add(GroupOptions{From: 5, To: 6})
	b := c.Add(GroupOptions{From: -2, To: 0, Blend: BlendMultiply})
	one := c.Add(GroupOptions{From: 3, To: 3})
	if c.Len() != 3 || c.Options(b).Blend != BlendMultiply || c.Options(one).From != 3 {
		t.Fatalf("groups = %+v", c.groups)
	}
	if want := []GroupID{b, one, a}; !slices.Equal(c.order, want) {
		t.Fatalf("order = %v, want %v (by From)", c.order, want)
	}

	// Set may keep or move a group's own range; order follows From.
	c.Set(a, GroupOptions{From: 5, To: 6, Params: [4]float32{1}})
	c.Set(b, GroupOptions{From: 8, To: 9})
	if want := []GroupID{one, a, b}; !slices.Equal(c.order, want) {
		t.Fatalf("order after Set = %v, want %v", c.order, want)
	}
	if c.Options(a).Params[0] != 1 {
		t.Fatal("Set did not store Params")
	}

	mustPanic(t, "render: group From > To", func() { c.Add(GroupOptions{From: 2, To: 1}) })
	mustPanic(t, "render: group layers overlap", func() { c.Add(GroupOptions{From: 6, To: 7}) })
	mustPanic(t, "render: group layers overlap", func() { c.Set(one, GroupOptions{From: 0, To: 5}) })
	mustPanic(t, "render: unknown group", func() { c.Options(GroupID(7)) })
	mustPanic(t, "render: unknown group", func() { c.Set(-1, GroupOptions{}) })

	var nilConfig *GroupConfig
	if nilConfig.Len() != 0 {
		t.Fatal("nil GroupConfig has groups")
	}
}

func TestGroupSetParamsNoAllocs(t *testing.T) {
	if testing.CoverMode() != "" || raceEnabled {
		t.Skip("instrumentation allocates")
	}
	var c GroupConfig
	id := c.Add(GroupOptions{From: 1, To: 2, Clear: color.RGBA{40, 40, 40, 255}})
	opts := c.Options(id)
	if n := testing.AllocsPerRun(100, func() {
		opts.Params[0] += 0.1
		c.Set(id, opts)
	}); n != 0 {
		t.Fatalf("allocs per Set = %v, want 0", n)
	}
}

// groupScene is a queue with one 4x4 sprite in each of layers 0, 1, 2 and 5.
func groupScene(t testing.TB) (*Renderer, *Queue) {
	r, id := testRenderer(t)
	var q Queue
	for i, layer := range []int16{0, 1, 2, 5} {
		it := q.Sprite(layer, id, gfx.Vec2{X: float32(i * 4)}, gfx.Vec2{X: float32(i * 4)})
		it.Space, it.Origin = ScreenSpace, topLeft
	}
	q.Sort(nil, r.stores.Sprites)
	return r, &q
}

// TestDrawGroupsStats checks which groups are composited: those with items,
// and empty ones whose Clear color shows.
func TestDrawGroupsStats(t *testing.T) {
	r, q := groupScene(t)
	var g GroupConfig
	g.Add(GroupOptions{From: 1, To: 2})                                     // items in 1 and 2
	g.Add(GroupOptions{From: 3, To: 4, Clear: color.RGBA{10, 10, 10, 255}}) // empty, visible clear
	g.Add(GroupOptions{From: 7, To: 8})                                     // empty, transparent: skipped
	g.Add(GroupOptions{From: -9, To: -1, Clear: color.RGBA{A: 255}})        // empty, below every item
	r.SetGroups(&g)
	r.Draw(newTarget(32, 32), q, &Camera{}, 1)
	// Quads batch per target: layer 0 and layer 5 on the screen, layers 1
	// and 2 in the first group. Plus one composite per group.
	if st := r.Stats(); st.Items != 4 || st.Groups != 3 || st.Quads != 4 || st.DrawCalls != 3+3 {
		t.Fatalf("Stats = %+v, want 4 items, 3 groups, 4 quads, 6 draw calls", st)
	}

	r.SetGroups(nil)
	r.Draw(newTarget(32, 32), q, &Camera{}, 1)
	if st := r.Stats(); st.Groups != 0 || st.DrawCalls != 1 {
		t.Fatalf("Stats without groups = %+v", st)
	}
}

// TestDrawGroupsNoComposite checks that NoComposite groups are drawn into but
// never composited, empty with a visible Clear included.
func TestDrawGroupsNoComposite(t *testing.T) {
	r, q := groupScene(t)
	var g GroupConfig
	g.Add(GroupOptions{From: 1, To: 2, NoComposite: true})
	g.Add(GroupOptions{From: 3, To: 4, NoComposite: true, Clear: color.RGBA{10, 10, 10, 255}})
	r.SetGroups(&g)
	r.Draw(newTarget(32, 32), q, &Camera{}, 1)
	// Layer 0, the group and layer 5 are three batches, with no composite.
	if st := r.Stats(); st.Groups != 0 || st.Quads != 4 || st.DrawCalls != 3 {
		t.Fatalf("Stats = %+v, want 0 groups, 4 quads, 3 draw calls", st)
	}
}

// TestDrawGroupsNoAllocs checks that groups add no allocations of their own
// after the first frame: compared with the same Draw without groups, the
// extra allocations are Ebiten's own cost of the extra fills and draws.
func TestDrawGroupsNoAllocs(t *testing.T) {
	if testing.CoverMode() != "" || raceEnabled {
		t.Skip("instrumentation allocates")
	}
	files := fstest.MapFS{"fx/g.kage": textFile(kageFlash)}
	e := newAssetEnv(t, files, 0)
	sprite := e.loadSheet(files, "s", 1)[0]
	sc := e.srv.NewScope("fx")
	sh := LoadShader(sc, "fx/g.kage")
	e.settle(sc)
	r := NewRenderer(StoresOf(e.srv))
	var q Queue
	for i := range 300 {
		q.Sprite(int16(i%6), sprite, gfx.Vec2{X: float32(i % 50)}, gfx.Vec2{X: float32(i % 50)})
	}
	q.Sort(nil, r.stores.Sprites)
	dst := newTarget(64, 64)
	cam := &Camera{}

	r.Draw(dst, &q, cam, 1)
	plain := testing.AllocsPerRun(10, func() { r.Draw(dst, &q, cam, 1) })

	var g GroupConfig
	g.Add(GroupOptions{From: 1, To: 2})
	g.Add(GroupOptions{From: 3, To: 3, Shader: sh, SampleBelow: true, Clear: color.RGBA{20, 20, 20, 255}})
	g.Add(GroupOptions{From: 7, To: 7, Blend: BlendMultiply, Clear: color.RGBA{60, 60, 60, 255}})
	r.SetGroups(&g)
	r.Draw(dst, &q, cam, 1) // allocates the group images
	grouped := testing.AllocsPerRun(10, func() { r.Draw(dst, &q, cam, 1) })

	// Ebiten's own cost per call, outside a running game (a shader draw
	// costs one more than DrawImage, see TestDrawShadedNoAllocs).
	src := ebiten.NewImage(4, 4)
	var opts ebiten.DrawImageOptions
	dst.DrawImage(src, &opts)
	perCall := testing.AllocsPerRun(100, func() { dst.DrawImage(src, &opts) })
	fill := testing.AllocsPerRun(100, func() { src.Fill(color.Black) })

	// 3 composites + 1 below copy, 3 clears; slack as in TestDrawNoAllocs.
	const slack = 16
	if limit := plain + 4*(perCall+1) + 3*fill + slack; grouped > limit {
		t.Fatalf("allocs per Draw: %v with groups, %v without (limit %v)", grouped, plain, limit)
	}
	t.Logf("allocs per Draw: %v with groups, %v without; Ebiten per draw %v, per fill %v", grouped, plain, perCall, fill)
}
