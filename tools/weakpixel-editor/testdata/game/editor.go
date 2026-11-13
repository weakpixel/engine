//go:build editor

package main

import (
	"log"

	"github.com/weakpixel/engine/editor/host"
	"github.com/weakpixel/engine/tools/weakpixel-editor/testdata/game/testgame"
)

// main is the editor build: GOOS=js GOARCH=wasm go build -tags editor. In a
// host page the workspace's files come first; the in-memory library fills in
// the rest.
func main() {
	if err := host.Run(testgame.Game(nil)); err != nil {
		log.Fatal(err)
	}
}
