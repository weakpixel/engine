package cmd

import (
	"github.com/spf13/cobra"

	"github.com/weakpixel/engine/tools/internal/gen"
)

func newGenCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "gen [package patterns]",
		Short: "Generate Game Object code and the GOD file from +game:* markers",
		Long: "gen generates the Game Object code and the GOD file of the packages matching the patterns\n" +
			"(default: the current package). Only the target packages' generated files are written or removed.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return gen.Run(gen.Options{Patterns: args})
		},
	}

	return cmd
}
