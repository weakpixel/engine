// +game:group:name=hierarchy
//
// Package hierarchy attaches entities to a parent. A child holds one
// component, [Parent], with the parent entity and a [Local] offset; the
// [System] writes the child's world gfx.Transform and, by default, its draw
// layer from the parent chain every tick. Children of a deleted parent are
// deleted. In Y-sorted layers a child sorts just in front of or behind its
// root. See doc/guide/engine/hierarchy.md.
//
// Package game installs the System; package extract reads [Parent.RootY].
package hierarchy

//go:generate go run github.com/weakpixel/engine/tools/wpix gen
