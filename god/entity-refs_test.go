package god

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/weakpixel/ecs"
)

// testLinks has entity references in every shape the generator emits.
type testLinks struct {
	Target ecs.Entity
	Owner  *ecs.Entity
	Group  []ecs.Entity
	Pair   [2]ecs.Entity
	ByName map[string]ecs.Entity
	Nested struct{ Parent ecs.Entity }
	Weight float64
}

func refSchema() *TypeSchema {
	return &TypeSchema{Type: "string", Format: FormatEntity, Nullable: true}
}

func linksSchema() *TypeSchema {
	return &TypeSchema{
		Type: "object",
		Properties: map[string]*TypeSchema{
			"Target": refSchema(),
			"Owner":  refSchema(),
			"Group":  {Type: "array", Nullable: true, Items: refSchema()},
			"Pair":   {Type: "array", Items: refSchema(), MinItems: new(2), MaxItems: new(2)},
			"ByName": {Type: "object", Nullable: true, AdditionalProperties: refSchema()},
			"Nested": {Type: "object", Properties: map[string]*TypeSchema{"Parent": refSchema()}},
			"Weight": {Type: "number"},
		},
	}
}

// mapRefsTable is a Refs over a fixed id <-> entity table.
type mapRefsTable map[string]ecs.Entity

func (m mapRefsTable) RefEntity(id string) (ecs.Entity, error) {
	return ObjectRefs(m.objects()).RefEntity(id)
}
func (m mapRefsTable) RefID(e ecs.Entity) (string, error) { return ObjectRefs(m.objects()).RefID(e) }

func (m mapRefsTable) objects() []Object {
	var objs []Object
	for id, e := range m {
		objs = append(objs, Object{ID: id, Entity: e})
	}
	return objs
}

func TestRefsHasEntityRef(t *testing.T) {
	if !hasEntityRef(linksSchema()) {
		t.Fatal("links schema has references")
	}
	if hasEntityRef(&TypeSchema{Type: "object", Properties: map[string]*TypeSchema{"X": {Type: "number"}}}) {
		t.Fatal("plain schema has no references")
	}
	if hasEntityRef(nil) {
		t.Fatal("nil schema has no references")
	}
}

func TestRefsRoundTrip(t *testing.T) {
	a, b := ecs.Entity{ID: 3, Generation: 1}, ecs.Entity{ID: 7, Generation: 2}
	refs := mapRefsTable{"a": a, "b": b}
	doc := `{"Target":"a","Owner":null,"Group":["a","b"],"Pair":["b",null],"ByName":{"x":"b"},"Nested":{"Parent":"a"},"Weight":1.25}`

	raw, err := refsToEntities(linksSchema(), []byte(doc), refs)
	if err != nil {
		t.Fatal(err)
	}
	var v testLinks
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatal(err)
	}
	if v.Target != a || v.Owner != nil || v.Group[1] != b || v.Pair[0] != b || v.Pair[1] != (ecs.Entity{}) ||
		v.ByName["x"] != b || v.Nested.Parent != a || v.Weight != 1.25 {
		t.Fatalf("decoded %+v", v)
	}

	data, _ := json.Marshal(v)
	out, err := entitiesToRefs(linksSchema(), data, refs)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"ByName":{"x":"b"},"Group":["a","b"],"Nested":{"Parent":"a"},"Owner":null,"Pair":["b",null],"Target":"a","Weight":1.25}`
	if string(out) != want {
		t.Fatalf("encoded\n%s\nwant\n%s", out, want)
	}
}

func TestRefsErrors(t *testing.T) {
	refs := mapRefsTable{"a": {ID: 1, Generation: 1}}
	_, err := refsToEntities(linksSchema(), []byte(`{"Target":"zz","Group":["a","yy"],"Nested":{"Parent":"a"},"Pair":[null,null]}`), refs)
	for _, want := range []string{`$.Target: unknown object id "zz"`, `$.Group[1]: unknown object id "yy"`} {
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Fatalf("error %v lacks %q", err, want)
		}
	}

	_, err = refsToEntities(linksSchema(), []byte(`{"Target":"a","Nested":{"Parent":null},"Pair":[null,null]}`), nil)
	if err == nil || !strings.Contains(err.Error(), "$.Target: no Refs for entity reference") {
		t.Fatalf("nil refs: %v", err)
	}

	data, _ := json.Marshal(testLinks{Target: ecs.Entity{ID: 9, Generation: 9}})
	_, err = entitiesToRefs(linksSchema(), data, refs)
	if err == nil || !strings.Contains(err.Error(), "$.Target: entity") {
		t.Fatalf("unknown entity: %v", err)
	}
	if _, err := entitiesToRefs(linksSchema(), data, nil); err == nil || !strings.Contains(err.Error(), "no Refs") {
		t.Fatalf("nil refs on encode: %v", err)
	}
	// Zero references need no Refs.
	zero, _ := json.Marshal(testLinks{})
	if _, err := entitiesToRefs(linksSchema(), zero, nil); err != nil {
		t.Fatalf("zero refs: %v", err)
	}
}
