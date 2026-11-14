package gen

import (
	"go/token"
	"go/types"
	"math"
	"reflect"
	"slices"
	"strings"

	"golang.org/x/tools/go/packages"

	"github.com/weakpixel/engine/god"
	"github.com/weakpixel/engine/internal/marker"
)

// packageGroup returns the +game:group:name of pkg, or "" when it has none.
// Problems are reported once per package.
func (p *parser) packageGroup(pkg *packages.Package) string {
	if g, done := p.l.groups[pkg]; done {
		return g.name
	}
	g := &groupInfo{}
	p.l.groups[pkg] = g
	for _, file := range pkg.Syntax {
		set, diags := registry.Collect(file.Doc, marker.DescribesPackage)
		p.diags = append(p.diags, diags...)
		mv, ok := set[markGroup]
		if !ok {
			continue
		}
		name := mv.Value.(groupArgs).Name
		switch {
		case !token.IsIdentifier(name):
			p.errorf(mv.Pos, "%s: +%s name %q is not a valid identifier", pkg.Name, markGroup, name)
		case g.name != "" && g.name != name:
			p.errorf(mv.Pos, "%s: conflicting +%s names %q and %q", pkg.Name, markGroup, g.name, name)
		default:
			g.name = name
		}
	}
	return g.name
}

// resolveMeta applies the group/class defaults of a type declared in pkg.
// args holds the explicit marker values; label and mark name the declaration
// and marker in diagnostics.
func (p *parser) resolveMeta(pkg *packages.Package, typeName string, args objectArgs, pos token.Pos, label, mark string) god.SchemaMeta {
	meta := god.SchemaMeta{Group: args.Group, Class: args.Class}
	for _, v := range []struct{ arg, value string }{{"group", args.Group}, {"class", args.Class}} {
		if v.value != "" && !token.IsIdentifier(v.value) {
			p.errorf(pos, "%s: +%s %s %q is not a valid identifier", label, mark, v.arg, v.value)
		}
	}
	if meta.Group == "" {
		if meta.Group = p.packageGroup(pkg); meta.Group == "" {
			meta.Group = pkg.Name
		}
	}
	if meta.Class == "" {
		meta.Class = typeName
	}
	return meta
}

// componentInfo records (once) the schema of component type named.
func (p *parser) componentInfo(named *types.Named) *ComponentInfo {
	key := types.TypeString(named, nil)
	if info := p.out.Components[key]; info != nil {
		return info
	}
	if decl := p.l.decls[named.Origin().Obj().Pos()]; decl != nil {
		_, diags := decl.typeMarkers()
		p.diags = append(p.diags, diags...)
	}
	schema, diags := componentSchema(p.l, named)
	p.diags = append(p.diags, diags...)
	info := &ComponentInfo{Schema: schema, Pos: named.Obj().Pos(), Type: named, GoType: key}
	if decl := p.l.decls[named.Origin().Obj().Pos()]; decl != nil {
		info.Doc = decl.docText()
	}
	p.componentIdentity(info)
	p.out.Components[key] = info
	return info
}

// componentIdentity sets the group and name of a component type. They
// belong to the declaring package: +game:component:group=g,name=n on the
// type, else the package's +game:group:name, else (group only) the import
// path, and the Go type name.
func (p *parser) componentIdentity(info *ComponentInfo) {
	obj := info.Type.Origin().Obj()
	info.Name = obj.Name()
	decl := p.l.decls[obj.Pos()]
	var args componentArgs
	if decl != nil {
		markers, _ := decl.typeMarkers() // problems were reported by componentInfo
		if mv, ok := markers[markComponent]; ok {
			args = mv.Value.(componentArgs)
			label := decl.pkg.Name + "." + obj.Name()
			for _, v := range []struct{ arg, value string }{{"group", args.Group}, {"name", args.Name}} {
				if v.value != "" && !token.IsIdentifier(v.value) {
					p.errorf(mv.Pos, "%s: +%s %s %q is not a valid identifier", label, markComponent, v.arg, v.value)
				}
			}
		}
	}
	if args.Name != "" {
		info.Name = args.Name
	}
	switch {
	case args.Group != "":
		info.Group, info.ExplicitGroup = args.Group, true
	case decl != nil:
		if g := p.packageGroup(decl.pkg); g != "" {
			info.Group, info.ExplicitGroup = g, true
		}
	}
	if !info.ExplicitGroup {
		info.Group = obj.Pkg().Path()
	}
}

// componentSchema is the JSON schema of a component type.
// It also returns the problems found in +game:default markers.
func componentSchema(l *loaded, named *types.Named) (*god.TypeSchema, []diag) {
	s := &schemaBuilder{l: l, stack: []*types.Named{named}}
	return s.schemaOf(named.Underlying()), s.diags
}

