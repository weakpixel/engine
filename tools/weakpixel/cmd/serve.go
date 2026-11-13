package cmd

import (
	"github.com/spf13/cobra"

	"github.com/weakpixel/engine/tools/internal/cli"
	"github.com/weakpixel/engine/tools/internal/devserver"
)

func newServeCmd() *cobra.Command {
	var opts devserver.Options

	cmd := &cobra.Command{
		Use:   "serve [flags] <dir>",
		Short: "Serve a browser build with no-cache and push asset changes to the open page",
		Long: "serve serves <dir> (the page and game.wasm) at /, and the --assets directory under /assets/,\n" +
			"both with Cache-Control: no-cache. GET /_weakpixel/changes is an event stream that names the\n" +
			"asset files that changed, which a game turns into hot reload with httpfs.WatchChanges.",
		Args: cli.Args(cobra.ExactArgs(1)),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Dir = args[0]
			return devserver.Run(cmd.Context(), opts, cmd.OutOrStdout())
		},
		ValidArgsFunction: func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
			return nil, cobra.ShellCompDirectiveFilterDirs
		},
	}

	cmd.Flags().StringVar(&opts.Addr, "addr", ":8080", "address to listen on")
	cmd.Flags().StringVar(&opts.Assets, "assets", "", "asset directory, served under /assets/ and watched")
	_ = cmd.MarkFlagDirname("assets")

	return cmd
}
