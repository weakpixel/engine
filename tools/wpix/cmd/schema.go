package cmd

import (
	"github.com/spf13/cobra"

	"github.com/weakpixel/engine/tools/internal/schema"
)

func newSchemaCmd() *cobra.Command {
	var opts schema.Options

	cmd := &cobra.Command{
		Use:   "schema [flags] [god inputs...]",
		Short: "Write JSON schemas of the game's JSON files, for editor completion and checks",
		Long: "schema writes a JSON Schema (draft-07) for every JSON file kind of a game project. The level and\n" +
			"prefab schemas come from the project's GOD files: the inputs are files, directories or ... patterns\n" +
			"as for wpix doc, by default god.specs of .weakpixel.json, else ./... Without GOD files, level\n" +
			"and prefab are skipped.",
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Inputs = args
			return schema.Run(opts, cmd.OutOrStdout())
		},
	}

	cmd.Flags().StringVarP(&opts.Out, "out", "o", "", "output folder (default schema.out of .weakpixel.json, else "+schema.DefaultOut+")")
	_ = cmd.MarkFlagDirname("out")

	return cmd
}
