//go:build !(js && wasm)

package cmd

import (
	"maps"
	"slices"

	"github.com/spf13/cobra"

	"github.com/weakpixel/engine/tools/internal/cli"
	"github.com/weakpixel/engine/tools/wpix-editor/internal/editorapp"
	"github.com/weakpixel/engine/tools/wpix-editor/internal/native"
)

// addToolCmds adds one subcommand per tool, in name order.
func addToolCmds(root *cobra.Command, run runFunc) {
	for _, name := range slices.Sorted(maps.Keys(editorapp.Tools)) {
		root.AddCommand(newToolCmd(name, editorapp.Tools[name], run))
	}
}

// newToolCmd is the subcommand that opens one tool on a file.
func newToolCmd(name string, tool editorapp.Tool, run runFunc) *cobra.Command {
	var sheet, image string

	use, args := name+" <file>", cobra.ExactArgs(1)
	if tool.FileOptional {
		use, args = name+" [file]", cobra.RangeArgs(0, 1)
	}
	cmd := &cobra.Command{
		Use:   use,
		Short: tool.Summary,
		Args:  cli.Args(args),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts := native.Options{Tool: name, Sheet: sheet, Image: image}
			if len(args) > 0 {
				opts.File = args[0]
			}
			return run(opts)
		},
	}
	if tool.Sheet {
		cmd.Flags().StringVar(&sheet, "sheet", "", "Aseprite export (.aseprite.json) a definition that does not exist yet starts from")
		_ = cmd.MarkFlagFilename("sheet", "json")
		cmd.Long = tool.Summary + "\n\nThe file may be one that does not exist yet when --sheet is given.\n\n" + keys
	}
	if tool.Image {
		cmd.Flags().StringVar(&image, "image", "", "texture a sheet that does not exist yet starts from")
		_ = cmd.MarkFlagFilename("image", "png", "jpg", "jpeg", "gif")
		cmd.Long = tool.Summary + "\n\nThe file may be one that does not exist yet when --image is given.\n\n" + keys
	}
	if name == "fx" {
		cmd.Long = tool.Summary + "\n\nfx keys: Space plays or pauses, . steps, R restarts, a click bursts, drag the cross to move the emitter.\n\n" + keys
	}
	return cmd
}
