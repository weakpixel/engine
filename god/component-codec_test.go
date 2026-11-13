package god

import (
	"slices"
	"strings"
	"testing"

	"github.com/weakpixel/ecs"
)

type testFollow struct {
	Target ecs.Entity
	Speed  float64
}

var (
	testFollowID = ecs.RegisterComp[testFollow]()
	testNameID   = ecs.RegisterComp[testRefName]()
)

type testRefName struct{ Name string }

func followComponent() Component {
	return Component{
		Group: "t", Name: "Follow", RuntimeID: testFollowID, Decode: DecodeFunc[testFollow](),
		Schema: &TypeSchema{Type: "object", Required: []string{"Speed"}, Properties: map[string]*TypeSchema{
			"Target": refSchema(),
			"Speed":  {Type: "number"},
		}},
	}
}

// registerRefSchema registers "ref:Unit" {Name, ?Follow}.
func registerRefSchema(t *testing.T) {
	t.Helper()
	follow := followComponent()
	follow.Optional = true
	RegisterSchema(Schema{SchemaMeta: SchemaMeta{Group: "ref", Class: "Unit"}, Components: []Component{
		testComponent[testRefName]("Name", testNameID), follow,
	}})
}

func TestDecodeComponent(t *testing.T) {
	w := ecs.NewWorld()
	target := w.NewEntity(testRefName{"t"})
	refs := ObjectRefs([]Object{{ID: "t", Entity: target}})
	c := followComponent()

	v, err := DecodeComponent(w, c, []byte(`{"Target":"t","Speed":2}`), refs)
	if err != nil {
		t.Fatal(err)
	}
	if got := v.(testFollow); got.Target != target || got.Speed != 2 {
		t.Fatalf("decoded %+v", got)
	}

	for _, tc := range []struct {
		name, raw string
		refs      Refs
		want      string
	}{
		{"invalid", `{"Target":"t","Speed":"x"}`, refs, "$.Speed: expected number, got string"},
		{"unknown id", `{"Target":"nope","Speed":1}`, refs, `$.Target: unknown object id "nope"`},
		{"nil refs", `{"Target":"t","Speed":1}`, nil, "$.Target: no Refs for entity reference"},
		{"empty id", `{"Target":"","Speed":1}`, refs, "$.Target: empty entity reference"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := DecodeComponent(w, c, []byte(tc.raw), tc.refs)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("got %v, want %q", err, tc.want)
			}
		})
	}

	// null and nil refs are fine.
	v, err = DecodeComponent(w, c, []byte(`{"Target":null,"Speed":1}`), nil)
	if err != nil || v.(testFollow).Target != (ecs.Entity{}) {
		t.Fatalf("null ref: %v %v", v, err)
	}

	// Asset fields still resolve (fake resolver).
	r := &fakeResolver{}
	SetAssetResolver(w, r)
	sprite := spriteComponent(ecs.RegisterComp[testSprite](), "sprite")
	sv, err := DecodeComponent(w, sprite, []byte(`{"ID":"a.png","Layer":1}`), nil)
	if err != nil || sv.(testSprite).ID == (fakeHandle{}) {
		t.Fatalf("asset decode: %+v %v", sv, err)
	}
	enc, err := EncodeComponent(w, sprite, sv, nil)
	if err != nil || !strings.Contains(string(enc), `"ID":"a.png"`) {
		t.Fatalf("asset encode: %s %v", enc, err)
	}
}

func TestEncodeComponent(t *testing.T) {
	w := ecs.NewWorld()
	target := w.NewEntity(testRefName{"t"})
	c := followComponent()
	refs := ObjectRefs([]Object{{ID: "t", Entity: target}})

	got, err := EncodeComponent(w, c, testFollow{Target: target, Speed: 1.5}, refs)
	if err != nil || string(got) != `{"Speed":1.5,"Target":"t"}` {
		t.Fatalf("EncodeComponent = %s, %v", got, err)
	}
	got, err = EncodeComponent(w, c, &testFollow{Speed: 1}, nil)
	if err != nil || string(got) != `{"Speed":1,"Target":null}` {
		t.Fatalf("zero ref = %s, %v", got, err)
	}
	if _, err := EncodeComponent(w, c, testFollow{Target: target}, nil); err == nil || !strings.Contains(err.Error(), "no Refs") {
		t.Fatalf("nil refs: %v", err)
	}
	stale := w.NewEntity(testRefName{})
	w.DeleteEntity(stale)
	if _, err := EncodeComponent(w, c, testFollow{Target: stale}, refs); err == nil || !strings.Contains(err.Error(), "not one of the objects") {
		t.Fatalf("stale ref: %v", err)
	}
	noID := ObjectRefs([]Object{{Entity: target}})
	if _, err := EncodeComponent(w, c, testFollow{Target: target}, noID); err == nil || !strings.Contains(err.Error(), "has no object id") {
		t.Fatalf("object without id: %v", err)
	}
}

const refDoc = `{"objects":[` +
	`{"id":"a","group":"ref","class":"Unit","components":{"t:Name":{"Name":"a"},"t:Follow":{"Target":"b","Speed":1}}},` +
	`{"id":"b","group":"ref","class":"Unit","components":{"t:Name":{"Name":"b"},"t:Follow":{"Target":"b","Speed":2}}},` +
	`{"group":"ref","class":"Unit","components":{"t:Name":{"Name":"c"},"t:Follow":{"Target":null,"Speed":3}}}]}`

