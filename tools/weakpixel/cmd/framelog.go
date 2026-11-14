package cmd

import (
	"github.com/spf13/cobra"

	"github.com/weakpixel/engine/tools/internal/cli"
	"github.com/weakpixel/engine/tools/internal/framelog"
)

func newFramelogCmd() *cobra.Command {
	var opts framelog.Options

	cmd := &cobra.Command{
		Use:   "framelog [--skip n] <frames.csv>",
		Short: "Summarize a frame log and list the frames that stutter",
		Long: "framelog reads a frame log written by game.Engine (Config.FrameLog or WEAKPIXEL_FRAMELOG)\n" +
			"and prints the number of frames, histograms of the draw gap and the ticks, the median draw gap\n" +
			"and one line per hitch.",
		Args: cli.Args(cobra.ExactArgs(1)),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.File = args[0]
			return framelog.Run(opts, cmd.OutOrStdout())
		},
		ValidArgsFunction: func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
			return []string{"csv"}, cobra.ShellCompDirectiveFilterFileExt
		},
	}

	cmd.Flags().IntVar(&opts.Skip, "skip", 60, "rows to drop at the start (window creation and asset loading make those frames late)")

	return cmd
}
