package resolver_test

import (
	"bytes"
	"image"
	"image/png"
	"slices"
	"strings"
	"sync"
	"testing"
	"testing/fstest"

	"github.com/weakpixel/asset"
	"github.com/weakpixel/ecs"
	"github.com/weakpixel/engine/anim"
	"github.com/weakpixel/engine/assetkind"
	"github.com/weakpixel/engine/gfx"
	"github.com/weakpixel/engine/god"
	"github.com/weakpixel/engine/particles"
	prefabs "github.com/weakpixel/engine/prefab"
	"github.com/weakpixel/engine/render"
	"github.com/weakpixel/engine/resolver"
	"github.com/weakpixel/engine/sound"
)

const heroJSON = `{"frames":{
 "hero 0.aseprite":{"frame":{"x":0,"y":0,"w":4,"h":4},"duration":100},
 "hero 1.aseprite":{"frame":{"x":4,"y":0,"w":4,"h":4},"duration":100}},
 "meta":{"image":"hero.png","frameTags":[{"name":"idle","from":0,"to":1,"direction":"forward"}]}}`

type scene struct {
	srv   *asset.Server
	world *ecs.World
	res   *resolver.Resolver
}

func newScene(t *testing.T) *scene {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 8, 4))); err != nil {
		t.Fatal(err)
	}
	srv := asset.NewServer(asset.Options{FS: fstest.MapFS{
		"chars/hero.aseprite.json": {Data: []byte(heroJSON)},
		"chars/hero.png":           {Data: buf.Bytes()},
		"fx/flash.kage":            {Data: []byte(flashKage)},
		goblinKey:                  {Data: []byte(goblinPrefab)},
		"fx/sparks.fx.json":        {Data: []byte(`{"emitters":[{"rate":10}]}`)},
	}})
	t.Cleanup(srv.Close)
	render.RegisterLoaders(srv, render.LoaderOptions{})
	anim.RegisterLoaders(srv)
	sound.RegisterLoaders(srv, 0)
	particles.RegisterLoaders(srv)

	s := &scene{srv: srv, world: ecs.NewWorld(), res: resolver.New(srv)}
	resolver.Install(s.world, s.res)
	return s
}

const prefab = `{"objects":[{"group":"gfx","class":"AnimatedVisual","components":{` +
	`"gfx:Transform":{"Pos":{"X":1,"Y":2},"PrevPos":{"X":1,"Y":2},"Rot":0,"Scale":{"X":1,"Y":1}},` +
	`"gfx:Sprite":{"ID":"chars/hero.aseprite.json#hero 0.aseprite","Layer":2,"Origin":{"X":0.5,"Y":0.5},"FlipX":false,"FlipY":false,"Tint":0,"Blend":0,"Hidden":false},` +
	`"gfx:Animator":{"Clip":"chars/hero.aseprite.json#idle","Speed":2}}}]}`

const flashKage = `//kage:unit pixels

package main

var Params vec4

func Fragment(dst vec4, src vec2, color vec4) vec4 {
	return mix(imageSrc0At(src), vec4(1), Params.x) * color
}
`

