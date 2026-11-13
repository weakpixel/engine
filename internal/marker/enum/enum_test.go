package enum

import (
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"testing"
)

const src = `package p

type Level int

const (
	High Level = 10
	Low  Level = 0
	Mid  Level = 5
	hidden Level = 7 // unexported
	Alias Level = 5
)

const Other int = 1

type Big uint64

const Huge Big = 1 << 63
`

func TestConsts(t *testing.T) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "p.go", src, 0)
	if err != nil {
		t.Fatal(err)
	}
	pkg, err := (&types.Config{Importer: importer.Default()}).Check("p", fset, []*ast.File{f}, nil)
	if err != nil {
		t.Fatal(err)
	}
	named := func(name string) *types.Named { return pkg.Scope().Lookup(name).Type().(*types.Named) }

	var got []string
	for _, c := range Consts(named("Level")) {
		got = append(got, c.Obj.Name())
	}
	want := []string{"Low", "Alias", "Mid", "High"} // by value, then name
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
	if c := Consts(named("Big")); len(c) != 0 {
		t.Errorf("a constant above int64 must be skipped, got %v", c)
	}
	if c := Consts(types.Universe.Lookup("error").Type().(*types.Named)); c != nil {
		t.Errorf("a type without a package has no constants, got %v", c)
	}
}
