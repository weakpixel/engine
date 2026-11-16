// Command wpix is the single command-line entry point of the engine's
// tools. Each tool is a subcommand; the command tree is in package cmd.
//
// Run it with go run github.com/weakpixel/engine/tools/wpix <command>,
// or as a Go tool directive of the game module (go tool wpix <command>).
// Every subcommand keeps the exit codes 0 ok, 1 failure, 2 usage error. The
// tools live in tools/internal; the command line is their API.
package main

import (
	"os"

	"github.com/weakpixel/engine/tools/wpix/cmd"
)

func main() {
	os.Exit(cmd.Execute(os.Args[1:], os.Stdout, os.Stderr))
}