// TestShaderOnLabelAndPanel checks that gfx:Label and gfx:NineSlice take
// Shader and Params from a document like gfx:Sprite.
func TestShaderOnLabelAndPanel(t *testing.T) {
	s := newScene(t)
	transform := `"gfx:Transform":{"Pos":{"X":1,"Y":2},"PrevPos":{"X":1,"Y":2},"Rot":0,"Scale":{"X":1,"Y":1}},`
	doc := `{"objects":[` +
		`{"group":"gfx","class":"Text","components":{` + transform +
		`"gfx:Label":{"Text":"hi","Layer":0,"Color":0,"Shader":"fx/flash.kage","Params":[1,0,0,0]}}},` +
		`{"group":"gfx","class":"Panel","components":{` + transform +
		`"gfx:NineSlice":{"Insets":{"Left":0,"Top":0,"Right":0,"Bottom":0},"Size":{"X":0,"Y":0},"Layer":0,` +
		`"Origin":{"X":0,"Y":0},"Tint":0,"Blend":0,"Tile":false,"Hidden":false,"Shader":"fx/flash.kage","Params":[0,1,0,0]}}}]}`
	objs, err := god.Deserialize(s.world, strings.NewReader(doc))
	if err != nil {
		t.Fatal(err)
	}
	l := ecs.GetComp[gfx.Label](s.world, objs[0].Entity)
	ns := ecs.GetComp[gfx.NineSlice](s.world, objs[1].Entity)
	if l.Shader.IsZero() || l.Params != [4]float32{1} || ns.Shader != l.Shader || ns.Params != [4]float32{0, 1} {
		t.Fatalf("label = %+v, panel = %+v", l, ns)
	}
	var out strings.Builder
	if err := god.Serialize(s.world, &out, objs...); err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(out.String(), `"Shader":"fx/flash.kage"`); n != 2 {
		t.Errorf("serialized document has %d shader keys, want 2:\n%s", n, out.String())
	}
}

