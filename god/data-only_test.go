package god

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/weakpixel/ecs"
)

// loadDataOnlyFixture registers the pf test schemas and the fixture GOD file,
// which names sample:Health and sample:Tint (no package registers them) and
// the class sample:Mob.
func loadDataOnlyFixture(t *testing.T) {
	t.Helper()
	registerPrefabSchema()
	f, err := os.Open("testdata/data-only.god.json")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := RegisterDataOnly(f); err != nil {
		t.Fatal(err)
	}
}

func TestCheckComponentsDataOnly(t *testing.T) {
	loadDataOnlyFixture(t)
	tests := []struct {
		name  string
		class string
		comps string
		want  string // empty: no error
	}{
		{"data-only passes", "Mob", `{"x:Loot":{"Gold":1},"sample:Health":{"Cur":1,"Max":2}}`, ""},
		{"optional data-only", "Mob", `{"x:Loot":{"Gold":1},"sample:Health":{"Cur":1,"Max":2},"sample:Tint":{"Icon":"a.png"}}`, ""},
		{"missing required", "Mob", `{"x:Loot":{"Gold":1}}`, "missing required component sample:Health"},
		{"wrong type", "Mob", `{"x:Loot":{"Gold":1},"sample:Health":{"Cur":"a","Max":2}}`, "sample:Health"},
		{"enum", "Mob", `{"x:Loot":{"Gold":1},"sample:Health":{"Cur":1,"Max":2,"Kind":7}}`, "sample:Health"},
		{"required property", "Mob", `{"x:Loot":{"Gold":1},"sample:Health":{"Cur":1}}`, "sample:Health"},
		{"unknown key", "Mob", `{"x:Loot":{"Gold":1},"sample:Health":{"Cur":1,"Max":2},"sample:Typo":{}}`, "component sample:Typo is not registered"},
		{"data-only on a registered class", "Unit", `{"x:Loot":{"Gold":1},"t:Sprite":{"ID":"a"},"t:Pos":{"X":1,"Y":2},"sample:Health":{"Cur":1,"Max":2}}`, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			group := "sample"
			if tt.class == "Unit" {
				group = "pf"
			}
			raw := parseRawMap(t, tt.comps)
			err := CheckComponents(group, tt.class, raw)
			switch {
			case tt.want == "" && err != nil:
				t.Fatalf("unexpected error: %v", err)
			case tt.want != "" && (err == nil || !strings.Contains(err.Error(), tt.want)):
				t.Fatalf("error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestDataOnlyRegisteredWins(t *testing.T) {
	loadDataOnlyFixture(t)
	// The fixture lists x:Loot with a string schema; the registered one stays.
	if _, ok := DataOnlyComponent("x:Loot"); ok {
		t.Fatal("registered component reported as data-only")
	}
	if _, ok := DataOnlyComponent("sample:Health"); !ok {
		t.Fatal("sample:Health is not data-only")
	}
	if _, ok := DataOnlyComponent("sample:Nope"); ok {
		t.Fatal("unknown key reported")
	}
	if err := CheckComponents("sample", "Mob", parseRawMap(t, `{"x:Loot":{"Gold":1},"sample:Health":{"Cur":1,"Max":2}}`)); err != nil {
		t.Fatal(err)
	}
	s, ok := LookupSchema("sample", "Mob")
	if !ok || len(s.Components) != 1 || s.Components[0].Key() != "x:Loot" {
		t.Fatalf("class schema = %+v, %v; want the registered component only", s, ok)
	}
}

func TestRegisterDataOnlyErrors(t *testing.T) {
	tests := []struct{ name, in, want string }{
		{"not json", `x`, "data-only file"},
		{"version", `{"version":1}`, "unsupported version 1"},
		{"no schema", `{"version":0,"components":[{"group":"g","name":"n"}]}`, "has no schema"},
		{"no name", `{"version":0,"components":[{"group":"g","schema":{"type":"object"}}]}`, "no group or name"},
		{"class without component", `{"version":0,"objects":[{"group":"g","class":"c","components":[{"component":"g:Missing"}]}]}`, "g:Missing is in no GOD file"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := RegisterDataOnly(strings.NewReader(tt.in))
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want %q", err, tt.want)
			}
		})
	}
	// Registering the fixture again is a no-op.
	loadDataOnlyFixture(t)
	loadDataOnlyFixture(t)
}

const mobPrefab = `{"objects":[{"id":"self","group":"sample","class":"Mob","components":{
	"x:Loot": {"Gold": 0},
	"sample:Health": {"Cur": 5, "Max": 5}
}}]}`

const bigMobPrefab = `{"objects":[{"prefab":"mob","components":{
	"sample:Health": {"Max": 9},
	"sample:Tint": {"Icon": "red.png"}
}}]}`

func TestDataOnlyPrefabs(t *testing.T) {
	loadDataOnlyFixture(t)
	m := PrefabMap{}
	if err := m.Add("mob", []byte(mobPrefab)); err != nil {
		t.Fatal(err)
	}
	if err := m.Add("bigmob", []byte(bigMobPrefab)); err != nil {
		t.Fatalf("variant whose base holds data-only components: %v", err)
	}
	w := ecs.NewWorld()
	SetAssetResolver(w, &fakeResolver{})
	SetPrefabSource(w, m)

	for _, key := range []string{"mob", "bigmob"} {
		p, err := lookupPrefab(m, key)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := p.Bind(w, nil); err != nil {
			t.Fatalf("Bind %s: %v", key, err)
		}
		objs, err := p.ExpandInstance(PrefabEntry{ID: "a"})
		if err != nil {
			t.Fatalf("ExpandInstance %s: %v", key, err)
		}
		if len(objs) != 1 || objs[0].Components["sample:Health"] == nil {
			t.Fatalf("%s: expanded %+v lost the data-only component", key, objs)
		}
	}
	p, _ := lookupPrefab(m, "bigmob")
	objs, _ := p.ExpandInstance(PrefabEntry{ID: "a"})
	if got := string(objs[0].Components["sample:Health"]); !strings.Contains(got, `"Max":9`) || !strings.Contains(got, `"Cur":5`) {
		t.Fatalf("merged data-only value = %s", got)
	}
	if _, ok := objs[0].Components["sample:Tint"]; !ok {
		t.Fatal("variant's data-only component missing")
	}
}

func parseRawMap(t *testing.T, s string) map[string]json.RawMessage {
	t.Helper()
	var m map[string]json.RawMessage
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		t.Fatal(err)
	}
	return m
}
