// Package paint is a dependency of the defaults case: a named integer type
// with constants, and a nested struct that carries its own defaults.
package paint

// Color is a packed RGBA value; zero means white.
type Color uint32

// Colors.
const (
	White Color = 0
	Red   Color = 0xff0000ff
)

// Vec is a nested struct: its defaults apply wherever it is used.
type Vec struct {
	// +game:default=1
	X float64
	// +game:default=2
	Y float64
}
