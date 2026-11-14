package cmd

import (
	"github.com/spf13/cobra"

	"github.com/weakpixel/engine/tools/internal/cli"
	"github.com/weakpixel/engine/tools/internal/mcpserver"
)

func newMCPCmd() *cobra.Command {
	var opts mcpserver.Options

	cmd := &cobra.Command{
		Use:   "mcp [flags]",
		Short: "Serve the project to an AI agent over the Model Context Protocol (stdio)",
		Long: "mcp is an MCP server on stdin and stdout for an AI agent such as Claude Code. Its tools read the\n" +
			"project's components, object types, assets, levels and prefabs. It serves until stdin closes;\n" +
			"diagnostics go to stderr. Register it with: claude mcp add weakpixel -- go tool wpix mcp",
		Args: cli.Args(cobra.NoArgs),
		RunE: func(cmd *cobra.Command, _ []string) error {
			opts.Version = cli.Version()
			return mcpserver.Run(cmd.Context(), opts, cmd.InOrStdin(), cmd.OutOrStdout(), cmd.ErrOrStderr())
		},
	}

	cmd.Flags().StringVar(&opts.Dir, "project", "", "project directory (default: found from the working directory)")
	cmd.Flags().StringVar(&opts.Game, "game", "", "package directory of the editor build (default: editor.game of .weakpixel.json)")
	cmd.Flags().StringSliceVar(&opts.Tags, "tags", nil, `build tags of the editor build (default: editor.tags, else "editor")`)
	_ = cmd.MarkFlagDirname("project")
	_ = cmd.MarkFlagDirname("game")

	return cmd
}
