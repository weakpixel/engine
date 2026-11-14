// Package foreign supplies cross-package constructs for the diagnostics
// test: a +game:object with an unexported +game:component field, and an
// exported alias to an unexported type.
package foreign

// +game:object
type Base struct {
	// +game:component
	hp *HP // unexported field name: diag package can't access it
}

type HP struct{ Value int }

type hiddenType struct{ X int }

// VisibleAlias is an exported alias to an unexported named type.
type VisibleAlias = hiddenType
