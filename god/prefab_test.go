package god

import (
	"bytes"
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/weakpixel/ecs"
)

type pfPos struct{ X, Y float64 }

type pfHealth struct{ Cur, Max int }

// pfLoot and pfBuff are registered component types that no class declares.
type pfLoot struct{ Gold int }

type pfBuff struct{ N int }

var (
	pfPosID    = ecs.RegisterComp[pfPos]()
	pfHealthID = ecs.RegisterComp[pfHealth]()
	pfSpriteID = ecs.RegisterComp[testSprite]()
)

// registerPrefabSchema registers "pf:Unit" {Pos, Sprite, ?Health, ?Follow},
// "pf:Other" {Pos} and the component types "x:Loot" and "x:Buff", which no
// class declares.
func registerPrefabSchema() {
	num := &TypeSchema{Type: "number"}
	health := Component{Group: "t", Name: "Health", Optional: true, RuntimeID: pfHealthID, Decode: DecodeFunc[pfHealth](),
		Schema: &TypeSchema{Type: "object", Required: []string{"Cur", "Max"}, Properties: map[string]*TypeSchema{"Cur": {Type: "integer"}, "Max": {Type: "integer"}}}}
	pos := Component{Group: "t", Name: "Pos", RuntimeID: pfPosID, Decode: DecodeFunc[pfPos](),
		Schema: &TypeSchema{Type: "object", Required: []string{"X", "Y"}, Properties: map[string]*TypeSchema{"X": num, "Y": num}}}
	follow := followComponent()
	follow.Optional = true
	RegisterSchema(Schema{SchemaMeta: SchemaMeta{Group: "pf", Class: "Unit"}, Components: []Component{
		pos, spriteComponent(pfSpriteID, "sprite"), health, follow,
	}})
	RegisterSchema(Schema{SchemaMeta: SchemaMeta{Group: "pf", Class: "Other"}, Components: []Component{pos}})
	gold := &TypeSchema{Type: "object", Properties: map[string]*TypeSchema{"Gold": {Type: "integer"}}}
	RegisterComponent(ComponentSchema{Group: "x", Name: "Loot", RuntimeID: ecs.RegisterComp[pfLoot](), Schema: gold, Decode: DecodeFunc[pfLoot]()})
	n := &TypeSchema{Type: "object", Properties: map[string]*TypeSchema{"N": {Type: "integer"}}}
	RegisterComponent(ComponentSchema{Group: "x", Name: "Buff", RuntimeID: ecs.RegisterComp[pfBuff](), Schema: n, Decode: DecodeFunc[pfBuff]()})
}

const goblinPrefab = `{"objects":[{"id":"self","group":"pf","class":"Unit","components":{
	"t:Pos": {"X": 0, "Y": 0},
	"t:Sprite": {"ID": "goblin.png", "Layer": 2},
	"t:Health": {"Cur": 5, "Max": 5}
}}]}`

const archerPrefab = `{"objects":[{"prefab":"goblin","components":{
	"t:Sprite": {"ID": "archer.png"},
	"t:Health": {"Max": 8}
}}]}`

// prefabWorld returns a world with a fake asset resolver and a PrefabMap
// holding "goblin" and its variant "archer".
func prefabWorld(t *testing.T) (*ecs.World, PrefabMap, *fakeResolver) {
	t.Helper()
	registerPrefabSchema()
	m := PrefabMap{}
	if err := m.Add("goblin", []byte(goblinPrefab)); err != nil {
		t.Fatal(err)
	}
	if err := m.Add("archer", []byte(archerPrefab)); err != nil {
		t.Fatal(err)
	}
	w := ecs.NewWorld()
	res := &fakeResolver{}
	SetAssetResolver(w, res)
	SetPrefabSource(w, m)
	return w, m, res
}

