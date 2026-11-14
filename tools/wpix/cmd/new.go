package cmd

import (
	"github.com/spf13/cobra"

	"github.com/weakpixel/engine/tools/internal/cli"
	"github.com/weakpixel/engine/tools/internal/newgame"
)

func newNewCmd() *cobra.Command {
	var opts newgame.Options

	cmd := &cobra.Command{
		Use:   "new [flags] <dir>",
		Short: "Create a game project that runs, tests and opens in the editor",
		Long: "new creates a game module in <dir>: a window, one level with a player that walks, a test, the\n" +
			"editor and browser builds, a Makefile and a README. Then it runs git init, go mod tidy,\n" +
			"go generate ./... and go build ./... in it.",
		Args: cli.Args(cobra.ExactArgs(1)),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Dir = args[0]
			return newgame.Run(opts, cmd.OutOrStdout(), cmd.ErrOrStderr())
		},
		ValidArgsFunction: func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
			return nil, cobra.ShellCompDirectiveFilterDirs
		},
	}

	cmd.Flags().StringVar(&opts.Module, "module", "", "module path (default: the base name of <dir>)")
	cmd.Flags().StringVar(&opts.Name, "name", "", "Game Object group of the game, [a-z][a-z0-9_]* (default: from the base name of <dir>)")
	cmd.Flags().StringVar(&opts.Replace, "replace", "", `add "replace github.com/weakpixel/engine => path" (an engine checkout)`)
	_ = cmd.MarkFlagDirname("replace")
	cmd.Flags().BoolVar(&opts.Force, "force", false, "write into a directory that is not empty (existing files are never overwritten)")
	cmd.Flags().BoolVar(&opts.NoGit, "no-git", false, "do not run git init")
	cmd.Flags().BoolVar(&opts.NoRun, "no-run", false, "only write the files: no git init, go mod tidy, go generate or go build")

	return cmd
}
