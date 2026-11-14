//go:build !(js && wasm)

// Package cmd is the command line of wpix-editor: one subcommand per
// tool of [editorapp.Tools]. The window itself is [native.Run].
package cmd

import (
	"io"

	"github.com/spf13/cobra"

	"github.com/weakpixel/engine/tools/internal/cli"
	"github.com/weakpixel/engine/tools/wpix-editor/internal/native"
)

// keys is the help of the shell every tool shares.
const keys = `Keys: middle mouse or Alt/Space+left drag pans, wheel zooms, F frames, 1 sets zoom 1,
Ctrl/Cmd+Z, Ctrl/Cmd+Shift+Z, Ctrl/Cmd+Y undo and redo and Ctrl/Cmd+S saves (document tools).`

// runFunc opens a tool's window; the command tree takes it so tests open none.
type runFunc func(native.Options) error

// Execute runs wpix-editor with the arguments after the program name and
// returns the exit code: 0 ok, 1 failure, 2 usage error.
func Execute(args []string, stdout, stderr io.Writer) int {
	root := newRootCmd(native.Run)
	root.SetArgs(args)
	root.SetOut(stdout)
	root.SetErr(stderr)
	return cli.Execute(root, stderr)
}

func newRootCmd(run runFunc) *cobra.Command {
	root := cli.Group(&cobra.Command{
		Use:   "wpix-editor",
		Short: "Visual editors of the Weakpixel engine",
		Long: "wpix-editor opens the editor of a tool in a window: the tool's panel is drawn by debugui.\n" +
			"Run it from a game module with go tool wpix-editor <tool> <file>.\n\n" + keys,
	})
	cli.AddVersion(root)
	addToolCmds(root, run)
	return root
}
