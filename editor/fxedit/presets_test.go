package fxedit

import (
	"bytes"
	"image/png"
	"slices"
	"testing"
	"testing/fstest"
	"time"

	"github.com/weakpixel/asset"
	"github.com/weakpixel/ecs"
	"github.com/weakpixel/ecs/system"
	"github.com/weakpixel/engine/gfx"
	"github.com/weakpixel/engine/internal/shaderparam"
	"github.com/weakpixel/engine/particles"
	"github.com/weakpixel/engine/particles/fxfile"
	"github.com/weakpixel/engine/render"
)

func TestPresetsAreValidAndPlay(t *testing.T) {
	full := false
	for _, p := range Presets() {
		t.Run(p.Name, func(t *testing.T) {
			if n := len(p.Emitters); n < 1 || n > fxfile.MaxEmitters {
				t.Fatalf("%d emitters", n)
			}
			full = full || len(p.Emitters) == fxfile.MaxEmitters
			if _, err := fxfile.Decode(textOf(p.Emitters)); err != nil {
				t.Fatal(err)
			}
			for i, em := range p.Emitters {
				if p.Multi() && em.Name == "" {
					t.Errorf("emitter %d has no name", i)
				}
				if !IsBuiltin(em.Sprite) {
					t.Errorf("emitter %d: sprite %q is not built in", i, em.Sprite)
				}
				if em.Shader != "" && !IsBuiltin(em.Shader) {
					t.Errorf("emitter %d: shader %q is not built in", i, em.Shader)
				}
				// Each emitter plays on its own: it spawns within a second
				// and stays inside its pool.
				if peak := playPeak(t, []Emitter{em}, 60); peak == 0 || peak > capOf(em) {
					t.Errorf("emitter %d: peak live = %d, want 1..%d", i, peak, capOf(em))
				}
			}
		})
	}
	if !full {
		t.Errorf("no preset uses all %d emitters", fxfile.MaxEmitters)
	}
	names := PresetNames()
	if len(names) != len(Presets()) || names[DefaultPreset] != "Smoke" || names[len(names)-1] != "Supernova (8 emitters)" {
		t.Errorf("names = %v", names)
	}
	if got := presetFile(len(names) - 3); got != "dragon-breath.fx.json" {
		t.Errorf("presetFile = %q", got)
	}
}

func capOf(em Emitter) uint16 {
	if em.Max == 0 {
		return 256
	}
	return em.Max
}

// playPeak plays the effect for the given number of 60 Hz ticks and returns
// the most particles alive at once.
func playPeak(t *testing.T, ems []Emitter, ticks int) uint16 {
	t.Helper()
	files := fstest.MapFS{"e.fx.json": {Data: textOf(ems)}}
	srv := builtinServer(files)
	defer srv.Close()
	h, err := particles.LoadEffectSync(srv.Global(), "e.fx.json")
	if err != nil {
		t.Fatal(err)
	}
	w := ecs.NewWorld()
	ctx := &system.Context{World: w, Time: system.Time{Delta: time.Second / 60}}
	sys := particles.NewSystem(w, particles.EffectStoreOf(srv))
	e := ecs.NewView2[gfx.Transform, gfx.Particles](w).NewEntity(gfx.Transform{}, gfx.Particles{Effect: h})
	peak := uint16(0)
	for range ticks {
		sys.Run(ctx)
		peak = max(peak, ecs.GetComp[gfx.Particles](w, e).Live)
	}
	return peak
}

// builtinServer returns an asset server over files plus every built-in
// sprite and shader.
func builtinServer(files fstest.MapFS) *asset.Server {
	for _, k := range BuiltinKeys() {
		data, _ := BuiltinPNG(k)
		files[k] = &fstest.MapFile{Data: data}
	}
	for _, k := range BuiltinShaderKeys() {
		data, _ := BuiltinShader(k)
		files[k] = &fstest.MapFile{Data: data}
	}
	srv := asset.NewServer(asset.Options{FS: files})
	render.RegisterLoaders(srv, render.LoaderOptions{})
	particles.RegisterLoaders(srv)
	return srv
}

