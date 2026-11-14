// Package animedit is the "anim" tool of wpix-editor: an editor for
// *.anim.json animation definitions (doc/guide/engine/animation.md). It plays
// a clip of the definition's sheet and shows the pivot, the body and the hit
// and hurt boxes of the current frame; boxes are drawn, moved, resized and
// deleted with the mouse, everything else the format has is edited in the
// panel.
//
// The model is the animfile.File itself. Every change is a function on the
// File (edit.go), then animfile.Encode, then one Shell.Edit, so the file the
// tool writes is one the loader and "wpix anim validate" accept. The
// files without Ebiten (edit, geometry, playback, panel) hold the logic and
// are tested without a display; tool.go is the glue.
package animedit