// TestShaderRoundTrip checks the "shader" kind: Sprite.Shader is an asset
// key in GOD, Params an optional array that defaults to zeros and Aux an
// optional sprite key.
func TestShaderRoundTrip(t *testing.T) {
	s := newScene(t)
	doc := strings.Replace(prefab, `"Hidden":false`, `"Hidden":false,"Shader":"fx/flash.kage","Params":[1,0,0,0.5],"Aux":"fx/noise.png"`, 1)
	objs, err := god.Deserialize(s.world, strings.NewReader(doc))
	if err != nil {
		t.Fatal(err)
	}
	sprite := ecs.GetComp[gfx.Sprite](s.world, objs[0].Entity)
	if sprite.Shader.IsZero() || sprite.Params != [4]float32{1, 0, 0, 0.5} || sprite.Aux.IsZero() || sprite.Aux == sprite.ID {
		t.Fatalf("sprite = %+v", sprite)
	}
	var out strings.Builder
	if err := god.Serialize(s.world, &out, objs...); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"Shader":"fx/flash.kage"`, `"Params":[1,0,0,0.5]`, `"Aux":"fx/noise.png"`} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("serialized prefab lacks %s:\n%s", want, out.String())
		}
	}
	if _, err := s.res.AssetKey(render.AssetKindShader, gfx.SpriteHandle{}); err == nil {
		t.Error("AssetKey accepted a sprite handle for the shader kind")
	}

	// Without Shader, Params and Aux (the plain prefab) the sprite is
	// unshaded.
	objs, err = god.Deserialize(s.world, strings.NewReader(prefab))
	if err != nil {
		t.Fatal(err)
	}
	if sp := ecs.GetComp[gfx.Sprite](s.world, objs[0].Entity); !sp.Shader.IsZero() || sp.Params != [4]float32{} || !sp.Aux.IsZero() {
		t.Fatalf("plain prefab sprite = %+v", sp)
	}
}

// TestPrefabRoundTrip deserializes a prefab with asset keys, checks the
// handles and serializes it back to the same keys.
func TestPrefabRoundTrip(t *testing.T) {
	s := newScene(t)
	objs, err := god.Deserialize(s.world, strings.NewReader(prefab))
	if err != nil {
		t.Fatal(err)
	}
	e := objs[0].Entity
	sprite, animator := ecs.GetComp[gfx.Sprite](s.world, e), ecs.GetComp[gfx.Animator](s.world, e)
	if sprite.ID.IsZero() || animator.Clip.IsZero() || animator.Speed != 2 || sprite.Layer != 2 {
		t.Fatalf("components = %+v %+v", sprite, animator)
	}

	var out strings.Builder
	if err := god.Serialize(s.world, &out, objs...); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`"ID":"chars/hero.aseprite.json#hero 0.aseprite"`,
		`"Clip":"chars/hero.aseprite.json#idle"`,
		`"Speed":2`,
	} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("serialized prefab lacks %s:\n%s", want, out.String())
		}
	}
	for _, runtime := range []string{`"Time"`, `"Frame"`, `"Done"`} {
		if strings.Contains(out.String(), runtime) {
			t.Errorf("serialized prefab contains runtime field %s", runtime)
		}
	}

	// The loaded art plays: the clip resolves once the scope is done.
	if _, err := anim.LoadClipSync(s.srv.Global(), "chars/hero.aseprite.json#idle"); err != nil {
		t.Fatal(err)
	}
}

func TestRuntimeFieldRejectedInInput(t *testing.T) {
	s := newScene(t)
	doc := strings.Replace(prefab, `"Speed":2`, `"Speed":2,"Time":5`, 1)
	_, err := god.Deserialize(s.world, strings.NewReader(doc))
	if err == nil || !strings.Contains(err.Error(), "$.Time: runtime field") {
		t.Fatalf("got %v", err)
	}
}

func TestResolveErrors(t *testing.T) {
	s := newScene(t)
	for _, tc := range []struct{ kind, path, want string }{
		{"sprite", "../escape.png", "bad key"},
		{"sprite", "", "bad key"},
		{"nope", "a.png", `unknown asset kind "nope"`},
	} {
		if _, err := s.res.ResolveAsset(tc.kind, tc.path); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("ResolveAsset(%q, %q) = %v, want error containing %q", tc.kind, tc.path, err, tc.want)
		}
	}
	if _, err := s.res.AssetKey("sprite", gfx.SpriteHandle{}); err == nil || !strings.Contains(err.Error(), "stale") {
		t.Errorf("AssetKey of a zero handle = %v", err)
	}
	if _, err := s.res.AssetKey("sprite", 3); err == nil {
		t.Error("AssetKey accepted a non-handle")
	}
}

// TestSetScope checks that assets resolved while a level scope is set are
// held by it, so releasing the scope frees them.
func TestSetScope(t *testing.T) {
	s := newScene(t)
	level := s.srv.NewScope("level")
	s.res.SetScope(level)
	if s.res.Scope() != level {
		t.Fatal("Scope() != level")
	}
	h, err := s.res.ResolveAsset("sprite", "chars/hero.aseprite.json#hero 1.aseprite")
	if err != nil {
		t.Fatal(err)
	}
	for range 5000 {
		s.srv.Update()
		if level.Done() {
			break
		}
	}
	key, err := s.res.AssetKey("sprite", h)
	if err != nil || key != "chars/hero.aseprite.json#hero 1.aseprite" {
		t.Fatalf("AssetKey = %q, %v", key, err)
	}
	level.Release()
	s.srv.Update()
	if _, err := s.res.AssetKey("sprite", h); err == nil {
		t.Fatal("handle still valid after the level scope was released")
	}
	s.res.SetScope(nil)
	if s.res.Scope() != s.srv.Global() {
		t.Fatal("nil scope must mean global")
	}
}

// TestTextFontKey checks that Label.Font round-trips as a "font" asset key.
func TestTextFontKey(t *testing.T) {
	s := newScene(t)
	doc := `{"objects":[{"group":"gfx","class":"Text","components":{` +
		`"gfx:Transform":{"Pos":{"X":4,"Y":5},"PrevPos":{"X":4,"Y":5},"Rot":0,"Scale":{"X":1,"Y":1}},` +
		`"gfx:Label":{"Text":"hi","Font":"fonts/ui.json","Layer":1,"Color":0}}}]}`
	objs, err := god.Deserialize(s.world, strings.NewReader(doc))
	if err != nil {
		t.Fatal(err)
	}
	if ecs.GetComp[gfx.Label](s.world, objs[0].Entity).Font.IsZero() {
		t.Fatal("Font handle not resolved")
	}
	var out strings.Builder
	if err := god.Serialize(s.world, &out, objs...); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"Font":"fonts/ui.json"`) {
		t.Fatalf("serialized: %s", out.String())
	}
}

