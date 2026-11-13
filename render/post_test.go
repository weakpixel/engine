package render

import (
	"image/color"
	"testing"
	"testing/fstest"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/weakpixel/engine/gfx"
)

// postEnv returns groupScene's renderer and queue plus a loaded shader for
// passes.
func postEnv(t testing.TB) (*Renderer, *Queue, gfx.ShaderHandle) {
	files := fstest.MapFS{"fx/p.kage": textFile(kageFlash)}
	e := newAssetEnv(t, files, 0)
	sprite := e.loadSheet(files, "s", 1)[0]
	sc := e.srv.NewScope("fx")
	sh := LoadShader(sc, "fx/p.kage")
	e.settle(sc)
	r := NewRenderer(StoresOf(e.srv))
	var q Queue
	for i := range 4 {
		it := q.Sprite(int16(i), sprite, gfx.Vec2{X: float32(i * 4)}, gfx.Vec2{X: float32(i * 4)})
		it.Space, it.Origin = ScreenSpace, topLeft
	}
	q.Sort(nil, r.stores.Sprites)
	return r, &q, sh
}

// TestDrawPostStats checks which passes run and which images the chain
// allocates.
func TestDrawPostStats(t *testing.T) {
	r, q, sh := postEnv(t)
	var missing gfx.ShaderHandle // zero: no shader
	tests := []struct {
		name   string
		passes []Pass
		runs   int
		images int // post images allocated
	}{
		{"no passes", nil, 0, 0},
		{"only off and unloaded", []Pass{{Shader: sh, Off: true}, {Shader: missing}}, 0, 0},
		{"one pass", []Pass{{Shader: sh}}, 1, 1},
		{"off skipped", []Pass{{Shader: sh}, {Shader: sh, Off: true}}, 1, 1},
		{"two passes", []Pass{{Shader: sh}, {Shader: missing}, {Shader: sh}}, 2, 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r.postImgs = [2]*ebiten.Image{}
			r.SetPost(&PostChain{Passes: tt.passes})
			r.Draw(newTarget(32, 32), q, &Camera{}, 1)
			st := r.Stats()
			if st.PostPasses != tt.runs || st.DrawCalls != 1+tt.runs || st.ShaderDraws != tt.runs {
				t.Fatalf("Stats = %+v, want %d passes", st, tt.runs)
			}
			images := 0
			for _, img := range r.postImgs {
				if img != nil {
					images++
				}
			}
			if images != tt.images {
				t.Fatalf("post images = %d, want %d", images, tt.images)
			}
		})
	}
}

// TestDrawPostFadeStats checks that PostChain.Fade costs one draw while its
// alpha is above zero and nothing otherwise, with and without passes.
func TestDrawPostFadeStats(t *testing.T) {
	r, q, sh := postEnv(t)
	for _, tt := range []struct {
		name   string
		chain  PostChain
		passes int
		fade   int
	}{
		{"zero", PostChain{}, 0, 0},
		{"transparent color", PostChain{Fade: gfx.RGBA(0, 0, 0, 0) | 0xff00}, 0, 0},
		{"fade only", PostChain{Fade: gfx.RGBA(0, 0, 0, 128)}, 0, 1},
		{"fade after a pass", PostChain{Passes: []Pass{{Shader: sh}}, Fade: gfx.Black}, 1, 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			r.SetPost(&tt.chain)
			r.Draw(newTarget(32, 32), q, &Camera{}, 1)
			st := r.Stats()
			if st.PostPasses != tt.passes || st.DrawCalls != 1+tt.passes+tt.fade {
				t.Fatalf("Stats = %+v, want %d passes and %d fade draws", st, tt.passes, tt.fade)
			}
		})
	}
}

// TestDrawFinalStats checks that DrawFinal runs the final pass only when it
// is set, on and loaded.
func TestDrawFinalStats(t *testing.T) {
	r, _, sh := postEnv(t)
	frame, window := newTarget(8, 8), newTarget(16, 16)
	var geoM ebiten.GeoM
	geoM.Scale(2, 2)
	for _, tt := range []struct {
		name  string
		final *Pass
		runs  int
	}{
		{"none", nil, 0},
		{"off", &Pass{Shader: sh, Off: true}, 0},
		{"unloaded", &Pass{}, 0},
		{"on", &Pass{Shader: sh}, 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			r.SetPost(&PostChain{Final: tt.final})
			r.stats = Stats{}
			r.DrawFinal(window, frame, geoM)
			if got := r.Stats().PostPasses; got != tt.runs {
				t.Fatalf("PostPasses = %d, want %d", got, tt.runs)
			}
		})
	}
}

