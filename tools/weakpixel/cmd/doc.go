package cmd

import (
	"strings"

	"github.com/spf13/cobra"

	"github.com/weakpixel/engine/tools/internal/goddoc"
)

func newDocCmd() *cobra.Command {
	var opts goddoc.Options

	cmd := &cobra.Command{
		Use:   "doc [flags] [inputs...]",
		Short: "Merge GOD files into one GOD, or document them as HTML or Markdown",
		Long: "doc merges the GOD files of a project into one document with an index of all Game Objects and\n" +
			"an index of all Components, or into one merged GOD. Inputs are GOD files (*.god.json), directories\n" +
			"(searched recursively) or patterns such as ./... and dir/...; the default is god.specs of the\n" +
			"project's .weakpixel.json, else ./... Flags override .weakpixel.json.",
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Inputs = args
			return goddoc.Run(opts, cmd.OutOrStdout())
		},
	}

	cmd.Flags().StringVar(&opts.Format, "format", "", "output format: "+strings.Join(goddoc.Formats, ", ")+" (default: from the --output extension, else html)")
	_ = cmd.RegisterFlagCompletionFunc("format", func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
		return goddoc.Formats, cobra.ShellCompDirectiveNoFileComp
	})
	cmd.Flags().StringVarP(&opts.Output, "output", "o", "", `output file, "-" for stdout (default from .weakpixel.json, else god-doc.html or god-doc.md; god-doc/ with --split)`)
	cmd.Flags().BoolVar(&opts.Split, "split", false, "write Markdown as several files into the directory --output")
	cmd.Flags().StringVar(&opts.Title, "title", "Game Objects", "document title")

	return cmd
}