// TestSoundSourceKeys checks that SoundEmitter.Sound round-trips as a "sound"
// asset key and that its runtime fields stay out of the data.
func TestSoundSourceKeys(t *testing.T) {
	s := newScene(t)
	doc := `{"objects":[{"group":"gfx","class":"SoundSource","components":{` +
		`"gfx:Transform":{"Pos":{"X":4,"Y":5},"PrevPos":{"X":4,"Y":5},"Rot":0,"Scale":{"X":1,"Y":1}},` +
		`"gfx:SoundEmitter":{"Sound":"sfx/waterfall.ogg","Volume":0.5,"Bus":0,"Loop":true}}}]}`
	objs, err := god.Deserialize(s.world, strings.NewReader(doc))
	if err != nil {
		t.Fatal(err)
	}
	em := ecs.GetComp[gfx.SoundEmitter](s.world, objs[0].Entity)
	if em.Sound.IsZero() || !em.Loop || em.Volume != 0.5 {
		t.Fatalf("emitter = %+v", *em)
	}
	em.Voice, em.Done = 7, true // runtime state must not be written
	var out strings.Builder
	if err := god.Serialize(s.world, &out, objs...); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"Sound":"sfx/waterfall.ogg"`) {
		t.Fatalf("serialized: %s", out.String())
	}
	for _, runtime := range []string{`"Voice"`, `"Done"`} {
		if strings.Contains(out.String(), runtime) {
			t.Errorf("serialized emitter contains runtime field %s", runtime)
		}
	}
}

// TestParticlesEffectKey checks that Particles.Effect round-trips as an
// "effect" asset key and that its runtime fields stay out of the data.
func TestParticlesEffectKey(t *testing.T) {
	s := newScene(t)
	doc := `{"objects":[{"group":"gfx","class":"ParticleSource","components":{` +
		`"gfx:Transform":{"Pos":{"X":4,"Y":5},"PrevPos":{"X":4,"Y":5},"Rot":0,"Scale":{"X":1,"Y":1}},` +
		`"gfx:Particles":{"Effect":"fx/sparks.fx.json","Layer":3,"Stop":false,"Seed":9}}}]}`
	objs, err := god.Deserialize(s.world, strings.NewReader(doc))
	if err != nil {
		t.Fatal(err)
	}
	c := ecs.GetComp[gfx.Particles](s.world, objs[0].Entity)
	if c.Effect.IsZero() || c.Layer != 3 || c.Seed != 9 {
		t.Fatalf("component = %+v", *c)
	}
	h, err := particles.LoadEffectSync(s.srv.Global(), "fx/sparks.fx.json")
	if err != nil || h != c.Effect {
		t.Fatalf("the level's effect %v is not the loaded one %v (%v)", c.Effect, h, err)
	}
	c.Pool, c.Live = 3, 4
	var out strings.Builder
	if err := god.Serialize(s.world, &out, objs...); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"Effect":"fx/sparks.fx.json"`) {
		t.Fatalf("serialized: %s", out.String())
	}
	for _, runtime := range []string{`"Pool"`, `"Live"`, `"Time"`, `"Done"`} {
		if strings.Contains(out.String(), runtime) {
			t.Errorf("serialized component contains runtime field %s", runtime)
		}
	}
}

// TestMusicKind checks the "music" kind, which no gfx component uses but game
// components can.
func TestMusicKind(t *testing.T) {
	s := newScene(t)
	h, err := s.res.ResolveAsset(sound.AssetKindMusic, "music/level1.ogg")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := h.(gfx.MusicHandle); !ok {
		t.Fatalf("ResolveAsset returned %T", h)
	}
	if key, err := s.res.AssetKey(sound.AssetKindMusic, h); err != nil || key != "music/level1.ogg" {
		t.Fatalf("AssetKey = %q, %v", key, err)
	}
	if _, err := s.res.AssetKey(sound.AssetKindSound, h); err == nil {
		t.Error("AssetKey accepted a music handle for the sound kind")
	}
}