// TestDrawPostNoAllocs checks that a running chain adds no allocations of
// its own after the first frame (as TestDrawGroupsNoAllocs for groups).
func TestDrawPostNoAllocs(t *testing.T) {
	if testing.CoverMode() != "" || raceEnabled {
		t.Skip("instrumentation allocates")
	}
	r, q, sh := postEnv(t)
	dst := newTarget(64, 64)
	cam := &Camera{}
	r.Draw(dst, q, cam, 1)
	plain := testing.AllocsPerRun(10, func() { r.Draw(dst, q, cam, 1) })

	post := &PostChain{Passes: []Pass{{Shader: sh}, {Shader: sh, Params: [4]float32{0.5}}, {Shader: sh}}}
	r.SetPost(post)
	r.Draw(dst, q, cam, 1) // allocates the post images
	withPost := testing.AllocsPerRun(10, func() {
		post.Passes[1].Params[0] += 0.01
		post.Fade = gfx.RGBA(0, 0, 0, uint8(post.Fade)+1)
		r.Draw(dst, q, cam, 1)
	})

	src := ebiten.NewImage(4, 4)
	var opts ebiten.DrawImageOptions
	dst.DrawImage(src, &opts)
	perCall := testing.AllocsPerRun(100, func() { dst.DrawImage(src, &opts) })
	// 1 copy + 3 shader passes + the fade; slack as in TestDrawNoAllocs.
	const slack = 16
	if limit := plain + 5*(perCall+1) + slack; withPost > limit {
		t.Fatalf("allocs per Draw: %v with post, %v without (limit %v)", withPost, plain, limit)
	}
	t.Logf("allocs per Draw: %v with post, %v without; Ebiten per draw %v", withPost, plain, perCall)
}

// TestPostTargetPanics checks the chain validation, which runs on every Draw
// over the running passes.
func TestPostTargetPanics(t *testing.T) {
	r, q, sh := postEnv(t)
	var groups GroupConfig
	groups.Add(GroupOptions{From: 8, To: 9})
	r.SetGroups(&groups)
	for _, tt := range []struct {
		name string
		pass Pass
		want string
	}{
		{"unknown src", Pass{Shader: sh, Src: 3}, "unknown post target 3 in pass 0"},
		{"unknown extra", Pass{Shader: sh, Extra: [3]TargetID{0, 0, 9}}, "unknown post target 9"},
		{"unknown out", Pass{Shader: sh, Out: 2}, "unknown post target 2"},
		{"reads and writes", Pass{Shader: sh, Extra: [3]TargetID{1}, Out: 1}, "pass 0 reads and writes target 1"},
		{"src is out", Pass{Shader: sh, Src: 1, Out: 1}, "reads and writes target 1"},
		{"writes frame", Pass{Shader: sh, Out: TargetFrame}, "pass 0 writes TargetFrame"},
		{"writes previous", Pass{Shader: sh, Out: TargetPrevious}, "pass 0 writes TargetPrevious"},
		{"unknown group", Pass{Shader: sh, Src: TargetGroup(1)}, "unknown post target -4 in pass 0"},
		{"unknown group extra", Pass{Shader: sh, Extra: [3]TargetID{TargetGroup(7)}}, "unknown post target -10"},
		{"writes group", Pass{Shader: sh, Out: TargetGroup(0)}, "pass 0 writes group 0"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			r.SetPost(&PostChain{Targets: []Target{{Scale: 2}}, Passes: []Pass{tt.pass}})
			mustPanic(t, tt.want, func() { r.Draw(newTarget(8, 8), q, &Camera{}, 1) })
		})
	}
	t.Run("group target without groups", func(t *testing.T) {
		r.SetGroups(nil)
		r.SetPost(&PostChain{Passes: []Pass{{Shader: sh, Src: TargetGroup(0)}}})
		mustPanic(t, "unknown post target -3", func() { r.Draw(newTarget(8, 8), q, &Camera{}, 1) })
	})
	t.Run("skipped passes are not checked", func(t *testing.T) {
		r.SetPost(&PostChain{Passes: []Pass{{Shader: sh, Out: 5, Off: true}}})
		r.Draw(newTarget(8, 8), q, &Camera{}, 1)
	})
}

