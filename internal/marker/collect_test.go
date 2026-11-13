package marker

import (
	"go/ast"
	"go/parser"
	"go/token"
	"reflect"
	"strings"
	"testing"
)

// docOf parses src and returns the doc comment of its first type declaration.
func docOf(t *testing.T, src string) (*token.FileSet, *ast.CommentGroup) {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "x.go", "package p\n"+src, parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}
	return fset, f.Decls[0].(*ast.GenDecl).Doc
}

func collectRegistry(t *testing.T, opts ...Option) *Registry {
	t.Helper()
	r := newTestRegistry(t, opts...)
	if err := r.DefinePaths("t:def", DescribesType); err != nil {
		t.Fatal(err)
	}
	return r
}

func TestCollect(t *testing.T) {
	r := collectRegistry(t, WithPrefix("t:"))
	_, doc := docOf(t, `
// Doc text.
//
// +build ignored
// +other:thing=1
// +t:flag trailing words
// +t:args:n=3
// +t:def:A=1
// +t:def:B=2
/* +t:flag */
type T int
`)
	set, diags := r.Collect(doc, DescribesType)
	if len(diags) != 0 {
		t.Fatalf("diags: %v", diags)
	}
	if !set.Has("t:flag") || set.Has("t:other") || len(set) != 3 {
		t.Fatalf("set = %v", set)
	}
	if a, ok := Value[args](set, "t:args"); !ok || a.N != 3 {
		t.Fatalf("args = %+v, %v", a, ok)
	}
	if _, ok := Value[flag](set, "t:args"); ok {
		t.Error("Value must not convert between struct types")
	}
	lines := set.Lines("t:def")
	if len(lines) != 2 || set["t:def"].Pos != lines[0].Pos || lines[0].Pos == lines[1].Pos {
		t.Fatalf("lines = %+v", lines)
	}
	if got := lines[1].Value.(PathArgs).Paths; !reflect.DeepEqual(got, []PathValue{{"B", "2"}}) {
		t.Errorf("second line = %v", got)
	}
}

func TestCollectDiagnostics(t *testing.T) {
	tests := []struct {
		name   string
		prefix string
		doc    string
		want   []string
		kept   string // marker that is still collected
	}{
		{"unknown", "t:", "// +t:nope\n// +t:flag", []string{"unknown marker +t:nope"}, "t:flag"},
		{"foreign ignored", "t:", "// +k8s:thing\n// +t:flag", nil, "t:flag"},
		{"unknown without prefix", "", "// +k8s:thing\n// +t:flag", []string{"unknown marker +k8s:thing"}, "t:flag"},
		{"wrong target", "t:", "// +t:args:n=1\n// +t:args:sub", []string{"marker +t:args:sub is not valid on a type"}, "t:args"},
		{"bad argument", "t:", "// +t:args:n=x", []string{`invalid int "x"`}, ""},
		{"duplicate", "t:", "// +t:flag\n// +t:flag", []string{"duplicate marker +t:flag"}, "t:flag"},
		{"several", "t:", "// +t:nope\n// +t:args", []string{"unknown marker", `missing required argument "n"`}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := collectRegistry(t, WithPrefix(tt.prefix))
			_, doc := docOf(t, tt.doc+"\ntype T int\n")
			set, diags := r.Collect(doc, DescribesType)
			if len(diags) != len(tt.want) {
				t.Fatalf("diags = %v, want %d", diags, len(tt.want))
			}
			for i, d := range diags {
				if !strings.Contains(d.Msg, tt.want[i]) || !d.Pos.IsValid() {
					t.Errorf("diag %d = %+v, want %q", i, d, tt.want[i])
				}
			}
			if tt.kept != "" && !set.Has(tt.kept) {
				t.Errorf("%s was not collected: %v", tt.kept, set)
			}
		})
	}
	if set, diags := collectRegistry(t).Collect(nil, DescribesType); len(set) != 0 || diags != nil {
		t.Errorf("nil group: %v %v", set, diags)
	}
}

func TestPathSettings(t *testing.T) {
	tests := []struct {
		name   string
		doc    string
		want   []PathSetting
		issues []PathIssueKind
	}{
		{"bare", "// +t:def=1", []PathSetting{{Value: "1"}}, nil},
		{"lines", "// +t:def:X=1\n// +t:def:Y=2,Z=3", []PathSetting{{Path: "X", Value: "1"}, {Path: "Y", Value: "2"}, {Path: "Z", Value: "3"}}, nil},
		{"duplicate", "// +t:def:X=1\n// +t:def:X=2", []PathSetting{{Path: "X", Value: "1"}, {Path: "X", Value: "2"}}, []PathIssueKind{PathDuplicate}},
		{"mixed", "// +t:def=1\n// +t:def:X=2", nil, []PathIssueKind{PathMixedForms}},
		{"none", "// +t:flag", nil, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			set, diags := collectRegistry(t).Collect(mustDoc(t, tt.doc), DescribesType)
			if len(diags) != 0 {
				t.Fatal(diags)
			}
			got, issues := PathSettings(set.Lines("t:def"))
			for i := range got {
				if !got[i].Pos.IsValid() {
					t.Errorf("setting %d has no position", i)
				}
				got[i].Pos = 0
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("settings = %+v, want %+v", got, tt.want)
			}
			var kinds []PathIssueKind
			for _, is := range issues {
				kinds = append(kinds, is.Kind)
			}
			if !reflect.DeepEqual(kinds, tt.issues) {
				t.Errorf("issues = %v, want %v", kinds, tt.issues)
			}
		})
	}
}

func mustDoc(t *testing.T, doc string) *ast.CommentGroup {
	t.Helper()
	_, g := docOf(t, doc+"\ntype T int\n")
	return g
}

// A Kage shader is Go syntax, so go/parser reads it and markers work in its
// comments like anywhere else.
func TestCollectKageSource(t *testing.T) {
	const src = `//kage:unit pixels

// +shader:effect:name=outline
package main

// +shader:uniform:name=Time,unit=s
var Time float

// Fragment is the entry point.
func Fragment(dstPos vec4, srcPos vec2, color vec4) vec4 {
	return imageSrc0At(srcPos)
}
`
	r := NewRegistry(WithPrefix("shader:"))
	type effect struct {
		Name string `marker:"name"`
	}
	type uniform struct {
		Name string `marker:"name"`
		Unit string `marker:"unit,optional"`
	}
	if err := r.Define("shader:effect", DescribesPackage, effect{}); err != nil {
		t.Fatal(err)
	}
	if err := r.Define("shader:uniform", DescribesField|DescribesType, uniform{}); err != nil {
		t.Fatal(err)
	}
	f, err := parser.ParseFile(token.NewFileSet(), "outline.kage", src, parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}
	pkgSet, diags := r.Collect(f.Doc, DescribesPackage)
	if len(diags) != 0 {
		t.Fatal(diags)
	}
	if e, _ := Value[effect](pkgSet, "shader:effect"); e.Name != "outline" {
		t.Errorf("package marker = %+v", e)
	}
	varDoc := f.Decls[0].(*ast.GenDecl).Doc
	varSet, diags := r.Collect(varDoc, DescribesType)
	if len(diags) != 0 {
		t.Fatal(diags)
	}
	if u, _ := Value[uniform](varSet, "shader:uniform"); u != (uniform{"Time", "s"}) {
		t.Errorf("uniform = %+v", u)
	}
}