// schemaBuilder derives the encoding/json view of Go types. stack guards
// against recursive types, which are reported as unknown fields.
type schemaBuilder struct {
	l     *loaded // source of field documentation and markers
	stack []*types.Named
	diags []diag // problems in +game:default and +game:editor markers

	// pending are the +game:editor showIf conditions of the struct being
	// described, resolved by checkShowIf.
	pending []pendingShowIf

	// pkg and pos are the scope +game:default values are evaluated in: the
	// package and field being processed.
	pkg *types.Package
	pos token.Pos
}

// unknown is the schema of a value whose shape is not described.
func unknown() *god.TypeSchema {
	return &god.TypeSchema{PreserveUnknownFields: true}
}

// structSchema describes st as an object. Embedded structs without a json name
// are inlined, like encoding/json does.
func (s *schemaBuilder) structSchema(st *types.Struct) *god.TypeSchema {
	out := &god.TypeSchema{Type: "object", Properties: map[string]*god.TypeSchema{}}
	// showIf names siblings, so it is resolved once all of out's properties
	// exist; a nested struct resolves its own conditions first.
	outer := s.pending
	s.pending = nil
	s.addProperties(out, st)
	s.checkShowIf(out, s.pending)
	s.pending = outer
	return out
}

func (s *schemaBuilder) addProperties(out *god.TypeSchema, st *types.Struct) {
	for i := range st.NumFields() {
		v := st.Field(i)
		tag := reflect.StructTag(st.Tag(i)).Get("json")
		markers := s.fieldMarkers(v)
		defaults := fieldDefaults(markers)
		editor, hasEditor := markers[markEditor]
		if tag == "-" {
			// A json:"-" field is normally invisible. +game:asset and
			// +game:runtime put it into the schema anyway (validate.go
			// guarantees the tag and the exported name).
			if prop := s.markedProperty(v); prop != nil {
				out.Properties[v.Name()] = prop // never required: absent means "unset"
				s.rejectDefault(v, defaults, notDescribed(prop))
				if hasEditor {
					s.applyEditor(v, prop, editor)
				}
			} else {
				s.rejectDefault(v, defaults, `a field with json:"-" (it is not in the schema)`)
				s.rejectEditor(v, hasEditor, editor, `a field with json:"-" (it is not in the schema)`)
			}
			continue
		}
		name, opts, _ := strings.Cut(tag, ",")
		if v.Anonymous() && name == "" {
			if inner, ok := structOf(v.Type()); ok {
				s.rejectDefault(v, defaults, "an embedded struct (set its fields in their own struct)")
				s.rejectEditor(v, hasEditor, editor, "an embedded struct (mark its fields in their own struct)")
				s.addProperties(out, inner)
				continue
			}
		}
		if !v.Exported() {
			s.rejectDefault(v, defaults, "an unexported field (it is not in the schema)")
			s.rejectEditor(v, hasEditor, editor, "an unexported field (it is not in the schema)")
			continue
		}
		if name == "" {
			name = v.Name()
		}
		prop := s.typeSchema(v.Type())
		prop.Description = fieldDoc(s.l.fields[v.Pos()])
		if slices.Contains(strings.Split(opts, ","), "string") {
			prop = unknown() // ",string" changes the wire type of scalars
		}
		if len(defaults) > 0 {
			s.applyDefaults(v, prop, defaults)
		}
		if hasEditor {
			s.applyEditor(v, prop, editor)
		}
		out.Properties[name] = prop
		// A property is required when it is always written and cannot be null.
		if !prop.Nullable && !prop.PreserveUnknownFields &&
			!slices.Contains(strings.Split(opts, ","), "omitempty") && !slices.Contains(strings.Split(opts, ","), "omitzero") {
			out.Required = append(out.Required, name)
		}
	}
}

// markedProperty is the schema of a json:"-" field marked +game:asset or
// +game:runtime, or nil when it has neither marker.
//
// An asset field is a string key in JSON (the Go field holds a handle); a
// runtime field keeps its normal schema and is flagged as not authored data.
func (s *schemaBuilder) markedProperty(v *types.Var) *god.TypeSchema {
	field := s.l.fields[v.Pos()]
	if field == nil || !v.Exported() {
		return nil
	}
	markers, _ := registry.Collect(field.Doc, marker.DescribesField)
	var prop *god.TypeSchema
	switch {
	case markers[markAsset].Pos.IsValid():
		prop = &god.TypeSchema{Type: "string", Format: "asset", XAsset: markers[markAsset].Value.(assetArgs).Kind}
	case markers[markRuntime].Pos.IsValid():
		prop = s.typeSchema(v.Type())
		prop.XRuntime = true
	default:
		return nil
	}
	prop.Description = fieldDoc(field)
	return prop
}

// structOf unwraps T / *T to its struct type.
func structOf(t types.Type) (*types.Struct, bool) {
	t = types.Unalias(t)
	if ptr, ok := t.(*types.Pointer); ok {
		t = types.Unalias(ptr.Elem())
	}
	st, ok := t.Underlying().(*types.Struct)
	return st, ok
}

