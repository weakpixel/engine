package gen

import (
	"go/ast"
	"go/token"
	"strings"
)

// docText is the documentation of a comment group: its text without the
// +game:* marker lines, trimmed. It returns "" for a nil group.
func docText(group *ast.CommentGroup) string {
	if group == nil {
		return ""
	}
	var lines []string
	for line := range strings.Lines(group.Text()) {
		if strings.HasPrefix(strings.TrimSpace(line), markerPrefix) {
			continue
		}
		lines = append(lines, line)
	}
	return strings.TrimSpace(strings.Join(lines, ""))
}

// fieldDoc is the documentation of a struct field: its doc comment, or the
// trailing line comment when there is none.
func fieldDoc(f *ast.Field) string {
	if f == nil {
		return ""
	}
	if doc := docText(f.Doc); doc != "" {
		return doc
	}
	return docText(f.Comment)
}

// indexFields records every struct field of file under the position of the
// identifier that go/types reports as the field's position: each name, or for
// an embedded field the (unqualified) type name.
func indexFields(file *ast.File, fields map[token.Pos]*ast.Field) {
	ast.Inspect(file, func(n ast.Node) bool {
		st, ok := n.(*ast.StructType)
		if !ok {
			return true
		}
		for _, f := range st.Fields.List {
			if len(f.Names) == 0 {
				if id := embeddedIdent(f.Type); id != nil {
					fields[id.Pos()] = f
				}
				continue
			}
			for _, name := range f.Names {
				fields[name.Pos()] = f
			}
		}
		return true
	})
}

// embeddedIdent returns the type name identifier of an embedded field type
// (T, *T, pkg.T, T[A]), or nil.
func embeddedIdent(expr ast.Expr) *ast.Ident {
	switch e := expr.(type) {
	case *ast.Ident:
		return e
	case *ast.StarExpr:
		return embeddedIdent(e.X)
	case *ast.SelectorExpr:
		return e.Sel
	case *ast.IndexExpr:
		return embeddedIdent(e.X)
	case *ast.IndexListExpr:
		return embeddedIdent(e.X)
	}
	return nil
}