func TestBuiltinShaders(t *testing.T) {
	keys := BuiltinShaderKeys()
	if !slices.Contains(keys, Burn) {
		t.Fatalf("keys = %v", keys)
	}
	srv := builtinServer(fstest.MapFS{})
	defer srv.Close()
	for _, k := range keys {
		src, ok := BuiltinShader(k)
		if !ok {
			t.Fatalf("no source for %s", k)
		}
		if _, err := asset.LoadSync[render.Shader](srv.Global(), k); err != nil {
			t.Errorf("%s does not compile: %v", k, err)
		}
		if ps, err := shaderparam.Parse(src); err != nil || len(ps) == 0 {
			t.Errorf("%s markers: %v %v", k, ps, err)
		}
	}
	if _, ok := BuiltinShader(SoftCircle); ok {
		t.Error("a sprite is a shader")
	}
}

func TestBuiltinSprites(t *testing.T) {
	for _, key := range BuiltinKeys() {
		data, ok := BuiltinPNG(key)
		if !ok {
			t.Fatalf("no PNG for %s", key)
		}
		img, err := png.Decode(bytes.NewReader(data))
		if err != nil {
			t.Fatalf("%s: %v", key, err)
		}
		if b := img.Bounds(); b.Dx() != 64 || b.Dy() != 64 {
			t.Errorf("%s is %v, want 64x64", key, b)
		}
		// A shape on a transparent field: something mostly opaque, nothing in the
		// corners.
		opaque := false
		for y := range 64 {
			for x := range 64 {
				_, _, _, a := img.At(x, y).RGBA()
				opaque = opaque || a > 0x8000
			}
		}
		_, _, _, corner := img.At(0, 0).RGBA()
		if !opaque || corner != 0 {
			t.Errorf("%s: opaque=%v corner alpha=%d, want a shape on a transparent field", key, opaque, corner)
		}
	}
	if _, ok := BuiltinPNG("global/fx/x.png"); ok {
		t.Error("a project key has a built-in PNG")
	}
}

func TestExportBuiltin(t *testing.T) {
	in := Emitter{Sprite: SoftCircle}
	out, files := ExportBuiltin(in, "global/fx/smoke.fx.json")
	if out.Sprite != "global/fx/smoke.png" || len(files) != 1 || files[0].Path != "global/fx/smoke.png" {
		t.Fatalf("out=%q files=%v", out.Sprite, files)
	}
	if want, _ := BuiltinPNG(SoftCircle); !bytes.Equal(files[0].Data, want) {
		t.Error("exported bytes differ from the built-in PNG")
	}
	if IsBuiltin(out.Sprite) {
		t.Error("exported emitter still names fx-builtin/")
	}
	own, files := ExportBuiltin(Emitter{Sprite: "art/p.png"}, "global/fx/a.fx.json")
	if own.Sprite != "art/p.png" || files != nil {
		t.Errorf("project sprite changed: %q %v", own.Sprite, files)
	}
	if got := PNGKey("a.fx.json"); got != "a.png" {
		t.Errorf("PNGKey = %q", got)
	}
}

func TestExportAll(t *testing.T) {
	ems := []Emitter{
		{Sprite: SoftCircle, Shader: Burn},
		{Sprite: Square, Shader: Burn},
		{Sprite: SoftCircle, Shader: "fx/mine.kage"},
	}
	out, files := ExportAll(ems, "global/fx/burn.fx.json")
	want := []Emitter{
		{Sprite: "global/fx/burn.png", Shader: "global/fx/burn.kage"},
		{Sprite: "global/fx/burn-2.png", Shader: "global/fx/burn.kage"},
		{Sprite: "global/fx/burn.png", Shader: "fx/mine.kage"},
	}
	if !slices.Equal(out, want) {
		t.Errorf("out = %+v", out)
	}
	if got := fileKeys(files); !slices.Equal(got, []string{"global/fx/burn.png", "global/fx/burn.kage", "global/fx/burn-2.png"}) {
		t.Errorf("files = %v", got)
	}
	if src, _ := BuiltinShader(Burn); !bytes.Equal(files[1].Data, src) {
		t.Error("exported shader differs from the built-in source")
	}
	if ems[0].Sprite != SoftCircle {
		t.Error("ExportAll changed its input")
	}
}