func TestParsePrefab(t *testing.T) {
	_, m, _ := prefabWorld(t)

	g := m["goblin"]
	if g.Key() != "goblin" || g.ID() != "self" || g.Meta().Key() != "pf:Unit" || g.Base() != "" {
		t.Fatalf("goblin: key %q id %q meta %v base %q", g.Key(), g.ID(), g.Meta(), g.Base())
	}
	if raw, _ := g.Component("t:Health"); string(raw) != `{"Cur":5,"Max":5}` {
		t.Fatalf("goblin Health = %s", raw)
	}

	a := m["archer"]
	if a.Base() != "goblin" || a.Meta().Key() != "pf:Unit" || a.ID() != "self" {
		t.Fatalf("archer: base %q meta %v id %q", a.Base(), a.Meta(), a.ID())
	}
	for name, want := range map[string]string{
		"t:Pos":    `{"X":0,"Y":0}`,
		"t:Sprite": `{"ID":"archer.png","Layer":2}`,
		"t:Health": `{"Cur":5,"Max":8}`,
	} {
		if raw, _ := a.Component(name); string(raw) != want {
			t.Errorf("archer %s = %s, want %s", name, raw, want)
		}
	}

	for _, tc := range []struct{ name, doc, want string }{
		{"no object", `{"objects":[]}`, `prefab "x": no object`},
		{"two objects without ids", `{"objects":[{"group":"pf","class":"Other","components":{"t:Pos":{"X":0,"Y":0}}},{"group":"pf","class":"Other","components":{"t:Pos":{"X":0,"Y":0}}}]}`, `a prefab with several objects needs an "id" on each`},
		{"bad json", `{"objects":`, `prefab "x": deserialize:`},
		{"unknown class", `{"objects":[{"group":"pf","class":"Nope","components":{}}]}`, "unknown object pf:Nope"},
		{"invalid component", `{"objects":[{"group":"pf","class":"Other","components":{"t:Pos":{"X":"a","Y":0}}}]}`, "component t:Pos: $.X: expected number, got string"},
		{"missing required", `{"objects":[{"group":"pf","class":"Other","components":{}}]}`, "missing required component t:Pos"},
		{"unknown base", `{"objects":[{"prefab":"nope"}]}`, `prefab "nope": unknown prefab`},
		{"class mismatch", `{"objects":[{"prefab":"goblin","group":"pf","class":"Other"}]}`, `object pf:Other does not match prefab "goblin", which is pf:Unit`},
		{"bad override", `{"objects":[{"prefab":"goblin","components":{"t:Health":{"Max":"x"}}}]}`, "component t:Health: $.Max: expected integer, got string"},
		{"remove required", `{"objects":[{"prefab":"goblin","components":{"t:Pos":null}}]}`, "missing required component t:Pos"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParsePrefab("x", []byte(tc.doc), m)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("got %v, want %q", err, tc.want)
			}
		})
	}

	if _, err := ParsePrefab("x", []byte(archerPrefab), nil); err == nil || !strings.Contains(err.Error(), "no PrefabSource set") {
		t.Fatalf("variant without source: %v", err)
	}
}

