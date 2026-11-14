// +game:group:name=extmod
//
// Package extmod is a separate module (own go.mod, replace directive onto
// the enclosing weakpixel checkout) used to prove the generator and its
// generated code work from outside the weakpixel module: it declares an
// object using a component from its own package and one from
// github.com/weakpixel/engine/gfx.
package extmod

//go:generate go run github.com/weakpixel/engine/tools/wpix gen