// spawner stands in for a game's component with a prefab field.
type spawner struct {
	Prefab gfx.PrefabHandle `json:"-"`
	Every  int
}

var registerSpawner = sync.OnceFunc(func() {
	god.RegisterSchema(god.Schema{
		SchemaMeta: god.SchemaMeta{Group: "t", Class: "Spawner"},
		Components: []god.Component{{
			Group:     "t",
			Name:      "Spawner",
			RuntimeID: ecs.RegisterComp[spawner](),
			Decode:    god.DecodeFunc[spawner](),
			Schema: &god.TypeSchema{
				Type: "object",
				Properties: map[string]*god.TypeSchema{
					"Prefab": {Type: "string", Format: "asset", XAsset: prefabs.AssetKind},
					"Every":  {Type: "integer"},
				},
			},
		}},
	})
})

const (
	goblinKey    = "prefabs/goblin.prefab.json"
	spawnerLevel = `{"objects":[{"group":"t","class":"Spawner","components":{"t:Spawner":` +
		`{"Prefab":"` + goblinKey + `","Every":3}}}]}`
	goblinPrefab = `{"objects":[{"group":"gfx","class":"Visual","components":{` +
		`"gfx:Transform":{"Pos":{"X":0,"Y":0},"PrevPos":{"X":0,"Y":0},"Rot":0,"Scale":{"X":1,"Y":1}},` +
		`"gfx:Sprite":{"ID":"chars/hero.png","Layer":1,"Origin":{"X":0,"Y":0},"FlipX":false,"FlipY":false,"Tint":0,"Blend":0,"Hidden":false}}}]}`
)

// prefabScene is a scene with the prefab loader registered and a level scope
// set on the resolver.
func prefabScene(t *testing.T) (*scene, *asset.Scope) {
	t.Helper()
	registerSpawner()
	s := newScene(t)
	prefabs.RegisterLoader(s.srv, prefabs.Options{
		World: s.world,
		Assets: func(sc *asset.Scope) god.AssetResolver {
			r := resolver.New(s.srv)
			r.SetScope(sc)
			return r
		},
	})
	level := s.srv.NewScope("level")
	s.res.SetScope(level)
	return s, level
}

func settle(srv *asset.Server, sc *asset.Scope) {
	for range 5000 {
		srv.Update()
		if sc.Done() {
			return
		}
	}
}

func deserializeSpawner(t *testing.T, s *scene, doc string) []god.Object {
	t.Helper()
	objs, err := god.Deserialize(s.world, strings.NewReader(doc))
	if err != nil {
		t.Fatal(err)
	}
	return objs
}

func TestPrefabKind(t *testing.T) {
	s, level := prefabScene(t)
	objs := deserializeSpawner(t, s, spawnerLevel)
	sp := ecs.GetComp[spawner](s.world, objs[0].Entity)
	if sp.Prefab.IsZero() || sp.Every != 3 {
		t.Fatalf("spawner = %+v", sp)
	}
	store := prefabs.StoreOf(s.srv)
	if got := store.Key(sp.Prefab).String(); got != goblinKey {
		t.Fatalf("Key = %q", got)
	}
	settle(s.srv, level)
	if !store.Ready(sp.Prefab) {
		t.Fatalf("prefab not ready: %v", store.Err(sp.Prefab))
	}
	if e := store.Spawn(sp.Prefab); !s.world.IsAlive(e) {
		t.Fatal("Spawn returned a dead entity")
	}

	// Round trip: the key is written back.
	out, err := god.SerializeComponents(s.world, "t", "Spawner", objs[0].Entity)
	if err != nil || !strings.Contains(string(out), `"Prefab":"`+goblinKey+`"`) {
		t.Fatalf("SerializeComponents = %s, %v", out, err)
	}

	// Releasing the level scope unloads the prefab; the handle goes stale.
	level.Release()
	s.srv.Update()
	if n := store.Generic().Len(); n != 0 {
		t.Fatalf("%d prefabs left after releasing the level scope", n)
	}
	if _, err := god.SerializeComponents(s.world, "t", "Spawner", objs[0].Entity); err == nil ||
		!strings.Contains(err.Error(), "stale prefab handle") {
		t.Fatalf("stale handle: %v", err)
	}
}

