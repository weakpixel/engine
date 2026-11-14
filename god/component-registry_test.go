package god

import (
	"encoding/json"
	"slices"
	"testing"

	"github.com/weakpixel/ecs"
)

// testComponentSchema registers nothing; its RuntimeID is T's real one,
// because Serialize writes every registered component an entity has.
func testComponentSchema[T any](group, name string) ComponentSchema {
	return ComponentSchema{
		Group: group, Name: name, GoType: "example.com/t." + name,
		RuntimeID: ecs.RegisterComp[T](),
		Schema:    &TypeSchema{Type: "object", PreserveUnknownFields: true},
		Decode:    DecodeFunc[T](),
	}
}

func TestRegisterComponent(t *testing.T) {
	c := testComponentSchema[testPos]("creg", "Pos")
	RegisterComponent(c)
	RegisterComponent(c) // identical: accepted

	documented := c
	documented.Doc = "Where it is."
	documented.Schema = &TypeSchema{Type: "object", PreserveUnknownFields: true, Description: "A position."}
	RegisterComponent(documented) // docs are not compared

	got, ok := LookupComponent("creg", "Pos")
	if !ok || got.Key() != "creg:Pos" || got.GoType != c.GoType {
		t.Fatalf("LookupComponent = %+v, %v", got, ok)
	}
	if _, ok := LookupComponent("creg", "Nope"); ok {
		t.Fatal("LookupComponent found an unregistered component")
	}

	RegisterComponent(testComponentSchema[testTag]("creg", "Tag"))
	var keys []string
	for c := range Components() {
		keys = append(keys, c.Key())
	}
	if !slices.Contains(keys, "creg:Pos") || !slices.Contains(keys, "creg:Tag") || !slices.IsSorted(keys) {
		t.Fatalf("Components() = %v", keys)
	}

	other := testComponentSchema[testTag]("creg", "Pos")
	mustPanic(t, `god: component "creg:Pos" registered twice with different definitions`, func() { RegisterComponent(other) })
}

func TestRegisterComponentValidation(t *testing.T) {
	tests := []struct {
		name string
		edit func(*ComponentSchema)
		want string
	}{
		{"empty group", func(c *ComponentSchema) { c.Group = "" }, "god: component without a group or name"},
		{"empty name", func(c *ComponentSchema) { c.Name = "" }, "god: component without a group or name"},
		{"colon", func(c *ComponentSchema) { c.Name = "a:b" }, `god: component "cval:a:b": group and name must not contain ':'`},
		{"no schema", func(c *ComponentSchema) { c.Schema = nil }, `god: component "cval:X" has no schema`},
		{"no decoder", func(c *ComponentSchema) { c.Decode = nil }, `god: component "cval:X" has no decoder`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := testComponentSchema[testPos]("cval", "X")
			tc.edit(&c)
			mustPanic(t, tc.want, func() { RegisterComponent(c) })
		})
	}
}

func TestComponentJSON(t *testing.T) {
	c := Component{Group: "gfx", Name: "Transform", Field: "Pos", Optional: true, EmbeddedVia: []string{"Common"}}
	data, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"component":"gfx:Transform","field":"Pos","optional":true,"embeddedVia":["Common"]}`
	if string(data) != want {
		t.Fatalf("Marshal = %s, want %s", data, want)
	}
	var back Component
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatal(err)
	}
	if back.Key() != "gfx:Transform" || back.Field != "Pos" || !back.Optional || len(back.EmbeddedVia) != 1 {
		t.Fatalf("Unmarshal = %+v", back)
	}
	if err := json.Unmarshal([]byte(`{"component":"Transform"}`), &back); err == nil {
		t.Fatal("expected an error for a key without a group")
	}
}
