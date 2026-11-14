package god

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/weakpixel/ecs"
)

// pfFixed has its entity references at fixed positions of the Go value;
// testLinks (entity-refs_test.go) also has them under a pointer, a slice
// and a map.
type pfFixed struct {
	Target ecs.Entity
	Pair   [2]ecs.Entity
	Nested struct{ Parent ecs.Entity }
}

// registerMultiSchema registers the schemas of prefabWorld and the
// component types "x:Fixed" and "x:Links", which hold references.
func registerMultiSchema() {
	registerPrefabSchema()
	fixed := &TypeSchema{Type: "object", Properties: map[string]*TypeSchema{
		"Target": refSchema(),
		"Pair":   {Type: "array", Items: refSchema(), MinItems: new(2), MaxItems: new(2)},
		"Nested": {Type: "object", Properties: map[string]*TypeSchema{"Parent": refSchema()}},
	}}
	RegisterComponent(ComponentSchema{Group: "x", Name: "Fixed", RuntimeID: ecs.RegisterComp[pfFixed](), Schema: fixed, Decode: DecodeFunc[pfFixed]()})
	RegisterComponent(ComponentSchema{Group: "x", Name: "Links", RuntimeID: ecs.RegisterComp[testLinks](), Schema: linksSchema(), Decode: DecodeFunc[testLinks]()})
}

// expandFixture is testdata/prefab-expand.json, shared with the extension.
type expandFixture struct {
	Prefabs []struct {
		Key string
		Doc json.RawMessage
	}
	Cases []struct {
		Name   string
		Prefab string
		Entry  instance
		Want   json.RawMessage
		Error  string
	}
	ParseErrors []struct {
		Name  string
		Doc   json.RawMessage
		Error string
	}
}

// loadExpandFixture parses the fixture's prefabs, in order, into a PrefabMap.
func loadExpandFixture(t *testing.T) (expandFixture, PrefabMap) {
	t.Helper()
	return loadExpandFixtureAs(t, false)
}

// loadExpandFixtureAs is loadExpandFixture; complete adds the Layer the
// Go struct encodes to every sprite of the prefabs' objects. The fixture
// leaves it out, but Serialize writes complete components, so a round trip
// only gives back the prefab's values if the prefab has them all.
func loadExpandFixtureAs(t *testing.T, complete bool) (expandFixture, PrefabMap) {
	t.Helper()
	registerMultiSchema()
	data, err := os.ReadFile("testdata/prefab-expand.json")
	if err != nil {
		t.Fatal(err)
	}
	var fx expandFixture
	if err := json.Unmarshal(data, &fx); err != nil {
		t.Fatal(err)
	}
	m := PrefabMap{}
	for _, p := range fx.Prefabs {
		doc := p.Doc
		if complete {
			doc = completeSprites(t, doc)
		}
		if err := m.Add(p.Key, doc); err != nil {
			t.Fatalf("prefab %s: %v", p.Key, err)
		}
	}
	return fx, m
}

