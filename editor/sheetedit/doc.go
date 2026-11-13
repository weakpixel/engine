// Package sheetedit is the "sheet" tool of weakpixel-editor: an editor for
// *.sheet.json grid sheets (doc/guide/engine/asset-kinds.md#grid-sheet). It
// shows the sheet's texture with the grid over it; the cell size, margin and
// spacing are edited in the panel, which also shows the frame count and the
// index of the frame under the mouse.
//
// The model is the sheetfile.File itself. Every change is a function on the
// File, then sheetfile.Encode, then one Shell.Edit, so the file the tool
// writes is one the atlas loader accepts. Everything but tool.go is plain Go
// without Ebiten and is tested without a display. The package never imports
// debugui; the shell draws the panel. See doc/design/editor.md#tools.
package sheetedit
