package cmd

import (
	"github.com/spf13/cobra"

	"github.com/weakpixel/engine/tools/internal/cli"
	"github.com/weakpixel/engine/tools/internal/tileset"
)

func newTilesetCmd() *cobra.Command {
	cmd := cli.Group(&cobra.Command{
		Use:   "tileset",
		Short: "Convert images into Tiled tilesets",
	})
	cmd.AddCommand(newTilesetFromAtlasCmd())
	return cmd
}

func newTilesetFromAtlasCmd() *cobra.Command {
	var opts tileset.FromAtlasOptions

	cmd := &cobra.Command{
		Use:   "from-atlas [flags] <atlas.xml>...",
		Short: "Make a uniform-tile sheet and a Tiled tileset from a texture atlas",
		Long: "from-atlas reads a packed Starling/ShoeBox TextureAtlas (<SubTexture> XML plus its PNG) and writes\n" +
			"a PNG in which every sub-texture sits in a cell of one uniform size, and a Tiled JSON tileset (.tsj)\n" +
			"next to it. Tile ids follow the order of the XML; each tile has a string property \"name\" with its\n" +
			"sub-texture name.\n\n" +
			"Several atlases (a wildcard such as *.xml) are converted one by one, each to its own output;\n" +
			"--out, --image and --name then are not allowed, and one failing atlas does not stop the rest.",
		Args: cli.Args(cobra.MinimumNArgs(1)),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Atlases = args
			return tileset.FromAtlas(opts, cmd.OutOrStdout())
		},
		ValidArgsFunction: func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
			return []string{"xml"}, cobra.ShellCompDirectiveFilterFileExt
		},
	}

	cmd.Flags().StringVarP(&opts.Out, "out", "o", "", "output PNG (default <atlas>.tiles.png next to the XML)")
	_ = cmd.MarkFlagFilename("out", "png")
	cmd.Flags().StringVar(&opts.Tile, "tile", "", "cell size WxH (default: the largest frame of the atlas)")
	cmd.Flags().IntVar(&opts.Columns, "columns", 16, "tiles per row, at most the tile count")
	cmd.Flags().IntVar(&opts.Margin, "margin", 0, "pixels around the grid")
	cmd.Flags().IntVar(&opts.Spacing, "spacing", 0, "pixels between cells")
	cmd.Flags().StringVar(&opts.Align, "align", "center", "where a frame smaller than the cell sits: center, topleft or bottomleft")
	_ = cmd.RegisterFlagCompletionFunc("align", func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
		return []string{"center", "topleft", "bottomleft"}, cobra.ShellCompDirectiveNoFileComp
	})
	cmd.Flags().StringVar(&opts.Image, "image", "", "atlas PNG (default: imagePath next to the XML, else <atlas>.png)")
	_ = cmd.MarkFlagFilename("image", "png")
	cmd.Flags().StringVar(&opts.Name, "name", "", "tileset name (default: output name without .png)")

	return cmd
}
