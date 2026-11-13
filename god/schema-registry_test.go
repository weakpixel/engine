package god

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/weakpixel/ecs"
)

func mustPanic(t *testing.T, want string, f func()) {
	t.Helper()
	defer func() {
		r := recover()
		if r == nil {
			t.Fatalf("expected panic containing %q", want)
		}
		if msg, _ := r.(string); !strings.Contains(msg, want) {
			t.Fatalf("panic %v does not contain %q", r, want)
		}
	}()
	f()
}

type testPos struct{ X, Y int }

type testTag struct{ Name string }

func testComponent[T any](name string, id ecs.ComponentID) Component {
	return Component{
		Group:     "t",
		Name:      name,
		RuntimeID: id,
		Schema:    &TypeSchema{Type: "object", PreserveUnknownFields: true},
		Decode:    DecodeFunc[T](),
	}
}

func TestRegisterSchemaDuplicates(t *testing.T) {
	s := Schema{
		Group: "sdup", Class: "o",
		Components: []Component{testComponent[testPos]("Pos", 1)},
	}
	RegisterSchema(s)
	RegisterSchema(s) // identical: accepted

	changed := s
	changed.Components = []Component{testComponent[testPos]("Pos", 2)}
	mustPanic(t, "sdup:o", func() { RegisterSchema(changed) })

	optional := testComponent[testPos]("Pos", 1)
	optional.Optional = true
	changed.Components = []Component{optional}
	mustPanic(t, "sdup:o", func() { RegisterSchema(changed) })
}

// TestRegisterSchemaIgnoresDocs checks that documentation is not part of a
// schema's identity.
func TestRegisterSchemaIgnoresDocs(t *testing.T) {
	s := Schema{
		Group: "sdoc", Class: "o",
		Components: []Component{testComponent[testPos]("Pos", 1)},
	}
	RegisterSchema(s)

	documented := s
	documented.Doc = "An object."
	c := testComponent[testPos]("Pos", 1)
	c.Doc = "Where it is."
	c.EmbeddedVia = []string{"Common"}
	c.Schema = &TypeSchema{Type: "object", PreserveUnknownFields: true, Description: "A position."}
	documented.Components = []Component{c}
	RegisterSchema(documented) // no panic
}

func TestWithoutDescriptions(t *testing.T) {
	s := &TypeSchema{
		Type: "object", Description: "root",
		Properties: map[string]*TypeSchema{
			"A": {Type: "array", Description: "a", Items: &TypeSchema{Type: "string", Description: "item"}},
			"B": {Type: "object", AdditionalProperties: &TypeSchema{Type: "integer", Description: "v"}},
			"C": {Type: "integer", XEnum: []EnumValue{{Value: 0, Name: "Zero", Doc: "none"}}},
		},
	}
	got, _ := json.Marshal(s.WithoutDescriptions())
	want := `{"type":"object","properties":{"A":{"type":"array","items":{"type":"string"}},"B":{"type":"object","additionalProperties":{"type":"integer"}},"C":{"type":"integer","x-enum":[{"value":0,"name":"Zero"}]}}}`
	if string(got) != want {
		t.Fatalf("got %s", got)
	}
	if s.Description != "root" || s.Properties["A"].Description != "a" || s.Properties["C"].XEnum[0].Doc != "none" {
		t.Fatal("receiver was modified")
	}
	if (*TypeSchema)(nil).WithoutDescriptions() != nil {
		t.Fatal("nil must stay nil")
	}
}

func TestRegisterSchemaValidation(t *testing.T) {
	meta := SchemaMeta{Group: "val", Class: "o"}
	good := testComponent[testPos]("Pos", 1)

	empty := good
	empty.Name = ""
	noSchema := good
	noSchema.Schema = nil
	noDecode := good
	noDecode.Decode = nil
	both := good
	both.Readonly, both.Optional = true, true

	for _, tc := range []struct {
		name  string
		want  string
		comps []Component
	}{
		{"duplicate name", "duplicate component t:Pos", []Component{good, good}},
		{"empty name", "without a group or name", []Component{empty}},
		{"nil schema", "no schema", []Component{noSchema}},
		{"nil decode", "no decoder", []Component{noDecode}},
		{"readonly and optional", "both readonly and optional", []Component{both}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mustPanic(t, tc.want, func() { RegisterSchema(Schema{SchemaMeta: meta, Components: tc.comps}) })
		})
	}
}

func registerCheckSchema(t *testing.T) string {
	t.Helper()
	tag := testComponent[testTag]("Tag", 2)
	tag.Optional = true
	s := Schema{
		Group: "chk", Class: "o",
		Components: []Component{testComponent[testPos]("Pos", 1), tag, testComponent[testTag]("Name", 3)},
	}
	RegisterSchema(s)
	return s.Key()
}

