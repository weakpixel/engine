package gen

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"slices"
	"strings"

	"golang.org/x/tools/go/packages"

	"github.com/weakpixel/engine/internal/marker"
)

type parser struct {
	l      *loaded
	target *packages.Package
	out    *Package
	diags  []diag
}

func (p *parser) errorf(pos token.Pos, format string, args ...any) {
	p.diags = append(p.diags, diag{Pos: pos, Msg: fmt.Sprintf(format, args...)})
}

// parsePackage builds the model of target from its marked declarations. It
// reports every problem it finds instead of stopping at the first.
func parsePackage(l *loaded, target *packages.Package) (*Package, []diag) {
	p := &parser{l: l, target: target}
	out := &Package{Pkg: target, Components: map[string]*ComponentInfo{}}
	p.out = out

	for _, file := range target.Syntax {
		for _, decl := range file.Decls {
			gd, ok := decl.(*ast.GenDecl)
			if !ok || gd.Tok != token.TYPE {
				continue
			}
			for _, spec := range gd.Specs {
				ts := spec.(*ast.TypeSpec)
				d := l.decls[ts.Name.Pos()]
				markers, diags := d.typeMarkers()
				p.diags = append(p.diags, diags...)
				p.parseType(out, d, markers)
			}
		}
	}

	slices.SortFunc(out.Objects, func(a, b *Object) int { return strings.Compare(a.Name, b.Name) })
	p.validateKeys(out)
	p.validateComponentKeys(out)
	p.validateAssetMarkers(out)
	return out, p.diags
}

func (p *parser) parseType(out *Package, d *typeDecl, markers marker.Set) {
	if mv, ok := markers[markEnum]; ok {
		p.checkEnum(d, mv.Pos)
	}
	_, isObject := markers[markObject]
	_, isComponent := markers[markComponent]
	if !isObject && !isComponent {
		return
	}

	ts := d.spec
	label := p.target.Name + "." + ts.Name.Name
	if !p.checkTypeDecl(d, label, isObject, isComponent) {
		return
	}
	named := namedType(d)
	if named == nil {
		p.errorf(ts.Name.Pos(), "%s: type does not resolve", label)
		return
	}

	if isComponent {
		p.checkNoNestedMarkers(d)
		out.Registered = append(out.Registered, named)
		p.componentInfo(named)
		return
	}

	obj := &Object{Name: ts.Name.Name, Pkg: p.target, Pos: ts.Name.Pos(), Doc: d.docText()}
	obj.Meta = p.resolveMeta(p.target, obj.Name, markers[markObject].Value.(objectArgs), obj.Pos, label, markObject)
	p.walk(obj, d, named.Underlying().(*types.Struct), nil, nil, []*types.TypeName{named.Obj()})
	p.validateObject(obj)
	out.Objects = append(out.Objects, obj)
}

// walk flattens the marked fields of one struct into obj, expanding embeds in
// place. path and ptr describe where in obj the struct lives.
func (p *parser) walk(obj *Object, d *typeDecl, st *types.Struct, path []string, ptr []Embed, stack []*types.TypeName) {
	index := 0
	for _, field := range d.structType().Fields.List {
		markers, diags := registry.Collect(field.Doc, marker.DescribesField)
		p.diags = append(p.diags, diags...)

		for range max(len(field.Names), 1) {
			if index >= st.NumFields() {
				return
			}
			v := st.Field(index)
			index++

			f := fieldInfo{
				owner:     d,
				label:     d.pkg.Name + "." + d.spec.Name.Name + "." + v.Name(),
				v:         v,
				anonymous: len(field.Names) == 0,
			}
			var mv marker.Marker
			mv, f.component = markers[markComponent]
			if f.component && mv.Value.(componentArgs) != (componentArgs{}) {
				p.errorf(mv.Pos, "%s: +game:component takes group and name on the component type, not on a field", f.label)
			}
			_, f.embed = markers[markEmbed]
			if mv, ok := markers[markDefault]; ok {
				p.errorf(mv.Pos, "%s: +%s is only valid on a field of a component struct (or a struct nested in one), not on an object field", f.label, markDefault)
			}
			_, f.optional = markers[markOptional]

			switch {
			case f.component && f.embed:
				p.errorf(v.Pos(), "%s: +game:component and +game:embed are mutually exclusive", f.label)
			case f.embed:
				p.walkEmbed(obj, f, path, ptr, stack)
			case f.component:
				p.walkComponent(obj, f, path, ptr, stack)
			case f.optional:
				p.errorf(v.Pos(), "%s: +game:optional requires +game:component on the same field", f.label)
			}
		}
	}
}

