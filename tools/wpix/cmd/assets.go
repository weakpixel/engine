package cmd

import (
	"github.com/spf13/cobra"

	"github.com/weakpixel/engine/tools/internal/assets"
	"github.com/weakpixel/engine/tools/internal/cli"
)

// rootArg is the Args validator of the assets commands: at most one
// positional, the library root, which defaults to assets.root of
// .weakpixel.json.
var rootArg = cli.Args(cobra.MaximumNArgs(1))

// rootOf returns the library root of args, or "" for the project's.
func rootOf(args []string) string {
	if len(args) == 0 {
		return ""
	}
	return args[0]
}

// completeRoot completes the root argument with directories.
func completeRoot(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
	return nil, cobra.ShellCompDirectiveFilterDirs
}

func newAssetsCmd() *cobra.Command {
	cmd := cli.Group(&cobra.Command{
		Use:   "assets",
		Short: "Check, list, catalog and pack an asset library",
		Long: "assets works on an asset library laid out in the standard convention. Every command takes the\n" +
			"library root as its one argument; it may be left out when a .weakpixel.json above the working\n" +
			"directory sets assets.root.",
	})
	cmd.AddCommand(newAssetsCheckCmd(), newAssetsLsCmd(), newAssetsCatalogCmd(), newAssetsPackCmd(), newAssetsKeysCmd(), newAssetsEmbedCmd())
	return cmd
}

func newAssetsCheckCmd() *cobra.Command {
	var opts assets.CheckOptions

	cmd := &cobra.Command{
		Use:   "check [flags] [root]",
		Short: "Report missing references, case mismatches and other findings",
		Long: "check prints every finding of the library, one line each (<file>: error|warn: ...), then a summary\n" +
			"line. With --json it prints one JSON object instead, for editors. It exits 1 when there are errors.",
		Args:              rootArg,
		ValidArgsFunction: completeRoot,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Root = rootOf(args)
			return assets.Check(opts, cmd.OutOrStdout())
		},
	}

	cmd.Flags().BoolVar(&opts.JSON, "json", false, "print the findings as one JSON object")

	return cmd
}

func newAssetsLsCmd() *cobra.Command {
	var opts assets.ListOptions

	cmd := &cobra.Command{
		Use:   "ls [root]",
		Short: "List the groups with their scope, size, entries and uses",
		Args:  rootArg, ValidArgsFunction: completeRoot,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Root = rootOf(args)
			return assets.List(opts, cmd.OutOrStdout())
		},
	}

	cmd.Flags().BoolVar(&opts.Ignored, "ignored", false, "list the files .assetignore files ignore, with the line that ignores each")

	return cmd
}

func newAssetsCatalogCmd() *cobra.Command {
	var opts assets.CatalogOptions

	cmd := &cobra.Command{
		Use:   "catalog [flags] [root]",
		Short: "Write assets.catalog.json for a loose library served over HTTP",
		Long: "catalog writes assets.catalog.json, so a loose library can be served from a file system that cannot\n" +
			"list directories (HTTP). It refuses a library with errors (run check).",
		Args: rootArg, ValidArgsFunction: completeRoot,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Root = rootOf(args)
			return assets.Catalog(opts, cmd.OutOrStdout())
		},
	}

	cmd.Flags().StringVarP(&opts.Out, "out", "o", "", `output file, "-" for stdout (default <root>/assets.catalog.json)`)

	return cmd
}

func newAssetsPackCmd() *cobra.Command {
	var opts assets.PackOptions

	cmd := &cobra.Command{
		Use:   "pack [flags] [root]",
		Short: "Write a packed build: one zip per group and the catalog",
		Long: "pack writes a packed build: one zip per group under packs/, files outside the groups copied loose,\n" +
			"and the catalog naming the packs. The output folder must not exist or be empty. It refuses a\n" +
			"library with errors (run check).",
		Args: rootArg, ValidArgsFunction: completeRoot,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Root = rootOf(args)
			return assets.Pack(opts, cmd.OutOrStdout())
		},
	}

	cmd.Flags().StringVarP(&opts.Out, "out", "o", assets.DefaultPackOut, "output folder, which must not exist or be empty")
	_ = cmd.MarkFlagDirname("out")

	return cmd
}

func newAssetsKeysCmd() *cobra.Command {
	var opts assets.KeysOptions

	cmd := &cobra.Command{
		Use:   "keys [flags] [root]",
		Short: "Write optional Go constants for the groups, files and clip tags",
		Long: "keys writes a Go file of constants for the groups, the files of a known kind and the clip tags, so\n" +
			"game code names assets by identifier instead of by string. It is meant for go:generate. It refuses a\n" +
			"library with errors (run check).",
		Args: rootArg, ValidArgsFunction: completeRoot,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Root = rootOf(args)
			return assets.Keys(opts, cmd.OutOrStdout())
		},
	}

	cmd.Flags().StringVar(&opts.Pkg, "pkg", "assets", "package name of the generated file")
	cmd.Flags().StringVarP(&opts.Out, "out", "o", "-", `output file, "-" for stdout`)

	return cmd
}

func newAssetsEmbedCmd() *cobra.Command {
	var opts assets.EmbedOptions

	cmd := &cobra.Command{
		Use:   "embed [flags] [root]",
		Short: "Write the go:embed file of a library, without the files .assetignore ignores",
		Long: "embed writes a Go file that declares an embed.FS of the library with go:embed patterns that leave out\n" +
			"what the .assetignore files ignore: a folder where nothing below it is left out, single files elsewhere.\n" +
			"The root is named relative to the package folder. It is meant for go:generate. It refuses a library\n" +
			"with errors (run check).",
		Args: rootArg, ValidArgsFunction: completeRoot,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Root = rootOf(args)
			return assets.Embed(opts, cmd.OutOrStdout())
		},
	}

	cmd.Flags().StringVar(&opts.Pkg, "pkg", "main", "package name of the generated file")
	cmd.Flags().StringVar(&opts.Var, "var", "assets", "name of the embed.FS variable")
	cmd.Flags().StringVar(&opts.Tags, "tags", "", `build constraint of the generated file, such as "!editor"`)
	cmd.Flags().StringVarP(&opts.Out, "out", "o", "-", `output file, "-" for stdout`)

	return cmd
}
