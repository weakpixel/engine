package cmd

import (
	"github.com/spf13/cobra"

	"github.com/weakpixel/engine/tools/internal/anim"
	"github.com/weakpixel/engine/tools/internal/cli"
)

func newAnimCmd() *cobra.Command {
	cmd := cli.Group(&cobra.Command{
		Use:   "anim",
		Short: "Validate and create *.anim.json animation definitions",
	})
	cmd.AddCommand(newAnimValidateCmd(), newAnimCreateCmd())
	return cmd
}

func newAnimValidateCmd() *cobra.Command {
	var opts anim.ValidateOptions

	cmd := &cobra.Command{
		Use:   "validate <file.anim.json>...",
		Short: "Check definitions against their Aseprite sheets",
		Long: "validate checks each definition against its Aseprite sheet JSON with the same rules as the anim\n" +
			"loader. It prints one line per finding, <file>: error|warn|info: ..., then a summary line. Tags the\n" +
			"definition does not list are info, not errors.",
		Args: cli.Args(cobra.MinimumNArgs(1)),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Files = args
			return anim.Validate(opts, cmd.OutOrStdout())
		},
		ValidArgsFunction: func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
			return []string{"json"}, cobra.ShellCompDirectiveFilterFileExt
		},
	}

	return cmd
}

func newAnimCreateCmd() *cobra.Command {
	var opts anim.CreateOptions

	cmd := &cobra.Command{
		Use:   "create --file <sheet.json> [-o <out.anim.json>|-] [--force]",
		Short: "Write a starter definition for an Aseprite sheet",
		Long: "create writes a starter definition that lists every tag of the sheet as {\"loop\": true}, with\n" +
			"defaultClip set to the first tag and no pivot or boxes. The default output is the sheet path with\n" +
			".json replaced by .anim.json; -o - writes to stdout. An existing file is only replaced with --force.",
		Args: cli.Args(cobra.NoArgs),
		RunE: func(cmd *cobra.Command, args []string) error {
			return anim.Create(opts, cmd.OutOrStdout(), cmd.ErrOrStderr())
		},
	}

	cmd.Flags().StringVarP(&opts.File, "file", "f", "", "the Aseprite sheet JSON (required)")
	_ = cmd.MarkFlagRequired("file")
	_ = cmd.MarkFlagFilename("file", "json")
	cmd.Flags().StringVarP(&opts.Out, "out", "o", "", `output file, "-" for stdout (default: the sheet path with .json replaced by .anim.json)`)
	cmd.Flags().BoolVar(&opts.Force, "force", false, "overwrite an existing output file")

	return cmd
}
