// Package toolshell is the shell the weakpixel-editor tools run on: a
// [game.Tool] on an engine with no game systems, so assets load through the
// engine's asset server and the content is drawn by the engine's renderer.
// The shell owns the viewport (pan, zoom, grid), the tool's [Panel] and,
// for document tools, the document.
//
// A tool describes its panel as data ([Panel]) and never draws widgets
// itself. Two renderers show the same description: the debugui renderer of
// package debugpanel in the native window, and the VSCode extension, which
// receives the panel as JSON over the tool protocol (see protocol.go) and
// draws HTML inputs. Document tools edit text through [Document]: a file on
// disk with its own undo natively, the host's document in VSCode.
//
// The package imports neither debugui nor the editor host, so the wasm build
// needs no go.sum entries beyond the engine's. See doc/design/editor.md#tools.
package toolshell
