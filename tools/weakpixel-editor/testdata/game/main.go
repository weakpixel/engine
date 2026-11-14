//go:build !editor

// Command game is the editor test game. Without the editor tag it writes the
// game's asset library into a folder, for editor hosts that serve the assets
// from disk:
//
//	go run ./tools/weakpixel-editor/testdata/game -o build/testgame/assets
//
// With -tags editor it is the editor build (editor.go).
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/weakpixel/engine/tools/weakpixel-editor/testdata/game/testgame"
)

func main() {
	out := flag.String("o", "", "folder to write the asset library to")
	flag.Parse()
	if *out == "" || flag.NArg() > 0 {
		fmt.Fprintln(os.Stderr, "usage: game -o <dir>")
		os.Exit(2)
	}
	for key, f := range testgame.Files() {
		p := filepath.Join(*out, filepath.FromSlash(key))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			log.Fatal(err)
		}
		if err := os.WriteFile(p, f.Data, 0o644); err != nil {
			log.Fatal(err)
		}
	}
}