func TestCheckComponents(t *testing.T) {
	registerCheckSchema(t)
	raw := func(names ...string) map[string]json.RawMessage {
		m := map[string]json.RawMessage{}
		for _, n := range names {
			m[n] = json.RawMessage(`{}`)
		}
		return m
	}

	if err := CheckComponents("chk", "o", raw("t:Pos", "t:Name")); err != nil {
		t.Fatalf("valid instance rejected: %v", err)
	}
	if err := CheckComponents("chk", "o", raw("t:Pos", "t:Name", "t:Tag")); err != nil {
		t.Fatalf("optional component rejected: %v", err)
	}

	err := CheckComponents("chk", "o", raw("Zed", "Alpha"))
	if err == nil {
		t.Fatal("expected errors")
	}
	want := []string{
		"missing required component t:Pos",
		"missing required component t:Name",
		"component Alpha is not registered",
		"component Zed is not registered",
	}
	last := -1
	for _, w := range want {
		i := strings.Index(err.Error(), w)
		if i <= last {
			t.Fatalf("error %q lacks %q in order", err, w)
		}
		last = i
	}

	if err := CheckComponents("chk", "none", nil); err == nil || !strings.Contains(err.Error(), "unknown object chk:none") {
		t.Fatalf("unknown object error = %v", err)
	}
}

func TestSerializeDeserializeRoundTrip(t *testing.T) {
	posID, tagID, nameID := ecs.RegisterComp[testPos](), ecs.RegisterComp[testTag](), ecs.RegisterComp[testTagName]()
	s := Schema{
		Group: "rt", Class: "o",
		Components: []Component{
			testComponent[testPos]("Pos", posID),
			testComponent[testTag]("Tag", tagID),
			testComponent[testTagName]("Name", nameID),
		},
	}
	s.Components[1].Optional = true
	RegisterSchema(s)

	world := ecs.NewWorld()
	doc := `{"objects":[{"group":"rt","class":"o","components":{"t:Pos":{"X":1,"Y":2},"t:Name":{"Name":"a"}}}]}`
	objs, err := Deserialize(world, strings.NewReader(doc))
	if err != nil {
		t.Fatal(err)
	}
	if len(objs) != 1 || objs[0].Group != "rt" || objs[0].Class != "o" {
		t.Fatalf("Deserialize objects = %+v", objs)
	}
	if world.GetCompOrNil(objs[0].Entity, tagID) != nil {
		t.Fatal("absent optional component was created")
	}

	out, err := SerializeComponents(world, "rt", "o", objs[0].Entity)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(out), `{"t:Name":{"Name":"a"},"t:Pos":{"X":1,"Y":2}}`; got != want {
		t.Fatalf("SerializeComponents = %s, want %s", got, want)
	}

	var buf strings.Builder
	if err := Serialize(world, &buf, objs...); err != nil {
		t.Fatal(err)
	}
	if got, want := buf.String(), `{"objects":[{"group":"rt","class":"o","components":{"t:Name":{"Name":"a"},"t:Pos":{"X":1,"Y":2}}}]}`; got != want {
		t.Fatalf("Serialize = %s, want %s", got, want)
	}

	// Serialize never changes the world: deserializing its own output
	// round-trips to the same entity shape.
	again, err := Deserialize(world, strings.NewReader(buf.String()))
	if err != nil {
		t.Fatal(err)
	}
	if len(again) != 1 {
		t.Fatalf("round trip produced %d objects, want 1", len(again))
	}

	_, err = Deserialize(world, strings.NewReader(`{"objects":[{"group":"rt","class":"o","components":{"t:Pos":{}}}]}`))
	if err == nil || !strings.Contains(err.Error(), "missing required component t:Name") {
		t.Fatalf("expected missing required error, got %v", err)
	}

	// Serialize is all-or-nothing on the buffer: an error on one object
	// (here a stale entity further on isn't tested here, but an
	// unregistered group:class is) writes nothing to w.
	var errBuf strings.Builder
	badObjs := append(slices.Clone(objs), Object{SchemaMeta: SchemaMeta{Group: "rt", Class: "nope"}, Entity: objs[0].Entity})
	if err := Serialize(world, &errBuf, badObjs...); err == nil {
		t.Fatal("expected error for unregistered group:class")
	} else if !strings.Contains(err.Error(), "objects[1]: unknown object rt:nope") {
		t.Fatalf("Serialize error = %v", err)
	}
	if errBuf.Len() != 0 {
		t.Fatalf("Serialize wrote %q to w on error", errBuf.String())
	}
}

// testTagName is a second struct type so a schema can hold two components of
// the same shape under different types.
type testTagName struct{ Name string }

func TestObjectComponents(t *testing.T) {
	registerPrefabSchema()
	s, _ := LookupSchema("pf", "Other")
	if err := CheckComponents("pf", "Other", map[string]json.RawMessage{"t:Pos": nil, "x:Loot": nil}); err != nil {
		t.Fatalf("registered extra component rejected: %v", err)
	}
	if c, ok := ComponentOf(s, "t:Pos"); !ok || c.Optional {
		t.Fatalf("ComponentOf(class component) = %+v, %v", c, ok)
	}
	if c, ok := ComponentOf(s, "x:Loot"); !ok || !c.Optional || c.RuntimeID != ecs.RegisterComp[pfLoot]() || c.Decode == nil {
		t.Fatalf("ComponentOf(registered) = %+v, %v", c, ok)
	}
	if _, ok := ComponentOf(s, "x:Nope"); ok {
		t.Fatal("ComponentOf found an unregistered component")
	}
	var keys []string
	for _, c := range ObjectComponents(s, slices.Values([]string{"x:Loot", "x:Nope", "t:Pos", "x:Buff", "x:Loot"})) {
		keys = append(keys, c.Key())
	}
	if want := []string{"t:Pos", "x:Buff", "x:Loot"}; !slices.Equal(keys, want) {
		t.Fatalf("ObjectComponents = %v, want %v", keys, want)
	}
}