// TestDrawPostTargetStats checks the draws of a chain with targets: passes
// that write a target add their draw, and a chain without a pass that writes
// the chain still copies the frame to the screen.
func TestDrawPostTargetStats(t *testing.T) {
	r, q, sh := postEnv(t)
	for _, tt := range []struct {
		name   string
		passes func(half, quarter TargetID) []Pass
		runs   int
		copies int // extra draws that are not passes
	}{
		{"only targets", func(h, _ TargetID) []Pass { return []Pass{{Shader: sh, Out: h}} }, 1, 1},
		{"target then chain", func(h, _ TargetID) []Pass {
			return []Pass{{Shader: sh, Out: h}, {Shader: sh, Extra: [3]TargetID{h}}}
		}, 2, 0},
		{"down and up", func(h, qu TargetID) []Pass {
			return []Pass{{Shader: sh, Out: h}, {Shader: sh, Src: h, Out: qu}, {Shader: sh, Extra: [3]TargetID{qu}}}
		}, 3, 0},
	} {
		t.Run(tt.name, func(t *testing.T) {
			post := &PostChain{}
			h, qu := post.AddTarget(Target{Scale: 2}), post.AddTarget(Target{Scale: 4})
			post.Passes = tt.passes(h, qu)
			r.SetPost(post)
			r.Draw(newTarget(32, 32), q, &Camera{}, 1)
			st := r.Stats()
			if st.PostPasses != tt.runs || st.ShaderDraws != tt.runs || st.DrawCalls != 1+tt.runs+tt.copies {
				t.Fatalf("Stats = %+v, want %d passes and %d copies", st, tt.runs, tt.copies)
			}
		})
	}
}

// TestDrawPostHistoryLifetime checks that the history images exist only
// while a running pass reads TargetPrevious.
func TestDrawPostHistoryLifetime(t *testing.T) {
	r, q, sh := postEnv(t)
	dst := newTarget(16, 16)
	post := &PostChain{Passes: []Pass{{Shader: sh}}}
	r.SetPost(post)
	r.Draw(dst, q, &Camera{}, 1)
	if r.hist[0] != nil || r.hist[1] != nil {
		t.Fatal("a chain without TargetPrevious allocated history")
	}
	post.Passes[0].Extra[0] = TargetPrevious
	r.Draw(dst, q, &Camera{}, 1)
	if r.hist[0] == nil || r.hist[1] == nil || !r.histOK {
		t.Fatal("TargetPrevious did not allocate history")
	}
	post.Passes[0].Off = true
	post.Passes = append(post.Passes, Pass{Shader: sh})
	r.Draw(dst, q, &Camera{}, 1)
	if r.hist[0] != nil || r.hist[1] != nil || r.histOK {
		t.Fatal("history was kept after no pass reads TargetPrevious")
	}
}

// TestDrawPostTargetsNoAllocs checks that a chain with targets, a frame
// input and the previous frame adds no allocations of its own after the
// first frame (as TestDrawPostNoAllocs).
func TestDrawPostTargetsNoAllocs(t *testing.T) {
	if testing.CoverMode() != "" || raceEnabled {
		t.Skip("instrumentation allocates")
	}
	r, q, sh := postEnv(t)
	dst := newTarget(64, 64)
	cam := &Camera{}
	r.Draw(dst, q, cam, 1)
	plain := testing.AllocsPerRun(10, func() { r.Draw(dst, q, cam, 1) })

	post := &PostChain{}
	half, quarter := post.AddTarget(Target{Scale: 2}), post.AddTarget(Target{Scale: 4})
	post.Passes = []Pass{
		{Shader: sh},
		{Shader: sh, Out: half},
		{Shader: sh, Src: half, Out: quarter},
		{Shader: sh, Params: [4]float32{0.5}},
		{Shader: sh, Extra: [3]TargetID{quarter, TargetFrame, TargetPrevious}},
	}
	r.SetPost(post)
	r.Draw(dst, q, cam, 1) // allocates the images
	withPost := testing.AllocsPerRun(10, func() {
		post.Passes[3].Params[0] += 0.01
		r.Draw(dst, q, cam, 1)
	})

	src := ebiten.NewImage(4, 4)
	var opts ebiten.DrawImageOptions
	dst.DrawImage(src, &opts)
	perCall := testing.AllocsPerRun(100, func() { dst.DrawImage(src, &opts) })
	// 1 copy + 5 passes + the history copy; slack as in TestDrawNoAllocs.
	const slack = 16
	if limit := plain + 8*(perCall+1) + slack; withPost > limit {
		t.Fatalf("allocs per Draw: %v with post, %v without (limit %v)", withPost, plain, limit)
	}
}

