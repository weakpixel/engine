//go:build !(js && wasm)

// Command wpix-editor runs the visual editors of the engine on a shared
// shell:
//
//	wpix-editor <file>                            open the file in the tool of its kind
//	wpix-editor --image <texture> <new>.sheet.json  start a new sheet
//	wpix-editor --sheet <export> <new>.anim.json    start a new animation definition
//	wpix-editor help                              list the file kinds
//
// The file name picks the kind: a kind that can be edited (.sheet.json,
// .anim.json, .fx.json) opens its editor, any other kind its preview; a new
// .fx.json starts from the default effect.
//
// Natively it opens a window with the tool's panel drawn by debugui. Built for
// js/wasm (GOOS=js GOARCH=wasm) it runs inside the VSCode extension's webview
// (main_js.go), which draws the panel as HTML and talks to the tool over the
// page's weakpixelHost (see doc/design/editor.md#tools). The wasm build never
// links Cobra or debugui, so a game module that builds it needs no new go.sum
// entries.
//
// Run it from a game module with go tool wpix-editor <file>; the
// module needs the tool directive (go get -tool
// github.com/weakpixel/engine/tools/wpix-editor, which wpix new
// adds). The asset root is assets.root of the .weakpixel.json found from the
// file's folder, or the file's folder without one.
package main

import (
	"os"

	"github.com/weakpixel/engine/tools/wpix-editor/cmd"
)

func main() { os.Exit(cmd.Execute(os.Args[1:], os.Stdout, os.Stderr)) }