// TestPrefabExpandOverrides covers the exported merge and its inverse, as
// the level editor uses them.
func TestPrefabExpandOverrides(t *testing.T) {
	_, m, _ := prefabWorld(t)
	a := m["archer"]
	got, err := a.Expand("", "", map[string]json.RawMessage{
		"t:Pos":    json.RawMessage(`{"X":7}`),
		"t:Health": json.RawMessage(`null`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if string(got["t:Pos"]) != `{"X":7,"Y":0}` || string(got["t:Sprite"]) != `{"ID":"archer.png","Layer":2}` || got["t:Health"] != nil {
		t.Fatalf("Expand = %s", got)
	}
	ov, err := a.Overrides(got)
	if err != nil {
		t.Fatal(err)
	}
	if len(ov) != 2 || string(ov["t:Pos"]) != `{"X":7}` || string(ov["t:Health"]) != `null` {
		t.Fatalf("Overrides = %s", ov)
	}
	if _, err := a.Expand("pf", "Other", nil); err == nil || !strings.Contains(err.Error(), `object pf:Other does not match prefab "archer"`) {
		t.Fatalf("class mismatch: %v", err)
	}
	if k := PrefabTag("archer").Key(); k != "archer" {
		t.Fatalf("PrefabTag(archer).Key() = %q", k)
	}
}

func TestParsePrefabCycle(t *testing.T) {
	_, m, _ := prefabWorld(t)
	// Re-parsing goblin as a variant of archer (a variant of goblin) closes
	// a cycle.
	_, err := ParsePrefab("goblin", []byte(`{"objects":[{"prefab":"archer"}]}`), m)
	if err == nil || !strings.Contains(err.Error(), "prefab cycle: goblin -> archer -> goblin") {
		t.Fatalf("cycle: %v", err)
	}

	// A chain of maxPrefabDepth prefabs is fine, one more is not.
	key := "goblin"
	for i := range maxPrefabDepth {
		next := "v" + string(rune('a'+i))
		err := m.Add(next, []byte(`{"objects":[{"prefab":"`+key+`"}]}`))
		if i < maxPrefabDepth-2 && err != nil {
			t.Fatalf("depth %d: %v", i, err)
		}
		if err != nil {
			if !strings.Contains(err.Error(), "variant chain deeper than 16") {
				t.Fatalf("depth error: %v", err)
			}
			return
		}
		key = next
	}
	t.Fatal("no depth error")
}

func TestDeserializePrefabInstances(t *testing.T) {
	w, _, res := prefabWorld(t)
	doc := `{"objects":[
		{"id":"g1","prefab":"goblin","components":{"t:Pos":{"X":10,"Y":20}}},
		{"id":"g2","prefab":"goblin","components":{"t:Pos":{"X":30,"Y":20},"t:Health":null,"t:Follow":{"Target":"g1","Speed":2}}},
		{"id":"a1","prefab":"archer","group":"pf","class":"Unit"},
		{"id":"plain","group":"pf","class":"Other","components":{"t:Pos":{"X":1,"Y":1}}}
	]}`
	objs, err := Deserialize(w, strings.NewReader(doc))
	if err != nil {
		t.Fatal(err)
	}
	wantPrefab := []string{"goblin", "goblin", "archer", ""}
	for i, o := range objs {
		if o.Prefab != wantPrefab[i] {
			t.Errorf("objs[%d].Prefab = %q, want %q", i, o.Prefab, wantPrefab[i])
		}
		key, ok := PrefabOf(w, o.Entity)
		if key != wantPrefab[i] || ok != (wantPrefab[i] != "") {
			t.Errorf("PrefabOf(objs[%d]) = %q, %v", i, key, ok)
		}
		if meta, _ := ClassOf(w, o.Entity); meta != o.SchemaMeta {
			t.Errorf("ClassOf(objs[%d]) = %v, want %v", i, meta, o.SchemaMeta)
		}
	}
	g1, g2, a1 := objs[0].Entity, objs[1].Entity, objs[2].Entity
	if p := ecs.GetComp[pfPos](w, g1); *p != (pfPos{10, 20}) {
		t.Errorf("g1 Pos = %+v", *p)
	}
	if h := ecs.GetComp[pfHealth](w, g1); *h != (pfHealth{5, 5}) {
		t.Errorf("g1 Health = %+v", *h)
	}
	if ecs.HasComp[pfHealth](w, g2) {
		t.Error("g2 kept Health, which its override removed")
	}
	if f := ecs.GetComp[testFollow](w, g2); f.Target != g1 {
		t.Errorf("g2 Follow.Target = %v, want %v", f.Target, g1)
	}
	if h := ecs.GetComp[pfHealth](w, a1); *h != (pfHealth{5, 8}) {
		t.Errorf("a1 Health = %+v", *h)
	}
	if s := ecs.GetComp[testSprite](w, a1); s.Layer != 2 || res.paths[s.ID.index-1] != "archer.png" {
		t.Errorf("a1 Sprite = %+v (%v)", *s, res.paths)
	}

	for _, tc := range []struct{ name, doc, want string }{
		{"unknown prefab", `{"objects":[{"prefab":"nope"}]}`, `objects[0]: prefab "nope": unknown prefab`},
		{"class mismatch", `{"objects":[{"prefab":"goblin","group":"pf","class":"Other"}]}`, `objects[0]: object pf:Other does not match prefab "goblin"`},
		{"bad override", `{"objects":[{"prefab":"goblin","components":{"t:Pos":{"X":true}}}]}`, "objects[0]: component t:Pos: $.X: expected number, got boolean"},
		{"unknown component", `{"objects":[{"prefab":"goblin","components":{"Nope":{}}}]}`, "objects[0]: object pf:Unit: component Nope is not registered"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			n := countEntities(w)
			_, err := Deserialize(w, strings.NewReader(tc.doc))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("got %v, want %q", err, tc.want)
			}
			if countEntities(w) != n {
				t.Fatal("a failed Deserialize left entities behind")
			}
		})
	}

	bare := ecs.NewWorld()
	if _, err := Deserialize(bare, strings.NewReader(`{"objects":[{"prefab":"goblin"}]}`)); err == nil || !strings.Contains(err.Error(), "no PrefabSource set") {
		t.Fatalf("without source: %v", err)
	}
}

func TestSerializePrefabInstances(t *testing.T) {
	w, _, _ := prefabWorld(t)
	doc := `{"objects":[` +
		`{"id":"g1","prefab":"goblin","components":{"t:Pos":{"X":10,"Y":20}}},` +
		`{"id":"g2","prefab":"goblin","components":{"t:Follow":{"Speed":2,"Target":"g1"},"t:Health":null,"t:Pos":{"X":30}}},` +
		`{"prefab":"archer"},` +
		`{"prefab":"archer","components":{"t:Sprite":{"ID":"","Layer":3}}},` +
		`{"group":"pf","class":"Other","components":{"t:Pos":{"X":1,"Y":1}}}]}`
	objs, err := Deserialize(w, strings.NewReader(doc))
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := Serialize(w, &out, objs...); err != nil {
		t.Fatal(err)
	}
	if out.String() != doc {
		t.Fatalf("round trip:\n got %s\nwant %s", out.String(), doc)
	}

	// Objects finds the prefab of each entity, so a whole-world save keeps
	// the instances.
	all := slices.Collect(Objects(w))
	slices.SortFunc(all, func(a, b Object) int { return int(a.Entity.ID) - int(b.Entity.ID) })
	for i, o := range all {
		if o.Prefab != objs[i].Prefab {
			t.Errorf("Objects()[%d].Prefab = %q, want %q", i, o.Prefab, objs[i].Prefab)
		}
	}

	// A changed field shows up as the only override.
	ecs.GetComp[pfHealth](w, objs[2].Entity).Cur = 1
	out.Reset()
	if err := Serialize(w, &out, objs[2]); err != nil {
		t.Fatal(err)
	}
	if want := `{"objects":[{"prefab":"archer","components":{"t:Health":{"Cur":1}}}]}`; out.String() != want {
		t.Fatalf("changed field:\n got %s\nwant %s", out.String(), want)
	}

	bare := ecs.NewWorld()
	e := bare.NewEntity(pfPos{})
	err = Serialize(bare, &out, Object{Prefab: "goblin", Entity: e})
	if err == nil || !strings.Contains(err.Error(), "objects[0]: prefab \"goblin\": no PrefabSource set") {
		t.Fatalf("without source: %v", err)
	}
}

func TestBoundPrefabSpawn(t *testing.T) {
	w, m, res := prefabWorld(t)
	b, err := m["archer"].Bind(w, nil)
	if err != nil {
		t.Fatal(err)
	}
	if b.Prefab() != m["archer"] {
		t.Fatal("Prefab()")
	}

	e := b.Spawn(pfPos{X: 3, Y: 4})
	if p := ecs.GetComp[pfPos](w, e); *p != (pfPos{3, 4}) {
		t.Errorf("Pos = %+v", *p)
	}
	if h := ecs.GetComp[pfHealth](w, e); *h != (pfHealth{5, 8}) {
		t.Errorf("Health = %+v", *h)
	}
	if s := ecs.GetComp[testSprite](w, e); res.paths[s.ID.index-1] != "archer.png" || s.Layer != 2 {
		t.Errorf("Sprite = %+v", *s)
	}
	if key, _ := PrefabOf(w, e); key != "archer" {
		t.Errorf("PrefabOf = %q", key)
	}
	if meta, _ := ClassOf(w, e); meta.Key() != "pf:Unit" {
		t.Errorf("ClassOf = %v", meta)
	}

	// The next spawn gets the prefab's values again, not the override.
	e2 := b.Spawn()
	if p := ecs.GetComp[pfPos](w, e2); *p != (pfPos{}) {
		t.Errorf("second Pos = %+v", *p)
	}

	// An optional component the prefab lacks can be added, and so can any
	// registered one; an unregistered type cannot.
	e3 := b.Spawn(testFollow{Target: e, Speed: 1})
	if f := ecs.GetComp[testFollow](w, e3); f.Target != e {
		t.Errorf("Follow = %+v", *f)
	}
	e4 := b.Spawn(pfBuff{N: 2})
	if g := ecs.GetComp[pfBuff](w, e4); g.N != 2 {
		t.Errorf("Buff = %+v", *g)
	}
	mustPanic(t, "god: Spawn: god.testRefName is not a registered component", func() { b.Spawn(testRefName{}) })

	// Serialize writes a spawned entity as an instance.
	var out bytes.Buffer
	if err := Serialize(w, &out, Object{Prefab: "archer", Entity: e}); err != nil {
		t.Fatal(err)
	}
	if want := `{"objects":[{"prefab":"archer","components":{"t:Pos":{"X":3,"Y":4}}}]}`; out.String() != want {
		t.Errorf("serialized spawn = %s", out.String())
	}
}

func TestBoundPrefabSelfReference(t *testing.T) {
	w, m, _ := prefabWorld(t)
	if err := m.Add("chaser", []byte(`{"objects":[{"id":"me","prefab":"goblin","components":{"t:Follow":{"Target":"me","Speed":1}}}]}`)); err != nil {
		t.Fatal(err)
	}
	b, err := m["chaser"].Bind(w, nil)
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		e := b.Spawn()
		if f := ecs.GetComp[testFollow](w, e); f.Target != e {
			t.Fatalf("Follow.Target = %v, want itself %v", f.Target, e)
		}
	}
	// An override of the referencing component replaces it.
	e := b.Spawn(testFollow{Speed: 9})
	if f := ecs.GetComp[testFollow](w, e); f.Speed != 9 || f.Target != (ecs.Entity{}) {
		t.Fatalf("overridden Follow = %+v", *f)
	}
}

