package gen

import (
	"go/ast"
	goparser "go/parser"
	"go/token"
	"testing"
)

func TestDocText(t *testing.T) {
	const src = `package p

// Hero is a hero.
//
// It has health.
// +game:object
// +game:object:group=x
type Hero struct {
	// Stores the hero's health.
	// +game:component
	Health int
	Mana   int // Trailing comment.
	Plain  int
	// +game:embed
	Common
	*Other
}
`
	fset := token.NewFileSet()
	file, err := goparser.ParseFile(fset, "p.go", src, goparser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}
	spec := file.Decls[0].(*ast.GenDecl).Specs[0].(*ast.TypeSpec)
	gd := file.Decls[0].(*ast.GenDecl)
	if got, want := docText(gd.Doc), "Hero is a hero.\n\nIt has health."; got != want {
		t.Errorf("type doc = %q, want %q", got, want)
	}

	fields := map[token.Pos]*ast.Field{}
	indexFields(file, fields)
	st := spec.Type.(*ast.StructType)
	tests := []struct {
		name  string
		field *ast.Field
		want  string
	}{
		{"doc comment without marker", st.Fields.List[0], "Stores the hero's health."},
		{"trailing comment", st.Fields.List[1], "Trailing comment."},
		{"no comment", st.Fields.List[2], ""},
		{"marker only", st.Fields.List[3], ""},
		{"nil field", nil, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := fieldDoc(tt.field); got != tt.want {
				t.Errorf("fieldDoc = %q, want %q", got, tt.want)
			}
		})
	}

	// Lookup by the identifier position, as types.Var.Pos reports it.
	if fields[st.Fields.List[0].Names[0].Pos()] != st.Fields.List[0] {
		t.Error("named field not indexed")
	}
	if fields[st.Fields.List[3].Type.Pos()] != st.Fields.List[3] {
		t.Error("embedded field not indexed")
	}
	if fields[st.Fields.List[4].Type.(*ast.StarExpr).X.Pos()] != st.Fields.List[4] {
		t.Error("pointer embedded field not indexed")
	}
}
