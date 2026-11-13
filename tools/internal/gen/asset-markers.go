package gen

import (
	"go/ast"
	"go/token"
	"go/types"
	"reflect"

	"github.com/weakpixel/engine/internal/marker"
)

// validateAssetMarkers enforces the rules of +game:asset and +game:runtime.
// Both are only valid on direct fields of a component struct, so the check
// runs after parsing, when every component type of the package is known.
func (p *parser) validateAssetMarkers(out *Package) {
	components := map[*ast.StructType]bool{}
	for _, info := range out.Components {
		decl := p.l.decls[info.Type.Origin().Obj().Pos()]
		st, isStruct := info.Type.Underlying().(*types.Struct)
		if decl == nil || decl.structType() == nil || !isStruct {
			continue
		}
		components[decl.structType()] = true
		p.checkComponentFieldMarkers(decl, st)
	}

	// Every other struct (objects, nested, inline or unrelated types) must be
	// free of the markers.
	for _, file := range p.target.Syntax {
		for _, decl := range file.Decls {
			gd, ok := decl.(*ast.GenDecl)
			if !ok || gd.Tok != token.TYPE {
				continue
			}
			for _, spec := range gd.Specs {
				ts := spec.(*ast.TypeSpec)
				ast.Inspect(ts.Type, func(n ast.Node) bool {
					if st, ok := n.(*ast.StructType); ok && !components[st] {
						p.rejectAssetMarkers(p.target.Name+"."+ts.Name.Name, st)
					}
					return true
				})
			}
		}
	}
}

// rejectAssetMarkers reports every asset or runtime marker on the fields of st,
// a struct that is not a component's top-level struct.
func (p *parser) rejectAssetMarkers(typeLabel string, st *ast.StructType) {
	for _, field := range st.Fields.List {
		markers, diags := registry.Collect(field.Doc, marker.DescribesField)
		p.diags = append(p.diags, diags...)
		for _, name := range []string{markAsset, markRuntime} {
			if markers[name].Pos.IsValid() {
				p.errorf(field.Pos(), "%s.%s: +%s is only valid on a direct field of a component struct, not on objects or nested, inlined or non-component structs",
					typeLabel, fieldLabel(field), name)
			}
		}
	}
}

// fieldLabel names a struct field for diagnostics.
func fieldLabel(f *ast.Field) string {
	if len(f.Names) > 0 {
		return f.Names[0].Name
	}
	if id := embeddedIdent(f.Type); id != nil {
		return id.Name
	}
	return "?"
}

// checkComponentFieldMarkers checks the +game:asset and +game:runtime fields
// of component struct decl (st is its type-checked form).
func (p *parser) checkComponentFieldMarkers(decl *typeDecl, st *types.Struct) {
	index := 0
	for _, field := range decl.structType().Fields.List {
		markers, diags := registry.Collect(field.Doc, marker.DescribesField)
		p.diags = append(p.diags, diags...)
		_, isAsset := markers[markAsset]
		_, isRuntime := markers[markRuntime]

		for range max(len(field.Names), 1) {
			if index >= st.NumFields() {
				return
			}
			v := st.Field(index)
			tag := reflect.StructTag(st.Tag(index)).Get("json")
			index++
			if !isAsset && !isRuntime {
				continue
			}
			label := decl.pkg.Name + "." + decl.spec.Name.Name + "." + v.Name()
			mark := markAsset
			if isRuntime {
				mark = markRuntime
			}
			switch {
			case isAsset && isRuntime:
				p.errorf(v.Pos(), "%s: +%s and +%s are mutually exclusive", label, markAsset, markRuntime)
			case v.Anonymous():
				p.errorf(v.Pos(), "%s: +%s is not allowed on an embedded field", label, mark)
			case !v.Exported():
				p.errorf(v.Pos(), "%s: +%s requires an exported field", label, mark)
			case tag != "-":
				p.errorf(v.Pos(), `%s: +%s requires the struct tag json:"-" (the field is runtime-only; the schema carries its property)`, label, mark)
			case isAsset && markers[markAsset].Value.(assetArgs).Kind == "":
				p.errorf(v.Pos(), "%s: +%s requires a non-empty kind", label, markAsset)
			case isAsset && !isStructHandle(v.Type()):
				p.errorf(v.Pos(), "%s: +%s requires a struct handle type (not a pointer or non-struct), got %s", label, markAsset, types.TypeString(v.Type(), types.RelativeTo(decl.pkg.Types)))
			}
		}
	}
}

// isStructHandle reports whether t is a (named or literal) non-pointer struct.
func isStructHandle(t types.Type) bool {
	_, ok := types.Unalias(t).Underlying().(*types.Struct)
	return ok
}