func TestBoundPrefabBindErrors(t *testing.T) {
	w, m, _ := prefabWorld(t)
	bare := ecs.NewWorld()
	if _, err := m["goblin"].Bind(bare, nil); err == nil || !strings.Contains(err.Error(), `prefab "goblin": component t:Sprite: $.ID: cannot resolve sprite asset "goblin.png": no AssetResolver set`) {
		t.Fatalf("no resolver: %v", err)
	}
	// An explicit resolver wins over the world's.
	own := &fakeResolver{paths: []string{"x", "y", "goblin.png"}}
	b, err := m["goblin"].Bind(w, own)
	if err != nil {
		t.Fatal(err)
	}
	if s := ecs.GetComp[testSprite](w, b.Spawn()); s.ID.index != 3 {
		t.Fatalf("explicit resolver not used: %+v", *s)
	}
}

func TestBoundPrefabSpawnAllocs(t *testing.T) {
	w, m, _ := prefabWorld(t)
	b, err := m["goblin"].Bind(w, nil)
	if err != nil {
		t.Fatal(err)
	}
	b.Spawn()
	if n := testing.AllocsPerRun(1000, func() { b.Spawn() }); n != 0 {
		t.Errorf("Spawn() allocates %v times per run", n)
	}
	var at any = pfPos{X: 1}
	if n := testing.AllocsPerRun(1000, func() { b.Spawn(at) }); n != 0 {
		t.Errorf("Spawn(override) allocates %v times per run", n)
	}
}