// completeSprites adds "Layer": 0 to the t:Sprite of each object of a
// prefab document that lacks it.
func completeSprites(t *testing.T, doc json.RawMessage) json.RawMessage {
	t.Helper()
	var d struct {
		Name    string           `json:"name,omitempty"`
		Objects []map[string]any `json:"objects"`
	}
	if err := json.Unmarshal(doc, &d); err != nil {
		t.Fatal(err)
	}
	for _, o := range d.Objects {
		comps, _ := o["components"].(map[string]any)
		if sprite, ok := comps["t:Sprite"].(map[string]any); ok {
			if _, has := sprite["Layer"]; !has {
				sprite["Layer"] = 0
			}
		}
	}
	out, err := json.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// multiWorld is a world that knows the fixture's prefabs and a fake asset
// resolver.
func multiWorld(t *testing.T) (*ecs.World, PrefabMap) {
	t.Helper()
	_, m := loadExpandFixtureAs(t, true)
	w := ecs.NewWorld()
	SetAssetResolver(w, &fakeResolver{})
	SetPrefabSource(w, m)
	return w, m
}

// TestPrefabExpandInstance runs the shared cases of testdata/prefab-expand.json
// (the VSCode extension runs the same file against its own expansion).
func TestPrefabExpandInstance(t *testing.T) {
	fx, m := loadExpandFixture(t)
	for _, tc := range fx.Cases {
		t.Run(tc.Name, func(t *testing.T) {
			got, err := m[tc.Prefab].ExpandInstance(tc.Entry.entry())
			if tc.Error != "" {
				if err == nil || !strings.Contains(err.Error(), tc.Error) {
					t.Fatalf("got error %v, want %q", err, tc.Error)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			type object struct {
				ID         string                     `json:"id"`
				Part       string                     `json:"part"`
				Group      string                     `json:"group"`
				Class      string                     `json:"class"`
				Components map[string]json.RawMessage `json:"components"`
			}
			list := make([]object, len(got))
			for i, o := range got {
				list[i] = object{o.ID, o.Part, o.Group, o.Class, o.Components}
			}
			data, err := json.Marshal(list)
			if err != nil {
				t.Fatal(err)
			}
			if eq, _ := jsonEqual(data, tc.Want); !eq {
				t.Fatalf("got\n%s\nwant\n%s", data, tc.Want)
			}
		})
	}
	for _, tc := range fx.ParseErrors {
		t.Run(tc.Name, func(t *testing.T) {
			_, err := ParsePrefab("bad", tc.Doc, m)
			if err == nil || !strings.Contains(err.Error(), tc.Error) {
				t.Fatalf("got error %v, want %q", err, tc.Error)
			}
		})
	}
}

func TestMultiObjectPrefabParts(t *testing.T) {
	_, m := loadExpandFixture(t)
	for _, tc := range []struct {
		key, root, base string
		parts           []string
	}{
		{"knight", "knight", "", []string{"knight", "sword", "shadow", "glint"}},
		{"rider", "rider", "", []string{"rider", "horse", "horse/saddle", "hat"}},
		{"king", "knight", "knight", []string{"knight", "sword", "shadow", "glint", "crown"}},
		{"lord", "lord", "knight", []string{"lord", "sword", "shadow", "glint"}},
	} {
		p := m[tc.key]
		if got := p.Parts(); strings.Join(got, ",") != strings.Join(tc.parts, ",") {
			t.Errorf("%s Parts = %v, want %v", tc.key, got, tc.parts)
		}
		if p.ID() != tc.root || p.Base() != tc.base || p.Meta().Key() != "pf:Unit" {
			t.Errorf("%s: id %q base %q meta %v", tc.key, p.ID(), p.Base(), p.Meta())
		}
		for i, id := range tc.parts {
			if got := p.PartIndex(id); got != i {
				t.Errorf("%s PartIndex(%q) = %d, want %d", tc.key, id, got, i)
			}
		}
		if p.PartIndex("nope") != -1 {
			t.Errorf("%s PartIndex(nope) != -1", tc.key)
		}
	}
	if raw, _ := m["knight"].Component("t:Sprite"); string(raw) != `{"ID":"knight.png"}` {
		t.Errorf("Component is the root's: %s", raw)
	}
	// A prefab with one object and no id has the part "".
	if p := (PrefabMap{}); p.Add("anon", []byte(`{"objects":[{"group":"pf","class":"Other","components":{"t:Pos":{"X":0,"Y":0}}}]}`)) != nil ||
		len(p["anon"].Parts()) != 1 || p["anon"].PartIndex("") != 0 {
		t.Error("anonymous single-object prefab")
	}
}

func TestMultiObjectPrefabCycle(t *testing.T) {
	_, m := loadExpandFixture(t)
	// horse nesting rider, which nests horse.
	err := m.Add("horse", []byte(`{"objects":[
		{"id":"horse","group":"pf","class":"Unit","components":{"t:Pos":{"X":0,"Y":0},"t:Sprite":{"ID":"h.png"}}},
		{"id":"r","prefab":"rider"}]}`))
	if err == nil || !strings.Contains(err.Error(), "prefab cycle: horse -> rider -> horse") {
		t.Fatalf("cycle: %v", err)
	}
}

const multiLevel = `{"objects":[
	{"id":"hero","group":"pf","class":"Other","components":{"t:Pos":{"X":1,"Y":1}}},
	{"id":"k1","prefab":"knight","components":{"t:Pos":{"X":5,"Y":5}},"children":{"sword":{"t:Follow":{"Target":"hero"}},"shadow":null}},
	{"id":"torch","group":"pf","class":"Unit","components":{"t:Pos":{"X":0,"Y":0},"t:Sprite":{"ID":"t.png","Layer":0},"t:Follow":{"Target":"k1/sword","Speed":1}}},
	{"id":"k2","prefab":"knight"},
	{"id":"r1","prefab":"rider","children":{"horse/saddle":{"t:Pos":{"X":1}},"hat":null}}
]}`

func TestDeserializeMultiObjectInstances(t *testing.T) {
	w, _ := multiWorld(t)
	objs, err := Deserialize(w, strings.NewReader(multiLevel))
	if err != nil {
		t.Fatal(err)
	}
	var ids, parts []string
	for _, o := range objs {
		ids = append(ids, o.ID)
		parts = append(parts, o.Part)
	}
	wantIDs := "hero,k1,k1/sword,k1/glint,torch,k2,k2/sword,k2/shadow,k2/glint,r1,r1/horse,r1/horse/saddle"
	wantParts := ",,sword,glint,,,sword,shadow,glint,,horse,horse/saddle"
	if strings.Join(ids, ",") != wantIDs || strings.Join(parts, ",") != wantParts {
		t.Fatalf("ids %v\nparts %v", ids, parts)
	}
	byID := map[string]Object{}
	for _, o := range objs {
		byID[o.ID] = o
	}
	follow := func(id string) ecs.Entity { return ecs.GetComp[testFollow](w, byID[id].Entity).Target }
	for _, tc := range []struct{ id, target string }{
		{"k1/sword", "hero"}, // override points at a level object
		{"k1/glint", "k1/sword"},
		{"torch", "k1/sword"}, // a level object attached to a part
		{"k2/sword", "k2"},
		{"k2/shadow", "k2"},
		{"k2/glint", "k2/sword"},
		{"r1/horse", "r1"},
		{"r1/horse/saddle", "r1/horse"},
	} {
		if got := follow(tc.id); got != byID[tc.target].Entity {
			t.Errorf("%s follows %v, want %s = %v", tc.id, got, tc.target, byID[tc.target].Entity)
		}
	}
	if p := ecs.GetComp[pfPos](w, byID["r1/horse/saddle"].Entity); *p != (pfPos{1, -3}) {
		t.Errorf("saddle Pos = %+v", *p)
	}
	for id, want := range map[string][2]string{"k1": {"knight", ""}, "k1/sword": {"knight", "sword"}, "r1/horse/saddle": {"rider", "horse/saddle"}} {
		key, part, ok := PrefabPartOf(w, byID[id].Entity)
		if !ok || key != want[0] || part != want[1] || byID[id].Prefab != want[0] {
			t.Errorf("PrefabPartOf(%s) = %q %q %v", id, key, part, ok)
		}
	}
	if _, _, ok := PrefabPartOf(w, byID["torch"].Entity); ok {
		t.Error("a level object has no prefab")
	}
	if key, ok := PrefabOf(w, byID["k1/sword"].Entity); !ok || key != "knight" {
		t.Errorf("PrefabOf(part) = %q %v", key, ok)
	}
	n := 0
	for o := range Objects(w) {
		if o.Part != "" {
			n++
		}
	}
	if n != 7 {
		t.Errorf("Objects reports %d parts, want 7", n)
	}
}

func TestDeserializeMultiObjectErrors(t *testing.T) {
	for _, tc := range []struct{ name, doc, want string }{
		{"no id", `{"objects":[{"prefab":"knight"}]}`, `objects[0]: instance of multi-object prefab "knight" needs an "id"`},
		{"removed part still referenced", `{"objects":[{"id":"a","prefab":"knight","children":{"sword":null}}]}`, `objects[0]: children: removing "sword" leaves "glint" with a reference to it`},
		{"unknown part", `{"objects":[{"id":"a","prefab":"knight","children":{"axe":{}}}]}`, `objects[0]: children: prefab "knight" has no object "axe"`},
		{"bad part override", `{"objects":[{"id":"a","prefab":"knight","children":{"sword":{"t:Sprite":{"ID":3}}}}]}`, `objects[0] part "sword": component t:Sprite: $.ID: expected string`},
		{"slash in an authored id", `{"objects":[{"id":"a/b","group":"pf","class":"Other","components":{"t:Pos":{"X":0,"Y":0}}}]}`, `objects[0]: id "a/b" contains "/"`},
		{"children without prefab", `{"objects":[{"id":"a","group":"pf","class":"Other","children":{},"components":{"t:Pos":{"X":0,"Y":0}}}]}`, `objects[0]: "children" needs a "prefab"`},
		{"duplicate with a part", `{"objects":[{"id":"a","prefab":"knight"},{"id":"b","prefab":"knight"},{"id":"a","prefab":"horse"}]}`, `objects[2]: duplicate id "a"`},
		{"unknown level id", `{"objects":[{"id":"a","prefab":"knight","children":{"sword":{"t:Follow":{"Target":"zz"}}}}]}`, `objects[0] part "sword": component t:Follow: $.Target: unknown object id "zz"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w, _ := multiWorld(t)
			_, err := Deserialize(w, strings.NewReader(tc.doc))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("got %v, want %q", err, tc.want)
			}
			for o := range Objects(w) {
				t.Errorf("entity %v left behind", o.Entity)
			}
		})
	}
}

// TestMultiObjectRoundTrip checks Deserialize then Serialize gives back the
// authored document, with nested, overridden and removed parts.
func TestMultiObjectRoundTrip(t *testing.T) {
	w, _ := multiWorld(t)
	objs, err := Deserialize(w, strings.NewReader(multiLevel))
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := Serialize(w, &out, objs...); err != nil {
		t.Fatal(err)
	}
	if eq, _ := jsonEqual(out.Bytes(), []byte(multiLevel)); !eq {
		t.Fatalf("round trip:\n%s\nwant\n%s", out.Bytes(), multiLevel)
	}
	// Folded parts are not entries of their own.
	var doc struct{ Objects []json.RawMessage }
	_ = json.Unmarshal(out.Bytes(), &doc)
	if len(doc.Objects) != 5 {
		t.Errorf("%d entries, want 5", len(doc.Objects))
	}
}

func TestSerializeMultiObjectErrors(t *testing.T) {
	w, _ := multiWorld(t)
	objs, err := Deserialize(w, strings.NewReader(`{"objects":[{"id":"k1","prefab":"knight"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	root, sword := objs[0], objs[1]
	for _, tc := range []struct {
		name string
		objs []Object
		want string
	}{
		{"orphan part", []Object{sword}, `part "sword" of prefab "knight" has no root object "k1"`},
		{"wrong id", []Object{root, {SchemaMeta: sword.SchemaMeta, ID: "k1/axe", Prefab: "knight", Part: "sword", Entity: sword.Entity}}, `needs the ID "<root ID>/sword", has "k1/axe"`},
		{"unknown part", []Object{root, {SchemaMeta: sword.SchemaMeta, ID: "k1/axe", Prefab: "knight", Part: "axe", Entity: sword.Entity}}, `prefab "knight" has no object "axe"`},
		{"part without prefab", []Object{{ID: "k1/sword", Part: "sword", Entity: sword.Entity}}, `part "sword" without a prefab`},
		{"root without id", []Object{{SchemaMeta: root.SchemaMeta, Prefab: "knight", Entity: root.Entity}}, `instance of multi-object prefab "knight" needs an ID`},
		{"slash in id", []Object{{SchemaMeta: sword.SchemaMeta, ID: "a/b", Entity: sword.Entity}}, `id "a/b" contains "/"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			err := Serialize(w, &out, tc.objs...)
			if err == nil || !strings.Contains(err.Error(), tc.want) || out.Len() != 0 {
				t.Fatalf("got %v (%d bytes written), want %q", err, out.Len(), tc.want)
			}
		})
	}
	// A part of the prefab that is not among the objects is removed.
	var out bytes.Buffer
	if err := Serialize(w, &out, root); err != nil {
		t.Fatal(err)
	}
	want := `{"objects":[{"id":"k1","prefab":"knight","children":{"glint":null,"shadow":null,"sword":null}}]}`
	if eq, _ := jsonEqual(out.Bytes(), []byte(want)); !eq {
		t.Fatalf("got %s, want %s", out.Bytes(), want)
	}
}

func TestBoundPrefabSpawnAll(t *testing.T) {
	w, m := multiWorld(t)
	b, err := m["king"].Bind(w, nil)
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		ents := b.SpawnAll(nil, pfPos{X: 100, Y: 200})
		if len(ents) != 5 {
			t.Fatalf("SpawnAll returned %d entities, want 5", len(ents))
		}
		idx := m["king"].PartIndex
		if p := ecs.GetComp[pfPos](w, ents[0]); *p != (pfPos{100, 200}) {
			t.Errorf("override applies to the root: %+v", *p)
		}
		if p := ecs.GetComp[pfPos](w, ents[idx("sword")]); *p != (pfPos{8, -4}) {
			t.Errorf("a part keeps its own Pos: %+v", *p)
		}
		for part, target := range map[string]string{"sword": "knight", "shadow": "knight", "glint": "sword", "crown": "knight"} {
			if got := ecs.GetComp[testFollow](w, ents[idx(part)]).Target; got != ents[idx(target)] {
				t.Errorf("%s follows %v, want %s = %v", part, got, target, ents[idx(target)])
			}
		}
		for i, e := range ents {
			key, part, ok := PrefabPartOf(w, e)
			wantPart := ""
			if i > 0 {
				wantPart = m["king"].Parts()[i]
			}
			if !ok || key != "king" || part != wantPart {
				t.Errorf("part %d is tagged %q %q %v", i, key, part, ok)
			}
			if meta, _ := ClassOf(w, e); meta.Key() != "pf:Unit" {
				t.Errorf("part %d class %v", i, meta)
			}
		}
	}
	// Spawn is SpawnAll's root, and SpawnAll appends.
	buf := make([]ecs.Entity, 0, 8)
	buf = append(buf, ecs.Entity{ID: 99})
	buf = b.SpawnAll(buf)
	if len(buf) != 6 || buf[0] != (ecs.Entity{ID: 99}) {
		t.Fatalf("SpawnAll does not append: %v", buf)
	}
	root := b.Spawn()
	if _, part, ok := PrefabPartOf(w, root); !ok || part != "" {
		t.Errorf("Spawn returned a part: %q", part)
	}
}

func TestBoundPrefabEntityReferenceShapes(t *testing.T) {
	w, m := multiWorld(t)
	if err := m.Add("linked", []byte(`{"objects":[
		{"id":"a","group":"pf","class":"Other","components":{"t:Pos":{"X":0,"Y":0},
			"x:Fixed":{"Target":"b","Pair":["a","b"],"Nested":{"Parent":"a"}},
			"x:Links":{"Target":"b","Owner":"a","Group":["a","b"],"Pair":["b","a"],"ByName":{"k":"b"},"Nested":{"Parent":"b"},"Weight":2}}},
		{"id":"b","group":"pf","class":"Other","components":{"t:Pos":{"X":1,"Y":1},"x:Fixed":{"Target":"a"}}}
	]}`)); err != nil {
		t.Fatal(err)
	}
	b, err := m["linked"].Bind(w, nil)
	if err != nil {
		t.Fatal(err)
	}
	fixed := map[string]bool{}
	for _, r := range b.parts[0].refs {
		fixed[r.c.Key()] = r.fixed
	}
	if !fixed["x:Fixed"] || fixed["x:Links"] || len(fixed) != 2 {
		t.Fatalf("fixed = %v, want x:Fixed only", fixed)
	}
	for range 2 {
		ents := b.SpawnAll(nil)
		a, bb := ents[0], ents[1]
		if f := ecs.GetComp[pfFixed](w, a); f.Target != bb || f.Pair != [2]ecs.Entity{a, bb} || f.Nested.Parent != a {
			t.Errorf("a Fixed = %+v", *f)
		}
		if f := ecs.GetComp[pfFixed](w, bb); f.Target != a || f.Pair != [2]ecs.Entity{} {
			t.Errorf("b Fixed = %+v", *f)
		}
		l := ecs.GetComp[testLinks](w, a)
		if l.Target != bb || *l.Owner != a || l.Group[0] != a || l.Group[1] != bb || l.Pair != [2]ecs.Entity{bb, a} || l.ByName["k"] != bb || l.Nested.Parent != bb || l.Weight != 2 {
			t.Errorf("a Links = %+v", *l)
		}
	}
}

func TestBoundPrefabMultiBindErrors(t *testing.T) {
	_, m := loadExpandFixture(t)
	w := ecs.NewWorld() // no AssetResolver
	if _, err := m["knight"].Bind(w, nil); err == nil || !strings.Contains(err.Error(), `prefab "knight": object "knight": component t:Sprite`) {
		t.Fatalf("got %v", err)
	}
}

func TestBoundPrefabMultiSpawnAllocs(t *testing.T) {
	w, m := multiWorld(t)
	b, err := m["knight"].Bind(w, nil) // knight, sword, shadow, glint: 3 Follows
	if err != nil {
		t.Fatal(err)
	}
	dst := make([]ecs.Entity, 0, 8)
	var at any = pfPos{X: 1}
	b.Spawn()
	// One box per component with references, and no JSON: decoding even
	// one Follow from JSON would allocate a dozen times.
	if n := testing.AllocsPerRun(500, func() { b.Spawn(at) }); n > 3 {
		t.Errorf("Spawn of 3 references allocates %v times per run, want at most 3", n)
	}
	if n := testing.AllocsPerRun(500, func() { dst = b.SpawnAll(dst[:0], at) }); n > 3 {
		t.Errorf("SpawnAll of 3 references allocates %v times per run, want at most 3", n)
	}
}

func BenchmarkPrefabSpawnMulti(b *testing.B) {
	registerMultiSchema()
	m := PrefabMap{}
	for _, doc := range []string{`{"objects":[
		{"id":"knight","group":"pf","class":"Unit","components":{"t:Pos":{"X":0,"Y":0},"t:Sprite":{"ID":"knight.png"}}},
		{"id":"sword","group":"pf","class":"Unit","components":{"t:Pos":{"X":8,"Y":-4},"t:Sprite":{"ID":"sword.png"},"t:Follow":{"Target":"knight","Speed":1}}},
		{"id":"shadow","group":"pf","class":"Unit","components":{"t:Pos":{"X":0,"Y":2},"t:Sprite":{"ID":"shadow.png"},"t:Follow":{"Target":"knight","Speed":2}}}]}`} {
		if err := m.Add("knight", []byte(doc)); err != nil {
			b.Fatal(err)
		}
	}
	w := ecs.NewWorld()
	SetAssetResolver(w, &fakeResolver{})
	bp, err := m["knight"].Bind(w, nil)
	if err != nil {
		b.Fatal(err)
	}
	dst := make([]ecs.Entity, 0, 4)
	var at any = pfPos{X: 1, Y: 2}
	b.ReportAllocs()
	for b.Loop() {
		dst = bp.SpawnAll(dst[:0], at)
	}
}

func TestPrefabPartOverrides(t *testing.T) {
	_, m := loadExpandFixture(t)
	k := m["knight"]
	// The sword of instance k1, moved and pointed at a level object: only
	// what differs from the prefab's sword expanded for k1 is returned.
	got, err := k.PartOverrides("k1", "sword", map[string]json.RawMessage{
		"t:Pos":    json.RawMessage(`{"X":9,"Y":-4}`),
		"t:Follow": json.RawMessage(`{"Target":"k1","Speed":1}`), // equal to the expanded prefab
	})
	if err != nil {
		t.Fatal(err)
	}
	// (Components the caller did not pass count as removed: callers read
	// the keys they passed.)
	if _, has := got["t:Follow"]; has || string(got["t:Pos"]) != `{"X":9}` {
		t.Fatalf("overrides = %s", got)
	}
	if got, err = k.PartOverrides("k1", "", map[string]json.RawMessage{"t:Pos": json.RawMessage(`{"X":5,"Y":0}`)}); err != nil || string(got["t:Pos"]) != `{"X":5}` {
		t.Fatalf("root overrides = %s, %v", got, err)
	}
	if _, err := k.PartOverrides("k1", "axe", nil); err == nil || !strings.Contains(err.Error(), `prefab "knight" has no object "axe"`) {
		t.Fatalf("unknown part: %v", err)
	}
}
