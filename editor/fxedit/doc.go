// Package fxedit is the "fx" tool of weakpixel-editor: it makes a particle
// effect, an *.fx.json file (package particles), without knowing which
// components are involved. It plays the effect in the shell's engine, shows
// the fields that matter as a panel and writes the whole file back in
// canonical form (fxfile.Encode).
//
// The preview plays the model through the engine's own effect loader and
// particle system: the tool serves the current text from the shell's
// overlay filesystem and a change is a hot reload of that file.
//
// Everything but tool.go is plain Go without Ebiten: the presets and
// built-in sprites, the edits, the visibility rules and the panel are tested
// without a window. The package never imports debugui; the shell draws the
// panel. See doc/design/editor.md#tools.
package fxedit