func BenchmarkPrefabSpawn(b *testing.B) {
	registerPrefabSchema()
	m := PrefabMap{}
	if err := m.Add("goblin", []byte(goblinPrefab)); err != nil {
		b.Fatal(err)
	}
	w := ecs.NewWorld()
	SetAssetResolver(w, &fakeResolver{})
	bp, err := m["goblin"].Bind(w, nil)
	if err != nil {
		b.Fatal(err)
	}
	var at any = pfPos{X: 1, Y: 2}
	b.ReportAllocs()
	for b.Loop() {
		bp.Spawn(at)
	}
}

func TestPrefabKeys(t *testing.T) {
	keys, err := PrefabKeys([]byte(`{"objects":[{"prefab":"b"},{"group":"g"},{"prefab":"a"},{"prefab":"b"}]}`))
	if err != nil || !slices.Equal(keys, []string{"b", "a"}) {
		t.Fatalf("PrefabKeys = %v, %v", keys, err)
	}
	if _, err := PrefabKeys([]byte(`{`)); err == nil {
		t.Fatal("bad JSON: no error")
	}
}

func TestPrefabName(t *testing.T) {
	_, m, _ := prefabWorld(t)
	named := `{"name":"Goblin","objects":[{"group":"pf","class":"Other","components":{"t:Pos":{"X":0,"Y":0}}}]}`
	namedVariant := `{"objects":[{"prefab":"named"}]}`
	if err := m.Add("named", []byte(named)); err != nil {
		t.Fatal(err)
	}
	if err := m.Add("variant", []byte(namedVariant)); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ key, want string }{
		{"named", "Goblin"},
		{"goblin", ""},
		{"variant", ""}, // a variant does not inherit its base's name
	} {
		if got := m[tc.key].Name(); got != tc.want {
			t.Errorf("%s: Name() = %q, want %q", tc.key, got, tc.want)
		}
	}

	_, err := ParsePrefab("x", []byte(`{"name":5,"objects":[]}`), m)
	if err == nil || !strings.Contains(err.Error(), "cannot unmarshal") {
		t.Errorf("non-string name: %v", err)
	}
	_, err = ParsePrefab("x", []byte(`{"title":"a","objects":[]}`), m)
	if err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Errorf("unknown top-level key: %v", err)
	}
}

