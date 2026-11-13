// Package cmd is the command line of weakpixel: one subcommand per tool in
// tools/internal. The commands are:
//
//	weakpixel gen [package patterns]             generate Game Object code and the GOD file
//	weakpixel doc [flags] [inputs...]            merge GOD files; HTML, Markdown or merged GOD
//	weakpixel anim validate|create ...           check and create *.anim.json definitions
//	weakpixel assets check|ls|catalog|pack|keys  check, list, catalog and pack an asset library
//	weakpixel tileset from-atlas [flags] <xml>   pack a texture atlas into a uniform-tile Tiled tileset
//	weakpixel schema [flags] [god inputs]        write JSON schemas for the game's JSON files
//	weakpixel framelog [--skip n] <frames.csv>   summarize a frame log
//	weakpixel serve [flags] <dir>                serve a browser build and push asset changes
//	weakpixel new [flags] <dir>                  create a game project
//	weakpixel mcp [flags]                        serve the project to an AI agent (MCP over stdio)
//	weakpixel help [command]                     list the commands, or show one command's usage
//	weakpixel version                            print the module version
package cmd

import (
	"io"

	"github.com/spf13/cobra"

	"github.com/weakpixel/engine/tools/internal/cli"
)

// Execute runs weakpixel with the arguments after the program name and returns
// the exit code: 0 ok, 1 failure, 2 usage error.
func Execute(args []string, stdout, stderr io.Writer) int {
	root := newRootCmd()
	root.SetArgs(args)
	root.SetOut(stdout)
	root.SetErr(stderr)
	return cli.Execute(root, stderr)
}

func newRootCmd() *cobra.Command {
	root := cli.Group(&cobra.Command{
		Use:   "weakpixel",
		Short: "Command-line tools of the weakpixel engine",
		Long: "weakpixel runs the engine's tools. Run weakpixel help <command> for the usage of one.\n" +
			"Every command exits 0 on success, 1 on failure and 2 on a usage error.",
	})
	cli.AddVersion(root)
	root.AddCommand(
		newGenCmd(),
		newDocCmd(),
		newAnimCmd(),
		newAssetsCmd(),
		newTilesetCmd(),
		newSchemaCmd(),
		newFramelogCmd(),
		newServeCmd(),
		newNewCmd(),
		newMCPCmd(),
	)
	return root
}