func TestDeserializeRefs(t *testing.T) {
	registerRefSchema(t)
	w := ecs.NewWorld()
	objs, err := Deserialize(w, strings.NewReader(refDoc))
	if err != nil {
		t.Fatal(err)
	}
	if objs[0].ID != "a" || objs[1].ID != "b" || objs[2].ID != "" {
		t.Fatalf("ids = %q %q %q", objs[0].ID, objs[1].ID, objs[2].ID)
	}
	follow := func(i int) testFollow { return *ecs.GetComp[testFollow](w, objs[i].Entity) }
	if follow(0).Target != objs[1].Entity {
		t.Fatal("forward reference not resolved")
	}
	if follow(1).Target != objs[1].Entity {
		t.Fatal("self reference not resolved")
	}
	if follow(2).Target != (ecs.Entity{}) {
		t.Fatal("null reference not zero")
	}

	// Loading twice gives independent links.
	again, err := Deserialize(w, strings.NewReader(refDoc))
	if err != nil {
		t.Fatal(err)
	}
	if got := ecs.GetComp[testFollow](w, again[0].Entity).Target; got != again[1].Entity || got == objs[1].Entity {
		t.Fatal("second load must link to its own objects")
	}
}

func TestDeserializeRefErrors(t *testing.T) {
	registerRefSchema(t)
	unit := func(id, target string) string {
		idPart := ""
		if id != "" {
			idPart = `"id":"` + id + `",`
		}
		return `{` + idPart + `"group":"ref","class":"Unit","components":{"t:Name":{"Name":"x"},"t:Follow":{"Target":` + target + `,"Speed":1}}}`
	}
	for _, tc := range []struct{ name, doc, want string }{
		{"unknown id", `{"objects":[` + unit("a", `"zz"`) + `]}`, `objects[0]: component t:Follow: $.Target: unknown object id "zz"`},
		{"duplicate id", `{"objects":[` + unit("a", "null") + `,` + unit("a", "null") + `]}`, `objects[1]: duplicate id "a" (first used by objects[0])`},
		{"later object fails", `{"objects":[` + unit("a", `"a"`) + `,` + unit("b", `"nope"`) + `]}`, `objects[1]: component t:Follow`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := ecs.NewWorld()
			_, err := Deserialize(w, strings.NewReader(tc.doc))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("got %v, want %q", err, tc.want)
			}
			if n := countEntities(w); n != 0 {
				t.Fatalf("%d entities left after a failed Deserialize", n)
			}
		})
	}
}

func countEntities(w *ecs.World) int {
	n := 0
	for range w.Query().Each() {
		n++
	}
	return n
}

func TestSerializeRefs(t *testing.T) {
	registerRefSchema(t)
	w := ecs.NewWorld()
	objs, err := Deserialize(w, strings.NewReader(refDoc))
	if err != nil {
		t.Fatal(err)
	}
	var buf strings.Builder
	if err := Serialize(w, &buf, objs...); err != nil {
		t.Fatal(err)
	}
	want := `{"objects":[` +
		`{"id":"a","group":"ref","class":"Unit","components":{"t:Follow":{"Speed":1,"Target":"b"},"t:Name":{"Name":"a"}}},` +
		`{"id":"b","group":"ref","class":"Unit","components":{"t:Follow":{"Speed":2,"Target":"b"},"t:Name":{"Name":"b"}}},` +
		`{"group":"ref","class":"Unit","components":{"t:Follow":{"Speed":3,"Target":null},"t:Name":{"Name":"c"}}}]}`
	if buf.String() != want {
		t.Fatalf("Serialize =\n%s\nwant\n%s", buf.String(), want)
	}

	// Without ids, referenced objects get "_<index>"; unreferenced get none.
	anon := slices.Clone(objs)
	for i := range anon {
		anon[i].ID = ""
	}
	buf.Reset()
	if err := Serialize(w, &buf, anon...); err != nil {
		t.Fatal(err)
	}
	if s := buf.String(); !strings.Contains(s, `{"id":"_1","group"`) || !strings.Contains(s, `"Target":"_1"`) || strings.Contains(s, `"id":"_0"`) {
		t.Fatalf("auto ids: %s", s)
	}
	// The output loads back with the same links.
	w2 := ecs.NewWorld()
	back, err := Deserialize(w2, strings.NewReader(buf.String()))
	if err != nil {
		t.Fatal(err)
	}
	if ecs.GetComp[testFollow](w2, back[0].Entity).Target != back[1].Entity {
		t.Fatal("auto-id round trip lost the link")
	}

	// An explicit id that collides with an automatic one.
	clash := slices.Clone(anon)
	clash[0].ID = "_1"
	buf.Reset()
	if err := Serialize(w, &buf, clash...); err == nil || !strings.Contains(err.Error(), `duplicate id "_1"`) || buf.Len() != 0 {
		t.Fatalf("collision: %v, wrote %q", err, buf.String())
	}

	// A reference to an object that is not serialized.
	buf.Reset()
	if err := Serialize(w, &buf, objs[0]); err == nil || !strings.Contains(err.Error(), "not one of the serialized objects") || buf.Len() != 0 {
		t.Fatalf("outside ref: %v", err)
	}

	// SerializeComponents has no document: a non-zero reference is an error.
	if _, err := SerializeComponents(w, "ref", "Unit", objs[0].Entity); err == nil || !strings.Contains(err.Error(), "no Refs") {
		t.Fatalf("SerializeComponents: %v", err)
	}
	if _, err := SerializeComponents(w, "ref", "Unit", objs[2].Entity); err != nil {
		t.Fatalf("SerializeComponents with a null ref: %v", err)
	}
}