// typeSchema describes a field type; pointers are nullable.
func (s *schemaBuilder) typeSchema(t types.Type) *god.TypeSchema {
	t = types.Unalias(t)
	ptr, isPtr := t.(*types.Pointer)
	if isPtr {
		t = types.Unalias(ptr.Elem())
	}
	out := s.schemaOf(t)
	if isPtr {
		out.Nullable = true
	}
	return out
}

func (s *schemaBuilder) schemaOf(t types.Type) *god.TypeSchema {
	if isEntity(t) {
		return entitySchema()
	}
	if named, ok := t.(*types.Named); ok {
		if slices.Contains(s.stack, named) {
			return unknown()
		}
		if hasCustomJSON(named) {
			return &god.TypeSchema{GoType: types.TypeString(named, func(p *types.Package) string { return p.Name() })}
		}
		s.stack = append(s.stack, named)
		defer func() { s.stack = s.stack[:len(s.stack)-1] }()
	}
	switch u := t.Underlying().(type) {
	case *types.Basic:
		out := basicSchema(u)
		if named, ok := t.(*types.Named); ok && out.Type == "integer" && s.l.isEnum(named) && validEnum(named) {
			out.XEnum = s.l.enumValues(named)
		}
		return out
	case *types.Struct:
		return s.structSchema(u)
	case *types.Slice:
		if b, ok := u.Elem().Underlying().(*types.Basic); ok && b.Kind() == types.Uint8 {
			return &god.TypeSchema{Type: "string", Format: "byte", Nullable: true}
		}
		return &god.TypeSchema{Type: "array", Nullable: true, Items: s.typeSchema(u.Elem())}
	case *types.Array:
		n := int(u.Len())
		return &god.TypeSchema{Type: "array", Items: s.typeSchema(u.Elem()), MinItems: new(n), MaxItems: new(n)}
	case *types.Map:
		key, ok := u.Key().Underlying().(*types.Basic)
		if !ok || key.Info()&(types.IsString|types.IsInteger) == 0 {
			return unknown()
		}
		return &god.TypeSchema{Type: "object", Nullable: true, AdditionalProperties: s.typeSchema(u.Elem())}
	}
	return unknown()
}

// entityPath is the import path of the package that declares ecs.Entity.
const entityPath = "github.com/weakpixel/ecs"

// isEntity reports whether t is the named type ecs.Entity, which is stored as
// a reference to another object of the same document.
func isEntity(t types.Type) bool {
	named, ok := types.Unalias(t).(*types.Named)
	if !ok {
		return false
	}
	obj := named.Obj()
	return obj.Name() == "Entity" && obj.Pkg() != nil && obj.Pkg().Path() == entityPath
}

// entitySchema is the schema of an entity reference: a document-local object
// id, or null for the zero Entity.
func entitySchema() *god.TypeSchema {
	return &god.TypeSchema{Type: "string", Format: god.FormatEntity, Nullable: true}
}

func basicSchema(b *types.Basic) *god.TypeSchema {
	switch info := b.Info(); {
	case info&types.IsBoolean != 0:
		return &god.TypeSchema{Type: "boolean"}
	case info&types.IsString != 0:
		return &god.TypeSchema{Type: "string"}
	case info&types.IsFloat != 0:
		format := "double"
		if b.Kind() == types.Float32 {
			format = "float"
		}
		return &god.TypeSchema{Type: "number", Format: format}
	case info&types.IsInteger != 0:
		return integerSchema(b)
	}
	return unknown()
}

// integerSchema bounds an integer type; 64-bit bounds are not representable
// exactly as float64 and are left open (unsigned ones keep minimum 0).
func integerSchema(b *types.Basic) *god.TypeSchema {
	out := &god.TypeSchema{Type: "integer", Format: "int64"}
	unsigned := b.Info()&types.IsUnsigned != 0
	bits := map[types.BasicKind]int{
		types.Int8: 8, types.Uint8: 8, types.Int16: 16, types.Uint16: 16,
		types.Int32: 32, types.Uint32: 32,
	}[b.Kind()]
	if bits != 0 && bits <= 32 {
		out.Format = "int32"
		if unsigned {
			out.Minimum, out.Maximum = new(0.0), new(math.Pow(2, float64(bits))-1)
		} else {
			out.Minimum, out.Maximum = new(-math.Pow(2, float64(bits-1))), new(math.Pow(2, float64(bits-1))-1)
		}
	} else if unsigned {
		out.Minimum = new(0.0)
	}
	return out
}

// hasCustomJSON reports whether encoding/json uses a Marshaler or TextMarshaler
// for named, so its wire shape is not described by its Go layout.
func hasCustomJSON(named *types.Named) bool {
	ms := types.NewMethodSet(types.NewPointer(named))
	for _, name := range []string{"MarshalJSON", "MarshalText"} {
		if ms.Lookup(nil, name) != nil {
			return true
		}
	}
	return false
}