func TestPrefabKindErrors(t *testing.T) {
	s, _ := prefabScene(t)
	h, err := s.res.ResolveAsset(prefabs.AssetKind, goblinKey)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := h.(gfx.PrefabHandle); !ok {
		t.Fatalf("ResolveAsset returned %T", h)
	}
	if _, err := s.res.AssetKey(prefabs.AssetKind, gfx.SpriteHandle{}); err == nil ||
		!strings.Contains(err.Error(), "needs the matching gfx handle") {
		t.Errorf("wrong handle: %v", err)
	}
	if _, err := s.res.AssetKey(render.AssetKindSprite, h); err == nil {
		t.Error("AssetKey accepted a prefab handle for the sprite kind")
	}

	// Without a prefab loader the kind is an error, not a panic.
	bare := newScene(t)
	if _, err := bare.res.ResolveAsset(prefabs.AssetKind, goblinKey); err == nil {
		t.Error("ResolveAsset without a prefab loader returned no error")
	}
}

// An asset kind registered from outside the engine, the way a game's own
// package adds one. Registered once per process, so -count=2 does not
// register the name twice.
func init() {
	assetkind.Register("test-sprite", assetkind.Of(render.LoadSprite,
		func(srv *asset.Server, h gfx.SpriteHandle) asset.Key { return render.SpriteStoreOf(srv).Key(h) }))
}

// TestRegisteredKind resolves and serializes the outside kind.
func TestRegisteredKind(t *testing.T) {
	s := newScene(t)
	h, err := s.res.ResolveAsset("test-sprite", "chars/hero.png")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := h.(gfx.SpriteHandle); !ok {
		t.Fatalf("handle %T", h)
	}
	if key, err := s.res.AssetKey("test-sprite", h); err != nil || key != "chars/hero.png" {
		t.Errorf("AssetKey = %q, %v", key, err)
	}
	if _, err := s.res.AssetKey("test-sprite", gfx.SoundHandle{}); err == nil || !strings.Contains(err.Error(), "matching gfx handle") {
		t.Errorf("wrong handle: %v", err)
	}
	for _, f := range []func() error{
		func() error { _, err := s.res.ResolveAsset("nope", "x"); return err },
		func() error { _, err := s.res.AssetKey("nope", h); return err },
	} {
		if err := f(); err == nil || !strings.Contains(err.Error(), `unknown asset kind "nope"`) {
			t.Errorf("unregistered kind: %v", err)
		}
	}
}

// TestOwnersRegisterTheirKinds checks that linking the owning packages is
// enough for the resolver to know their kinds (the table in
// doc/guide/engine/kinds.md; "voice" and "dialogue" are checked in package
// speech, which this package cannot import).
func TestOwnersRegisterTheirKinds(t *testing.T) {
	names := assetkind.Names()
	for _, kind := range []string{
		render.AssetKindSprite, render.AssetKindFont, render.AssetKindShader, anim.AssetKindClip,
		sound.AssetKindSound, sound.AssetKindMusic, prefabs.AssetKind, particles.AssetKindEffect,
	} {
		if !slices.Contains(names, kind) {
			t.Errorf("kind %q is not registered", kind)
		}
	}
}
