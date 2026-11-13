// +game:group:name=gfx
//
// Package gfx holds the plain-data side of rendering: value types (Vec2,
// Rect, Color), asset handles and the components that describe how an
// entity looks. It imports only the stdlib-only asset package, neither ecs nor Ebiten, so components stay serializable
// through the Game Object Definition and tools never have to compile an
// engine. Package render draws them; package extract connects the two.
package gfx

//go:generate go run github.com/weakpixel/engine/tools/weakpixel gen