func TestDeserializeIgnoresName(t *testing.T) {
	w, _, _ := prefabWorld(t)
	doc := `{"name":"level one","objects":[{"id":"p","group":"pf","class":"Other","components":{"t:Pos":{"X":1,"Y":1}}}]}`
	objs, err := Deserialize(w, strings.NewReader(doc))
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := Serialize(w, &buf, objs...); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(buf.String(), `"name"`) {
		t.Errorf("Serialize wrote a name: %s", buf.String())
	}
}

func TestDocumentName(t *testing.T) {
	for _, tc := range []struct{ name, doc, want string }{
		{"level", `{"name":"Forest","objects":[]}`, "Forest"},
		{"prefab", `{"name":"Goblin","objects":[{"group":"pf","class":"Other"}]}`, "Goblin"},
		{"missing", `{"objects":[]}`, ""},
		{"empty", `{"name":"","objects":[]}`, ""},
		{"not a string", `{"name":5,"objects":[]}`, ""},
		{"null", `{"name":null,"objects":[]}`, ""},
		{"invalid JSON", `{"name":"x"`, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := DocumentName([]byte(tc.doc)); got != tc.want {
				t.Errorf("DocumentName = %q, want %q", got, tc.want)
			}
		})
	}
}

// A prefab may hold any registered component, not only its class's: the
// class is the minimum of an object, not its limit.
func TestPrefabFreeComponents(t *testing.T) {
	w, m, _ := prefabWorld(t)
	if err := m.Add("chest", []byte(`{"objects":[{"group":"pf","class":"Other","components":{
		"t:Pos": {"X": 1, "Y": 2},
		"x:Loot": {"Gold": 7}
	}}]}`)); err != nil {
		t.Fatal(err)
	}
	if raw, ok := m["chest"].Component("x:Loot"); !ok || string(raw) != `{"Gold":7}` {
		t.Fatalf("Component(x:Loot) = %s, %v", raw, ok)
	}

	// Instances inherit the extra component, change it, remove it, or add
	// another one; Serialize writes back exactly that.
	doc := `{"objects":[` +
		`{"prefab":"chest"},` +
		`{"prefab":"chest","components":{"x:Loot":{"Gold":9}}},` +
		`{"prefab":"chest","components":{"x:Loot":null}},` +
		`{"prefab":"chest","components":{"x:Buff":{"N":1}}},` +
		`{"group":"pf","class":"Other","components":{"t:Pos":{"X":0,"Y":0},"x:Buff":{"N":3}}}]}`
	objs, err := Deserialize(w, strings.NewReader(doc))
	if err != nil {
		t.Fatal(err)
	}
	for i, want := range []int{7, 9, -1, 7, -1} {
		l := ecs.GetCompOrNil[pfLoot](w, objs[i].Entity)
		if (l == nil) != (want < 0) || (l != nil && l.Gold != want) {
			t.Errorf("objs[%d] Loot = %v, want %d", i, l, want)
		}
	}
	if b := ecs.GetComp[pfBuff](w, objs[4].Entity); b.N != 3 {
		t.Errorf("plain object Buff = %+v", *b)
	}
	var out bytes.Buffer
	if err := Serialize(w, &out, objs...); err != nil {
		t.Fatal(err)
	}
	if out.String() != doc {
		t.Fatalf("round trip:\n got %s\nwant %s", out.String(), doc)
	}

	// Bind and Spawn copy the extra component; an override replaces it.
	b, err := m["chest"].Bind(w, nil)
	if err != nil {
		t.Fatal(err)
	}
	if l := ecs.GetComp[pfLoot](w, b.Spawn()); l.Gold != 7 {
		t.Errorf("spawned Loot = %+v", *l)
	}
	if l := ecs.GetComp[pfLoot](w, b.Spawn(pfLoot{Gold: 1})); l.Gold != 1 {
		t.Errorf("overridden Loot = %+v", *l)
	}
	var buff any = pfBuff{N: 4}
	b.Spawn(buff) // the first override of an unseen type looks it up once
	if n := testing.AllocsPerRun(1000, func() { b.Spawn(buff) }); n != 0 {
		t.Errorf("Spawn(registered extra) allocates %v times per run", n)
	}

	// The extra component is validated against its registered schema.
	if err := m.Add("bad", []byte(`{"objects":[{"group":"pf","class":"Other","components":{"t:Pos":{"X":0,"Y":0},"x:Loot":{"Gold":"lots"}}}]}`)); err == nil ||
		!strings.Contains(err.Error(), `prefab "bad": component x:Loot: $.Gold: expected integer, got string`) {
		t.Fatalf("bad extra component: %v", err)
	}
}
