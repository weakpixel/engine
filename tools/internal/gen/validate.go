package gen

import (
	"go/types"
	"maps"
	"slices"
	"sort"
	"strings"

	"github.com/weakpixel/engine/internal/marker"
)

// checkTypeDecl enforces the type-level marker rules (1 and 2). It reports
// whether the declaration may be processed further.
func (p *parser) checkTypeDecl(d *typeDecl, label string, isObject, isComponent bool) bool {
	pos := d.spec.Name.Pos()
	switch {
	case isObject && isComponent:
		p.errorf(pos, "%s: a type cannot be both +game:object and +game:component", label)
	case d.structType() == nil || d.spec.Assign.IsValid():
		p.errorf(pos, "%s: +game:object and +game:component must be applied to a struct type", label)
	case d.spec.TypeParams != nil:
		p.errorf(pos, "%s: generic types are not supported", label)
	default:
		return true
	}
	return false
}

// checkNoNestedMarkers enforces rule 3: a component type must not itself
// contain +game:component or +game:embed fields.
func (p *parser) checkNoNestedMarkers(d *typeDecl) {
	st := d.structType()
	if st == nil {
		return
	}
	for _, field := range st.Fields.List {
		markers, diags := registry.Collect(field.Doc, marker.DescribesField)
		p.diags = append(p.diags, diags...)
		_, isComponent := markers[markComponent]
		_, isEmbed := markers[markEmbed]
		if !isComponent && !isEmbed {
			continue
		}
		name := d.spec.Name.Name
		if len(field.Names) > 0 {
			name += "." + field.Names[0].Name
		}
		p.errorf(field.Pos(), "%s.%s: component type must not contain +game:component or +game:embed fields", d.pkg.Name, name)
	}
}

// componentType enforces rule 6 on a component field: the type must be a
// named, non-interface type used as T or *T.
func (p *parser) componentType(f fieldInfo) (named *types.Named, byValue bool, ok bool) {
	t := types.Unalias(f.v.Type())
	if t == types.Typ[types.Invalid] {
		p.errorf(f.v.Pos(), "%s: component type does not resolve", f.label)
		return nil, false, false
	}
	byValue = true
	if ptr, isPtr := t.(*types.Pointer); isPtr {
		byValue = false
		t = types.Unalias(ptr.Elem())
		if _, nested := t.(*types.Pointer); nested {
			p.errorf(f.v.Pos(), "%s: component type must be T or *T, not a nested pointer", f.label)
			return nil, false, false
		}
	}
	named, isNamed := t.(*types.Named)
	switch {
	case !isNamed:
		p.errorf(f.v.Pos(), "%s: component type must be a named type, got %s", f.label, types.TypeString(t, types.RelativeTo(p.target.Types)))
	case types.IsInterface(named):
		p.errorf(f.v.Pos(), "%s: component type must not be an interface", f.label)
	default:
		return named, byValue, true
	}
	return nil, false, false
}

// checkForeignField enforces rule 9 for fields: generated code in the target
// package must be able to name a field of a struct from another package.
func (p *parser) checkForeignField(f fieldInfo) {
	if f.owner.pkg != p.target && !f.v.Exported() {
		p.errorf(f.v.Pos(), "%s: field %s is unexported and cannot be accessed from package %s", f.label, f.v.Name(), p.target.Name)
	}
}

// checkForeignType enforces rule 9 for types declared outside the target.
func (p *parser) checkForeignType(f fieldInfo, named *types.Named, what string) {
	if named.Obj().Pkg() != p.target.Types && !named.Obj().Exported() {
		p.errorf(f.v.Pos(), "%s: %s %s.%s is unexported and cannot be named from package %s",
			f.label, what, named.Obj().Pkg().Name(), named.Obj().Name(), p.target.Name)
	}
}

// validateObject enforces rule 7 after flattening: each component type and
// each flattened field name appears once per object.
func (p *parser) validateObject(obj *Object) {
	label := obj.Pkg.Name + "." + obj.Name
	byType := map[string]Component{}
	byName := map[string]Component{}
	for _, c := range obj.Components {
		key := types.TypeString(c.Type, nil)
		if prev, exists := byType[key]; exists {
			p.errorf(c.Pos, "%s: component type %s is used twice: %s and %s",
				label, c.Type.Obj().Name(), prev.PathString(), c.PathString())
		} else {
			byType[key] = c
		}
		if prev, exists := byName[c.Name]; exists {
			p.errorf(c.Pos, "%s: field name %s collides after flattening: %s and %s",
				label, c.Name, prev.PathString(), c.PathString())
		} else {
			byName[c.Name] = c
		}
	}
}

// formatDiags sorts, deduplicates and formats diagnostics.
func (l *loaded) formatDiags(diags []diag) []string {
	sort.SliceStable(diags, func(i, j int) bool {
		a, b := l.fset.Position(diags[i].Pos), l.fset.Position(diags[j].Pos)
		if a.Filename != b.Filename {
			return a.Filename < b.Filename
		}
		if a.Line != b.Line {
			return a.Line < b.Line
		}
		if a.Column != b.Column {
			return a.Column < b.Column
		}
		return diags[i].Msg < diags[j].Msg
	})
	var out []string
	seen := map[string]bool{}
	for _, d := range diags {
		line := l.position(d.Pos) + ": " + strings.TrimSpace(d.Msg)
		if !seen[line] {
			seen[line] = true
			out = append(out, line)
		}
	}
	return out
}

// checkComponentGroup enforces rule 12: a component used by an object has an
// explicit group (on its type or its package), so no import path ends up as
// a key in a data file.
func (p *parser) checkComponentGroup(f fieldInfo, obj *Object, info *ComponentInfo) {
	if info.ExplicitGroup {
		return
	}
	p.errorf(f.v.Pos(), "%s: component %s (%s) is used by object %s but has no group; add +game:component:group=... or +game:group:name to its package",
		f.label, info.Type.Obj().Name(), info.Type.Obj().Pkg().Path(), obj.Meta.Key())
}

// validateComponentKeys enforces rule 13: group:name keys are unique among
// the component types of the package (declared or used, which covers the
// types of each object).
func (p *parser) validateComponentKeys(out *Package) {
	infos := slices.Collect(maps.Values(out.Components))
	slices.SortFunc(infos, func(a, b *ComponentInfo) int { return strings.Compare(a.GoType, b.GoType) })
	byKey := map[string]*ComponentInfo{}
	for _, info := range infos {
		if prev, exists := byKey[info.Key()]; exists {
			p.errorf(info.Pos, "%s.%s: component key %s is already used by %s", info.Type.Obj().Pkg().Name(), info.Type.Obj().Name(), info.Key(), prev.GoType)
			continue
		}
		byKey[info.Key()] = info
	}
}

// validateKeys enforces rule 11: group:class keys are unique among the
// package's objects.
func (p *parser) validateKeys(out *Package) {
	objects := map[string]*Object{}
	for _, obj := range out.Objects {
		key := obj.Meta.Key()
		if prev, exists := objects[key]; exists {
			p.errorf(obj.Pos, "%s.%s: object key %s is already used by %s", obj.Pkg.Name, obj.Name, key, prev.Name)
			continue
		}
		objects[key] = obj
	}
}
