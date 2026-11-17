package render

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

func TestAddBloom(t *testing.T) {
	var p PostChain
	p.Passes = []Pass{{Off: true}} // something before the bloom
	b := p.AddBloom(BloomOptions{})
	if len(p.Targets) != 3 || p.Targets[0].Scale != 2 || p.Targets[1].Scale != 4 || p.Targets[2].Scale != 4 {
		t.Fatalf("Targets = %+v, want half, quarter, quarter", p.Targets)
	}
	if len(p.Passes) != 1+bloomPasses || b.first != 1 {
		t.Fatalf("%d passes, first %d, want %d passes from 1", len(p.Passes), b.first, 1+bloomPasses)
	}
	const half, quarter, scratch = 1, 2, 3
	want := []struct {
		src, out TargetID
		extra    TargetID
		params   [4]float32
	}{
		{TargetChain, half, 0, [4]float32{0.7}},
		{half, quarter, 0, [4]float32{}},
		{quarter, scratch, 0, [4]float32{1, 0}},
		{scratch, quarter, 0, [4]float32{0, 1}},
		{TargetChain, TargetChain, quarter, [4]float32{1}},
	}
	for i, w := range want {
		got := p.Passes[1+i]
		if got.builtin == 0 || got.Src != w.src || got.Out != w.out || got.Extra[0] != w.extra || got.Params != w.params {
			t.Errorf("pass %d = %+v, want src %d out %d extra %d params %v", i, got, w.src, w.out, w.extra, w.params)
		}
	}
}

func TestBloomSetAndOff(t *testing.T) {
	var p PostChain
	b := p.AddBloom(BloomOptions{})
	b.Set(&p, BloomOptions{Threshold: 0.5, Strength: 2, Radius: 3})
	if p.Passes[0].Params[0] != 0.5 || p.Passes[2].Params != [4]float32{3, 0} ||
		p.Passes[3].Params != [4]float32{0, 3} || p.Passes[4].Params[0] != 2 {
		t.Fatalf("Set: passes %+v", p.Passes)
	}
	if p.Passes[0].Src != TargetChain {
		t.Fatalf("Set: bright pass reads %d, want the chain", p.Passes[0].Src)
	}
	b.Set(&p, BloomOptions{Src: TargetGroup(2)})
	if p.Passes[0].Src != TargetGroup(2) || p.Passes[4].Src != TargetChain || p.Passes[4].Out != TargetChain {
		t.Fatalf("Set with Src: passes %+v, want the bright pass to read group 2 and the glow added to the chain", p.Passes)
	}
	b.SetOff(&p, true)
	for i, ps := range p.Passes {
		if !ps.Off {
			t.Fatalf("pass %d is on after SetOff(true)", i)
		}
	}
	b.SetOff(&p, false)
	if p.Passes[0].Off {
		t.Fatal("SetOff(false) left pass 0 off")
	}
}

// TestBuiltinShadersCompile checks that every shader the engine ships
// compiles; a failure would otherwise panic in the first Draw that uses it.
func TestBuiltinShadersCompile(t *testing.T) {
	r, _ := testRenderer(t)
	for b := builtinShader(1); b < builtinCount; b++ {
		if r.builtinShader(b) == nil {
			t.Errorf("built-in shader %d did not compile", b)
		}
	}
}

// TestDrawBloomNoAllocs checks that a bloom adds no allocations of its own
// after the first frame (as TestDrawPostTargetsNoAllocs).
func TestDrawBloomNoAllocs(t *testing.T) {
	if testing.CoverMode() != "" || raceEnabled {
		t.Skip("instrumentation allocates")
	}
	r, q, _ := postEnv(t)
	dst := newTarget(64, 64)
	cam := &Camera{}
	r.Draw(dst, q, cam, 1)
	plain := testing.AllocsPerRun(10, func() { r.Draw(dst, q, cam, 1) })

	var post PostChain
	b := post.AddBloom(BloomOptions{})
	r.SetPost(&post)
	r.Draw(dst, q, cam, 1) // compiles the shaders, allocates the images
	withPost := testing.AllocsPerRun(10, func() {
		b.Set(&post, BloomOptions{Strength: 1.5})
		r.Draw(dst, q, cam, 1)
	})
	if st := r.Stats(); st.PostPasses != bloomPasses {
		t.Fatalf("PostPasses = %d, want %d", st.PostPasses, bloomPasses)
	}

	src := ebiten.NewImage(4, 4)
	var opts ebiten.DrawImageOptions
	dst.DrawImage(src, &opts)
	perCall := testing.AllocsPerRun(100, func() { dst.DrawImage(src, &opts) })
	const slack = 16
	if limit := plain + 7*(perCall+1) + slack; withPost > limit {
		t.Fatalf("allocs per Draw: %v with bloom, %v without (limit %v)", withPost, plain, limit)
	}
}