// TestTargetGroup checks the encoding and that groups are accepted as inputs.
func TestTargetGroup(t *testing.T) {
	for _, g := range []GroupID{0, 1, 5} {
		if got, ok := groupOf(TargetGroup(g)); !ok || got != g {
			t.Fatalf("groupOf(TargetGroup(%d)) = %d, %v", g, got, ok)
		}
	}
	for _, id := range []TargetID{TargetChain, TargetFrame, TargetPrevious, 3} {
		if _, ok := groupOf(id); ok {
			t.Fatalf("groupOf(%d) = group", id)
		}
	}
	r, q, sh := postEnv(t)
	var g GroupConfig
	a, b := g.Add(GroupOptions{From: 1, To: 2}), g.Add(GroupOptions{From: 3, To: 3, NoComposite: true})
	r.SetGroups(&g)
	r.SetPost(&PostChain{Passes: []Pass{{Shader: sh, Src: TargetGroup(a), Extra: [3]TargetID{TargetGroup(b)}}}})
	r.Draw(newTarget(8, 8), q, &Camera{}, 1)
}

// TestDrawPostGroupStats checks the draws of passes that read groups: a group
// with items is drawn once, an empty one is cleared on demand and not
// composited, and a NoComposite group is never composited.
func TestDrawPostGroupStats(t *testing.T) {
	r, q, sh := postEnv(t) // items in layers 0..3
	var g GroupConfig
	used := g.Add(GroupOptions{From: 1, To: 1, NoComposite: true})
	empty := g.Add(GroupOptions{From: 8, To: 9, NoComposite: true})
	r.SetGroups(&g)
	for _, tt := range []struct {
		name  string
		src   TargetID
		draws int
	}{
		{"group with items", TargetGroup(used), 1 + 1}, // copy + pass
		{"empty group", TargetGroup(empty), 1 + 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			r.SetPost(&PostChain{Passes: []Pass{{Shader: sh, Src: tt.src}}})
			r.Draw(newTarget(32, 32), q, &Camera{}, 1)
			if st := r.Stats(); st.Groups != 0 || st.PostPasses != 1 || st.DrawCalls != 3+tt.draws-1 && st.DrawCalls != 3+tt.draws {
				t.Fatalf("Stats = %+v", st)
			}
		})
	}
	// A visible Clear on a NoComposite group is begun for the pass only.
	g.Set(empty, GroupOptions{From: 8, To: 9, NoComposite: true, Clear: color.RGBA{A: 255}})
	r.SetPost(&PostChain{Passes: []Pass{{Shader: sh, Src: TargetGroup(empty)}}})
	r.Draw(newTarget(32, 32), q, &Camera{}, 1)
	if got := r.begun[empty]; got != r.frame {
		t.Fatalf("empty group begun in frame %d, want %d", got, r.frame)
	}
	if st := r.Stats(); st.Groups != 0 {
		t.Fatalf("Stats.Groups = %d, want 0", st.Groups)
	}
}

// TestDrawPostGroupInputNoAllocs checks that reading groups from passes adds
// no allocations of its own after the first frame.
func TestDrawPostGroupInputNoAllocs(t *testing.T) {
	if testing.CoverMode() != "" || raceEnabled {
		t.Skip("instrumentation allocates")
	}
	r, q, sh := postEnv(t)
	dst := newTarget(64, 64)
	cam := &Camera{}
	var g GroupConfig
	mask := g.Add(GroupOptions{From: 1, To: 1, NoComposite: true})
	g.Add(GroupOptions{From: 2, To: 2})
	empty := g.Add(GroupOptions{From: 8, To: 8, NoComposite: true})
	r.SetGroups(&g)
	post := &PostChain{Passes: []Pass{
		{Shader: sh, Extra: [3]TargetID{TargetGroup(mask), TargetGroup(empty)}},
	}}
	r.SetPost(post)
	r.Draw(dst, q, cam, 1)
	got := testing.AllocsPerRun(10, func() { r.Draw(dst, q, cam, 1) })

	post.Passes[0].Extra = [3]TargetID{}
	r.Draw(dst, q, cam, 1)
	base := testing.AllocsPerRun(10, func() { r.Draw(dst, q, cam, 1) })
	src := ebiten.NewImage(4, 4)
	var opts ebiten.DrawImageOptions
	dst.DrawImage(src, &opts)
	perCall := testing.AllocsPerRun(100, func() { dst.DrawImage(src, &opts) })
	// One extra group clear for the empty group; slack as elsewhere.
	if limit := base + 2*(perCall+1) + 16; got > limit {
		t.Fatalf("allocs per Draw: %v reading groups, %v without (limit %v)", got, base, limit)
	}
}