type fieldInfo struct {
	owner     *typeDecl
	label     string
	v         *types.Var
	anonymous bool
	component bool
	embed     bool
	optional  bool
}

func (p *parser) walkComponent(obj *Object, f fieldInfo, path []string, ptr []Embed, stack []*types.TypeName) {
	if f.anonymous {
		p.errorf(f.v.Pos(), "%s: +game:component requires a named field (embedded fields use +game:embed)", f.label)
		return
	}
	named, byValue, ok := p.componentType(f)
	if !ok {
		return
	}
	if f.optional && byValue {
		p.errorf(f.v.Pos(), "%s: +game:optional is not allowed on a value component (absent and zero are indistinguishable)", f.label)
		return
	}
	p.checkForeignField(f)
	p.checkForeignType(f, named, "component type")
	p.checkComponentGroup(f, obj, p.componentInfo(named))
	if decl := p.l.decls[named.Origin().Obj().Pos()]; decl != nil {
		p.checkNoNestedMarkers(decl)
	}

	// stack is the object followed by the embedded objects being walked.
	var via []string
	for _, tn := range stack[1:] {
		via = append(via, tn.Name())
	}

	obj.Components = append(obj.Components, Component{
		Path:      append(slices.Clone(path), f.v.Name()),
		Name:      f.v.Name(),
		Type:      named,
		Optional:  f.optional,
		ByValue:   byValue,
		PtrEmbeds: slices.Clone(ptr),
		Pos:       f.v.Pos(),
		Doc:       fieldDoc(p.l.fields[f.v.Pos()]),
		Via:       via,
	})
}

func (p *parser) walkEmbed(obj *Object, f fieldInfo, path []string, ptr []Embed, stack []*types.TypeName) {
	if f.optional {
		p.errorf(f.v.Pos(), "%s: +game:optional requires +game:component on the same field", f.label)
		return
	}
	named, byPointer, ok := embedTarget(f.v.Type())
	if !ok {
		p.errorf(f.v.Pos(), "%s: +game:embed field must be a +game:object struct (or a pointer to one)", f.label)
		return
	}
	if named.TypeArgs().Len() > 0 {
		p.errorf(f.v.Pos(), "%s: embedding a generic object is not supported", f.label)
		return
	}
	decl := p.l.decls[named.Obj().Pos()]
	if decl == nil || decl.structType() == nil {
		p.errorf(f.v.Pos(), "%s: embedded type %s must be a struct declared with +game:object", f.label, named.Obj().Name())
		return
	}
	markers, diags := decl.typeMarkers()
	p.diags = append(p.diags, diags...)
	if _, isObject := markers[markObject]; !isObject {
		p.errorf(f.v.Pos(), "%s: embedded type %s is not marked +game:object", f.label, named.Obj().Name())
		return
	}
	if slices.Contains(stack, named.Obj()) {
		names := make([]string, 0, len(stack)+1)
		for _, tn := range stack {
			names = append(names, tn.Name())
		}
		names = append(names, named.Obj().Name())
		p.errorf(f.v.Pos(), "%s: embed cycle %s", f.label, strings.Join(names, " -> "))
		return
	}
	p.checkForeignField(f)
	p.checkForeignType(f, named, "embedded type")

	path = append(slices.Clone(path), f.v.Name())
	if byPointer {
		ptr = append(slices.Clone(ptr), Embed{Path: path, Type: named})
	}
	p.walk(obj, decl, named.Underlying().(*types.Struct), path, ptr, append(slices.Clone(stack), named.Obj()))
}

// namedType returns the defined type of a declaration, or nil.
func namedType(d *typeDecl) *types.Named {
	tn, ok := d.pkg.TypesInfo.Defs[d.spec.Name].(*types.TypeName)
	if !ok {
		return nil
	}
	named, _ := tn.Type().(*types.Named)
	return named
}

// embedTarget unwraps T / *T (through aliases) to the named type.
func embedTarget(t types.Type) (named *types.Named, byPointer bool, ok bool) {
	t = types.Unalias(t)
	if ptr, isPtr := t.(*types.Pointer); isPtr {
		t = types.Unalias(ptr.Elem())
		byPointer = true
	}
	named, ok = t.(*types.Named)
	return named, byPointer, ok
}
