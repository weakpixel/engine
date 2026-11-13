package god

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/weakpixel/ecs"
)

// fakeHandle stands in for an engine asset handle.
type fakeHandle struct{ index, gen uint32 }

type testSprite struct {
	ID    fakeHandle `json:"-"`
	Layer int
	Time  float64 `json:"-"` // runtime
}

// fakeResolver hands out handles 1..n for the paths it knows.
type fakeResolver struct{ paths []string }

func (r *fakeResolver) ResolveAsset(kind, path string) (any, error) {
	switch {
	case kind == "wrong":
		return 42, nil
	case path == "boom":
		return nil, errors.New("no such file")
	}
	for i, p := range r.paths {
		if p == path {
			return fakeHandle{uint32(i + 1), 1}, nil
		}
	}
	r.paths = append(r.paths, path)
	return fakeHandle{uint32(len(r.paths)), 1}, nil
}

func (r *fakeResolver) AssetKey(kind string, h any) (string, error) {
	fh := h.(fakeHandle)
	if int(fh.index) > len(r.paths) {
		return "", errors.New("stale handle")
	}
	return r.paths[fh.index-1], nil
}

func spriteComponent(id ecs.ComponentID, kind string) Component {
	return Component{
		Group:     "t",
		Name:      "Sprite",
		RuntimeID: id,
		Decode:    DecodeFunc[testSprite](),
		Schema: &TypeSchema{
			Type: "object",
			Properties: map[string]*TypeSchema{
				"ID":    {Type: "string", Format: "asset", XAsset: kind},
				"Layer": {Type: "integer"},
				"Time":  {Type: "number", XRuntime: true},
			},
		},
	}
}

func registerSpriteSchema(class, kind string) {
	RegisterSchema(Schema{
		SchemaMeta: SchemaMeta{Group: "asset", Class: class},
		Components: []Component{spriteComponent(ecs.RegisterComp[testSprite](), kind)},
	})
}

func deserializeSprite(w *ecs.World, class, components string) ([]Object, error) {
	return Deserialize(w, strings.NewReader(fmt.Sprintf(`{"objects":[{"group":"asset","class":%q,"components":{"t:Sprite":%s}}]}`, class, components)))
}

func TestAssetRoundTrip(t *testing.T) {
	registerSpriteSchema("sprite", "sprite")
	w := ecs.NewWorld()
	res := &fakeResolver{}
	SetAssetResolver(w, res)

	objs, err := deserializeSprite(w, "sprite", `{"ID":"chars/hero.json#idle","Layer":2}`)
	if err != nil {
		t.Fatal(err)
	}
	got := ecs.GetComp[testSprite](w, objs[0].Entity)
	if got.ID != (fakeHandle{1, 1}) || got.Layer != 2 {
		t.Fatalf("component = %+v", got)
	}

	out, err := SerializeComponents(w, "asset", "sprite", objs[0].Entity)
	if err != nil {
		t.Fatal(err)
	}
	if want := `{"t:Sprite":{"ID":"chars/hero.json#idle","Layer":2}}`; string(out) != want {
		t.Fatalf("SerializeComponents = %s, want %s", out, want)
	}
}

func TestAssetAbsentAndEmptyKeepZeroHandle(t *testing.T) {
	registerSpriteSchema("sprite", "sprite")
	for _, comps := range []string{`{"Layer":1}`, `{"ID":"","Layer":1}`} {
		w := ecs.NewWorld() // no resolver needed when no key is given
		objs, err := deserializeSprite(w, "sprite", comps)
		if err != nil {
			t.Fatalf("%s: %v", comps, err)
		}
		if got := ecs.GetComp[testSprite](w, objs[0].Entity); got.ID != (fakeHandle{}) {
			t.Fatalf("%s: handle = %+v, want zero", comps, got.ID)
		}
		out, err := SerializeComponents(w, "asset", "sprite", objs[0].Entity)
		if err != nil {
			t.Fatal(err)
		}
		if want := `{"t:Sprite":{"Layer":1}}`; string(out) != want {
			t.Fatalf("zero handle serialized as %s, want %s", out, want)
		}
	}
}

func TestAssetErrors(t *testing.T) {
	registerSpriteSchema("sprite", "sprite")
	registerSpriteSchema("wrong", "wrong")
	for _, tc := range []struct {
		name, class, json, want string
		resolver                bool
	}{
		{"no resolver", "sprite", `{"ID":"a.png"}`, "no AssetResolver", false},
		{"unknown path", "sprite", `{"ID":"boom"}`, `resolve sprite asset "boom": no such file`, true},
		{"wrong type", "wrong", `{"ID":"a.png"}`, "resolver returned int", true},
		{"not a string", "sprite", `{"ID":3}`, "$.ID: expected string, got number", true},
		{"runtime field", "sprite", `{"Time":1.5}`, "$.Time: runtime field", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := ecs.NewWorld()
			if tc.resolver {
				SetAssetResolver(w, &fakeResolver{})
			}
			_, err := deserializeSprite(w, tc.class, tc.json)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("got %v, want error containing %q", err, tc.want)
			}
			n := 0
			for range ecs.NewView1[testSprite](w).Each() {
				n++
			}
			if n != 0 {
				t.Fatalf("%d entities left after error", n)
			}
		})
	}
}

func TestAssetSerializeErrors(t *testing.T) {
	registerSpriteSchema("sprite", "sprite")
	w := ecs.NewWorld()
	e := w.NewEntity(testSprite{ID: fakeHandle{9, 1}})

	if _, err := SerializeComponents(w, "asset", "sprite", e); err == nil || !strings.Contains(err.Error(), "no AssetResolver") {
		t.Fatalf("missing resolver: %v", err)
	}
	SetAssetResolver(w, &fakeResolver{})
	if _, err := SerializeComponents(w, "asset", "sprite", e); err == nil || !strings.Contains(err.Error(), "stale handle") {
		t.Fatalf("AssetKey error: %v", err)
	}
}

// TestAssetRuntimeFieldOmitted checks that runtime state never reaches the output.
func TestAssetRuntimeFieldOmitted(t *testing.T) {
	registerSpriteSchema("sprite", "sprite")
	w := ecs.NewWorld()
	e := w.NewEntity(testSprite{Layer: 3, Time: 12.5})
	out, err := SerializeComponents(w, "asset", "sprite", e)
	if err != nil {
		t.Fatal(err)
	}
	if want := `{"t:Sprite":{"Layer":3}}`; string(out) != want {
		t.Fatalf("got %s, want %s", out, want)
	}
}

// TestDecodeAssetsPlainComponentAllocs checks that components without asset
// properties pay nothing for the asset pass.
func TestDecodeAssetsPlainComponentAllocs(t *testing.T) {
	w := ecs.NewWorld()
	s := &TypeSchema{Type: "object", Properties: map[string]*TypeSchema{"X": {Type: "integer"}, "Y": {Type: "integer"}}}
	var v any = testPos{1, 2}
	data := []byte(`{"X":1,"Y":2}`)
	allocs := testing.AllocsPerRun(100, func() {
		if _, err := decodeAssets(worldAssetResolver(w), s, data, v); err != nil {
			t.Fatal(err)
		}
		if _, err := encodeAssets(w, s, v, data); err != nil {
			t.Fatal(err)
		}
	})
	if allocs != 0 {
		t.Fatalf("allocs = %v, want 0", allocs)
	}
}
