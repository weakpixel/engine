//go:build !(js && wasm)

// Package cmd is the command line of wpix-editor: one command that opens a
// file in the tool of its kind ([editorapp.Kinds]). The window itself is
// [native.Run].
package cmd

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/weakpixel/engine/tools/internal/cli"
	"github.com/weakpixel/engine/tools/wpix-editor/internal/editorapp"
	"github.com/weakpixel/engine/tools/wpix-editor/internal/native"
)

// keys is the help of the shell every tool shares.
const keys = `Keys: middle mouse or Alt/Space+left drag pans, wheel zooms, F frames, 1 sets zoom 1,
Ctrl/Cmd+Z, Ctrl/Cmd+Shift+Z, Ctrl/Cmd+Y undo and redo and Ctrl/Cmd+S saves (editors).
fx keys: Space plays or pauses, . steps, R restarts, a click bursts, drag the cross to move the emitter.`

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
	var opts native.Options
	root := &cobra.Command{
		Use:   "wpix-editor <file>",
		Short: "Visual editors of the Weakpixel engine",
		Long: "wpix-editor opens a file in a window, in the tool of its kind: a kind that can be edited\n" +
			"opens its editor, any other kind its preview. The file name picks the kind.\n" +
			"Run it from a game module with go tool wpix-editor <file>.\n\n" + kindsHelp() + "\n" + keys,
		Args: func(cmd *cobra.Command, args []string) error {
			return checkArgs(cmd, args, opts)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.File = args[0]
			return run(opts)
		},
	}
	root.Flags().StringVar(&opts.Sheet, "sheet", "", "Aseprite export (.aseprite.json or .ase.json) a new .anim.json starts from")
	_ = root.MarkFlagFilename("sheet", "json")
	root.Flags().StringVar(&opts.Image, "image", "", "texture a new .sheet.json starts from")
	_ = root.MarkFlagFilename("image", "png", "jpg", "jpeg", "gif")
	cli.AddVersion(root)
	return root
}

// checkArgs takes exactly one file of a known kind, and --sheet and --image
// only for the kind whose starter needs them. Every refusal is a usage error.
func checkArgs(cmd *cobra.Command, args []string, opts native.Options) error {
	if len(args) == 0 {
		fmt.Fprint(cmd.ErrOrStderr(), cmd.UsageString())
		return cli.Usagef("wpix-editor needs a file")
	}
	kind, err := editorapp.KindOf(args[0])
	if err != nil {
		return cli.Usagef("%v", err)
	}
	if len(args) > 1 {
		return cli.Usagef("accepts 1 file, received %d", len(args))
	}
	for flag, val := range map[string]string{"sheet": opts.Sheet, "image": opts.Image} {
		if val != "" && kind.Flag != flag {
			return cli.Usagef("--%s applies to %s files only, not to %s", flag, suffixOfFlag(flag), filepath.Base(args[0]))
		}
	}
	return nil
}

// suffixOfFlag names the suffixes of the kind that takes flag.
func suffixOfFlag(flag string) string {
	for _, k := range editorapp.Kinds {
		if k.Flag == flag {
			return strings.Join(k.Suffixes, ", ")
		}
	}
	return "no"
}

// kindsHelp lists the kinds in help: suffixes, then what the tool does.
func kindsHelp() string {
	var b strings.Builder
	b.WriteString("Files:\n")
	for _, k := range editorapp.Kinds {
		fmt.Fprintf(&b, "  %-26s %s\n", strings.Join(k.Suffixes, " "), k.Summary)
		if k.Flag != "" {
			fmt.Fprintf(&b, "  %-26s a new file starts from --%s <%s>\n", "", k.Flag, k.FlagValue)
		}
	}
	return b.String()
}
