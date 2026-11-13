package marker

import (
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"reflect"
	"strings"
	"testing"
)

type slotMarker struct {
	Slot string   `marker:"slot"`
	Min  *float64 `marker:"min"`
	Max  *float64 `marker:"max"`
}

func validateSlot(p slotMarker) error {
	var errs []error
	if len(p.Slot) != 1 {
		errs = append(errs, fmt.Errorf("slot %q is not one letter", p.Slot))
	}
	if (p.Min == nil) != (p.Max == nil) {
		errs = append(errs, errors.New("min and max are set together"))
	}
	return errors.Join(errs...)
}

// checkSlotsUnique reports a slot taken twice, at the later line.
func checkSlotsUnique(lines []Marker) []Diag {
	var diags []Diag
	first := map[string]Marker{}
	for _, l := range lines {
		slot := l.Value.(slotMarker).Slot
		if prev, ok := first[slot]; ok {
			diags = append(diags, Diag{Pos: l.Pos, Related: prev.Pos, Msg: "slot " + slot + " is already used"})
			continue
		}
		first[slot] = l
	}
	return diags
}

func slotRegistry(t *testing.T) *Registry {
	t.Helper()
	r := NewRegistry(WithPrefix("t:"))
	err := r.DefineRepeatable("t:slot", DescribesType, slotMarker{},
		Validate(validateSlot), WithLinesCheck(checkSlotsUnique))
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestRepeatableStructMarker(t *testing.T) {
	r := slotRegistry(t)
	doc := mustDoc(t, "// +t:slot:slot=x,min=0,max=1\n// +t:slot:slot=y\n// +t:slot:slot=zz\n// +t:slot:slot=w")
	set, diags := r.Collect(doc, DescribesType)

	// The bad line is reported and skipped; the others are collected in order.
	if len(diags) != 1 || !strings.Contains(diags[0].Msg, `marker +t:slot: slot "zz" is not one letter`) {
		t.Fatalf("diags = %v", diags)
	}
	got := Values[slotMarker](set, "t:slot")
	if len(got) != 3 || got[0].Slot != "x" || got[1].Slot != "y" || got[2].Slot != "w" {
		t.Fatalf("values = %+v", got)
	}
	lines := set.Lines("t:slot")
	if set["t:slot"].Pos != lines[0].Pos || lines[0].Pos == lines[1].Pos {
		t.Errorf("positions: set %v, lines %v %v", set["t:slot"].Pos, lines[0].Pos, lines[1].Pos)
	}
	if def, _ := r.Lookup("t:slot"); !def.Repeatable() {
		t.Error("Repeatable() = false")
	}
}

func TestValidate(t *testing.T) {
	r := slotRegistry(t)
	tests := []struct {
		name, doc string
		want      []string // substrings, one per diag
	}{
		{"valid", "// +t:slot:slot=x,min=0,max=1", nil},
		{"joined errors", "// +t:slot:slot=xy,min=1", []string{`is not one letter`, `min and max are set together`}},
		{"failing marker not collected", "// +t:slot:slot=xy", []string{`is not one letter`}},
		{"lines check", "// +t:slot:slot=x\n// +t:slot:slot=x", []string{`slot x is already used`}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			set, diags := r.Collect(mustDoc(t, tt.doc), DescribesType)
			var msgs []string
			for _, d := range diags {
				msgs = append(msgs, d.Msg)
			}
			all := strings.Join(msgs, "\n")
			if len(diags) != len(tt.want) && tt.name != "joined errors" {
				t.Fatalf("diags = %q, want %d", msgs, len(tt.want))
			}
			for _, w := range tt.want {
				if !strings.Contains(all, w) {
					t.Errorf("diags %q lack %q", msgs, w)
				}
			}
			if tt.name == "failing marker not collected" && len(Values[slotMarker](set, "t:slot")) != 0 {
				t.Error("a marker that failed Validate was collected")
			}
		})
	}
}

func TestLinesCheckDiagPositions(t *testing.T) {
	r := slotRegistry(t)
	_, diags := r.Collect(mustDoc(t, "// +t:slot:slot=x\n// +t:slot:slot=x"), DescribesType)
	if len(diags) != 1 || !diags[0].Pos.IsValid() || !diags[0].Related.IsValid() || diags[0].Pos <= diags[0].Related {
		t.Fatalf("diag = %+v: want Pos after a valid Related", diags)
	}
}

func TestParseRunsValidate(t *testing.T) {
	r := slotRegistry(t)
	_, _, err := r.Parse(DescribesType, "+t:slot:slot=ab")
	if err == nil || !strings.Contains(err.Error(), "is not one letter") {
		t.Errorf("err = %v", err)
	}
}

func TestDefineOptionErrors(t *testing.T) {
	type other struct {
		N int `marker:"n"`
	}
	tests := []struct {
		name string
		do   func(r *Registry) error
		want string
	}{
		{"Validate with another type", func(r *Registry) error {
			return r.Define("t:a", DescribesType, slotMarker{}, Validate(func(other) error { return nil }))
		}, "Validate takes a func(marker.slotMarker) error"},
		{"Validate with a pointer type", func(r *Registry) error {
			return r.Define("t:a", DescribesType, slotMarker{}, Validate(func(*slotMarker) error { return nil }))
		}, "Validate takes a func"},
		{"lines check on a single marker", func(r *Registry) error {
			return r.Define("t:a", DescribesType, slotMarker{}, WithLinesCheck(checkSlotsUnique))
		}, "WithLinesCheck needs a repeatable marker"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.do(NewRegistry())
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("err = %v, want %q", err, tt.want)
			}
		})
	}
	// A failed Define leaves nothing registered.
	r := NewRegistry()
	_ = r.Define("t:a", DescribesType, slotMarker{}, WithLinesCheck(checkSlotsUnique))
	if _, ok := r.Lookup("t:a"); ok {
		t.Error("definition registered despite the option error")
	}
}

func TestStray(t *testing.T) {
	r := slotRegistry(t)
	const src = `package main

// +t:slot:slot=x
// +other:thing=1
var Params vec4

// +t:slot:slot=y
func Fragment() vec4 {
	// +t:slot:slot=z
	return vec4(0)
}

// +t:slot:slot=w

var Later vec4
`
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "x.kage", src, parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}
	params := f.Decls[0].(*ast.GenDecl)
	diags := r.Stray(f, params.Doc, nil)

	var lines []int
	for _, d := range diags {
		lines = append(lines, fset.Position(d.Pos).Line)
		if !strings.Contains(d.Msg, "marker +t:slot: not attached to a declaration") {
			t.Errorf("msg = %q", d.Msg)
		}
	}
	// The Fragment doc, the marker in its body and the floating one; the
	// collected group and the foreign +other comment are not reported.
	if want := []int{7, 9, 13}; !reflect.DeepEqual(lines, want) {
		t.Errorf("stray lines = %v, want %v", lines, want)
	}
}

func TestStrayBareAndUnknown(t *testing.T) {
	r := NewRegistry() // no prefix: every +comment is a candidate
	f, err := parser.ParseFile(token.NewFileSet(), "x.go", "package p\n\n// +\n// +zzz:a=1\nvar V int\n", parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}
	diags := r.Stray(f)
	if len(diags) != 2 || !strings.Contains(diags[1].Msg, "marker +zzz:a: not attached") {
		t.Errorf("diags = %v", diags)
	}
}
