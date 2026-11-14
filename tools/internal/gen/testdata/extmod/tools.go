//go:build tools

// This file exists only so `go mod tidy` keeps the generator (run via the
// //go:generate directive in package.go, which tidy can't see) and its
// dependencies in go.mod/go.sum.
package extmod

import (
	_ "github.com/weakpixel/engine/tools/wpix"
)
