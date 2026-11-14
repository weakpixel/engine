//go:build !(js && wasm)

// Command wpix-editor runs the visual editors of the engine on a shared
// shell:
//
//	wpix-editor anim <file>.anim.json  edit an animation definition
//	wpix-editor fx [file.fx.json]      make a particle effect (untitled without a file)
//	wpix-editor view <file>            preview a texture, an atlas, a font or a Tiled map, tileset or world
//	wpix-editor help                   list the tools
//
// Natively it opens a window with the tool's panel drawn by debugui. Built for
// js/wasm (GOOS=js GOARCH=wasm) it runs inside the VSCode extension's webview
// (main_js.go), which draws the panel as HTML and talks to the tool over the
// page's weakpixelHost (see doc/design/editor.md#tools). The wasm build never
// links Cobra or debugui, so a game module that builds it needs no new go.sum
// entries.
//
// Run it from a game module with go tool wpix-editor <tool> <file>; the
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
