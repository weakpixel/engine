// Package enum lists the constants of a Go enum type: the exported
// package-level constants of exactly one named integer type. It depends on the
// standard library only and works on any type-checked package, so a tool that
// reads a marker such as `+x:enum` on a type can find the members without
// knowing what they mean.
package enum

import (
	"cmp"
	"go/constant"
	"go/types"
	"slices"
)

// Const is one exported constant of an enum type.
type Const struct {
	Obj   *types.Const
	Value int64
}

// Consts returns the exported package-level constants of exactly type named,
// sorted by value, then name. Constants whose value does not fit an int64 are
// skipped. It returns nil when named has no package.
func Consts(named *types.Named) []Const {
	obj := named.Obj()
	if obj.Pkg() == nil {
		return nil
	}
	scope := obj.Pkg().Scope()
	var out []Const
	for _, name := range scope.Names() {
		c, ok := scope.Lookup(name).(*types.Const)
		if !ok || !c.Exported() || !types.Identical(c.Type(), named) {
			continue
		}
		v, exact := constant.Int64Val(constant.ToInt(c.Val()))
		if !exact {
			continue
		}
		out = append(out, Const{c, v})
	}
	slices.SortFunc(out, func(a, b Const) int {
		return cmp.Or(cmp.Compare(a.Value, b.Value), cmp.Compare(a.Obj.Name(), b.Obj.Name()))
	})
	return out
}
