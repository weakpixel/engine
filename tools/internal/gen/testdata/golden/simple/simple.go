// +game:group:name=simple
//
// Package simple is the golden-test baseline: one object, two required
// components, nothing else.
package simple

// +game:object
type Thing struct {
	// +game:component
	Pos *Pos

	// +game:component
	Vel *Vel
}

type Pos struct{ X, Y float64 }

type Vel struct{ X, Y float64 }
