package god

import (
	"encoding/json"
	"os"
	"testing"
)

// TestInitialValue runs the shared cases of testdata/initial-value.json (the
// VSCode extension runs the same file against its own initialValue).
func TestInitialValue(t *testing.T) {
	data, err := os.ReadFile("testdata/initial-value.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Cases []struct {
			Name   string
			Schema *TypeSchema
			Want   json.RawMessage
		}
	}
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	for _, tc := range fixture.Cases {
		t.Run(tc.Name, func(t *testing.T) {
			got := tc.Schema.InitialValue()
			if eq, _ := jsonEqual(got, tc.Want); !eq {
				t.Fatalf("got %s, want %s", got, tc.Want)
			}
		})
	}
}

func TestInitialValueNilSchema(t *testing.T) {
	if got := (*TypeSchema)(nil).InitialValue(); string(got) != "null" {
		t.Errorf("got %s, want null", got)
	}
}

// TestDefaultIsSchemaIdentity: a default is data, not documentation.
func TestDefaultIsSchemaIdentity(t *testing.T) {
	s := &TypeSchema{Type: "number", Description: "doc", Default: json.RawMessage("1")}
	got := s.WithoutDescriptions()
	if got.Description != "" || string(got.Default) != "1" {
		t.Errorf("WithoutDescriptions = %+v, want the default kept and the description dropped", got)
	}
}
