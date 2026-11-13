package gen

import (
	"go/token"
	"go/types"

	"github.com/weakpixel/engine/god"
	"github.com/weakpixel/engine/internal/marker/enum"
)

// isEnum reports whether named is declared with +game:enum.
func (l *loaded) isEnum(named *types.Named) bool {
	decl := l.decls[named.Origin().Obj().Pos()]
	if decl == nil {
		return false
	}
	markers, _ := decl.typeMarkers()
	_, ok := markers[markEnum]
	return ok
}

// enumValues is the x-enum of named: one entry per constant, with its
// documentation when the declaring package was loaded with syntax.
func (l *loaded) enumValues(named *types.Named) []god.EnumValue {
	consts := enum.Consts(named)
	out := make([]god.EnumValue, len(consts))
	for i, c := range consts {
		out[i] = god.EnumValue{Value: c.Value, Name: c.Obj.Name(), Doc: l.consts[c.Obj.Pos()]}
	}
	return out
}

// validEnum reports whether named can carry an x-enum: an integer type
// without custom JSON whose constants include 0 and have distinct values.
// checkEnum reports the reasons for the declarations of a target package.
func validEnum(named *types.Named) bool {
	b, ok := named.Underlying().(*types.Basic)
	if !ok || b.Info()&types.IsInteger == 0 || hasCustomJSON(named) || named.TypeParams() != nil {
		return false
	}
	consts := enum.Consts(named)
	hasZero := false
	for i, c := range consts {
		hasZero = hasZero || c.Value == 0
		if i > 0 && consts[i-1].Value == c.Value {
			return false
		}
	}
	return hasZero
}

// checkEnum enforces the +game:enum rules on a declaration of the target
// package. All problems are reported.
func (p *parser) checkEnum(d *typeDecl, pos token.Pos) {
	label := p.target.Name + "." + d.spec.Name.Name
	named := namedType(d)
	if named == nil || d.spec.Assign.IsValid() || d.spec.TypeParams != nil {
		p.errorf(pos, "%s: +%s requires a named, non-generic integer type", label, markEnum)
		return
	}
	if u, ok := named.Underlying().(*types.Basic); !ok || u.Info()&types.IsInteger == 0 {
		p.errorf(pos, "%s: +%s requires a named integer type, got %s", label, markEnum, named.Underlying())
		return
	}
	if hasCustomJSON(named) {
		p.errorf(pos, "%s: +%s on a type with MarshalJSON or MarshalText; its wire value is not the integer", label, markEnum)
	}
	consts := enum.Consts(named)
	if len(consts) == 0 {
		p.errorf(pos, "%s: +%s has no exported constants of type %s", label, markEnum, d.spec.Name.Name)
		return
	}
	hasZero := false
	for i, c := range consts {
		hasZero = hasZero || c.Value == 0
		if i > 0 && consts[i-1].Value == c.Value {
			p.errorf(c.Obj.Pos(), "%s: +%s constants %s and %s have the same value %d", label, markEnum, consts[i-1].Obj.Name(), c.Obj.Name(), c.Value)
		}
	}
	if !hasZero {
		p.errorf(pos, "%s: +%s has no constant with value 0; the zero value would not validate", label, markEnum)
	}
}
