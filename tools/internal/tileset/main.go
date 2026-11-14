// Package tileset implements `wpix tileset`: tools that turn images into
// Tiled tilesets the engine loads (doc/guide/engine/tilemaps.md).
//
// Usage:
//
//	wpix tileset from-atlas [flags] <atlas.xml>...
//
// from-atlas reads a packed Starling/ShoeBox TextureAtlas (<SubTexture>
// XML plus its PNG) and writes a PNG in which every sub-texture sits in a
// cell of one uniform size, and a Tiled JSON tileset (.tsj) next to it. Tile
// ids follow the order of the XML; each tile has a string property "name"
// with its sub-texture name. Flags:
//
//	-o, --out file  output PNG (default <atlas>.tiles.png next to the XML)
//	--tile WxH      cell size (default: the largest frame of the atlas)
//	--columns n     tiles per row (default 16, at most the tile count)
//	--margin n      pixels around the grid (default 0)
//	--spacing n     pixels between cells (default 0)
//	--align a       center (default), topleft or bottomleft, for frames smaller than the cell
//	--image file    atlas PNG (default: imagePath next to the XML, else <atlas>.png)
//	--name s        tileset name (default: output name without .png)
//
// A sub-texture larger than the cell, or a missing image is an
// error; all problems are reported together. Several atlases (a wildcard such
// as *.xml) are converted one by one, each to its own output; --out, --image and
// --name then are not allowed, and one failing atlas does not stop the rest.
// Exit codes: 0 ok, 1 errors, 2 usage error.
package tileset

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/weakpixel/engine/tools/internal/cli"
)

// FromAtlasOptions are the arguments of `wpix tileset from-atlas`.
type FromAtlasOptions struct {
	Atlases []string // atlas XML files, or wildcard patterns the tool expands
	Out     string   // output PNG; empty means <atlas>.tiles.png next to the XML
	Tile    string   // cell size "WxH"; empty means the largest frame of the atlas
	Columns int      // tiles per row, at most the tile count; 0 means 16
	Margin  int      // pixels around the grid
	Spacing int      // pixels between cells
	Align   string   // "center", "topleft" or "bottomleft"; empty means center
	Image   string   // atlas PNG; empty means imagePath next to the XML, else <atlas>.png
	Name    string   // tileset name; empty means the output name without .png
}

// options are the settings of one atlas conversion.
type options struct {
	atlas, out, image, name  string
	columns, margin, spacing int
}

// FromAtlas converts every atlas of opts. Wrong options are a usage error and
// nothing is converted. Every atlas is converted on its own: failures are
// collected into the returned error and do not stop the rest.
func FromAtlas(opts FromAtlasOptions, stdout io.Writer) error {
	atlases, err := expandGlobs(opts.Atlases)
	if err != nil {
		return cli.Usagef("%v", err)
	}
	if len(atlases) == 0 {
		return cli.Usagef("from-atlas takes one or more <atlas.xml>")
	}
	if len(atlases) > 1 && (opts.Out != "" || opts.Image != "" || opts.Name != "") {
		return cli.Usagef("--out, --image and --name need a single <atlas.xml>")
	}
	if opts.Columns == 0 {
		opts.Columns = 16
	}
	if opts.Align == "" {
		opts.Align = "center"
	}
	align, err := ParseAlign(opts.Align)
	if err != nil {
		return cli.Usagef("%v", err)
	}
	var tile image.Point
	if opts.Tile != "" {
		if _, err := fmt.Sscanf(opts.Tile, "%dx%d", &tile.X, &tile.Y); err != nil || tile.X < 1 || tile.Y < 1 {
			return cli.Usagef("--tile %q is not WxH", opts.Tile)
		}
	}
	if opts.Out != "" && !strings.HasSuffix(opts.Out, ".png") {
		return cli.Usagef("--out must end in .png")
	}

	var errs []error
	for _, atlas := range atlases {
		one := options{atlas: atlas, out: opts.Out, image: opts.Image, name: opts.Name,
			columns: opts.Columns, margin: opts.Margin, spacing: opts.Spacing}
		if one.out == "" {
			one.out = strings.TrimSuffix(atlas, filepath.Ext(atlas)) + ".tiles.png"
		}
		if one.name == "" {
			one.name = strings.TrimSuffix(filepath.Base(one.out), ".png")
		}
		if err := convert(one, tile, align, stdout); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// expandGlobs returns the arguments with wildcard patterns expanded. The
// shell normally does that; this covers quoted patterns and shells that do
// not (Windows). An argument naming an existing file is kept as is, and a
// pattern that matches nothing is an error.
func expandGlobs(args []string) ([]string, error) {
	var out []string
	for _, a := range args {
		if _, err := os.Stat(a); err == nil || !strings.ContainsAny(a, "*?[") {
			out = append(out, a)
			continue
		}
		matches, err := filepath.Glob(a)
		if err != nil {
			return nil, fmt.Errorf("pattern %q: %v", a, err)
		}
		if len(matches) == 0 {
			return nil, fmt.Errorf("pattern %q matches no file", a)
		}
		out = append(out, matches...)
	}
	return out, nil
}

// convert does the work of from-atlas and returns all problems it found.
func convert(o options, tile image.Point, align Align, stdout io.Writer) error {
	xmlData, err := os.ReadFile(o.atlas)
	if err != nil {
		return err
	}
	a, perr := ParseAtlas(bytes.NewReader(xmlData))
	if perr != nil {
		return prefixed(o.atlas, perr)
	}
	imgPath, err := findImage(o, a)
	if err != nil {
		return err
	}
	f, err := os.Open(imgPath)
	if err != nil {
		return err
	}
	defer f.Close()
	src, err := png.Decode(f)
	if err != nil {
		return fmt.Errorf("%s: %w", imgPath, err)
	}
	if err := a.CheckBounds(src.Bounds()); err != nil {
		return prefixed(o.atlas, err)
	}
	l, err := NewLayout(a, tile, o.columns, o.margin, o.spacing)
	if err != nil {
		return prefixed(o.atlas, err)
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, Render(src, a, l, align)); err != nil {
		return err
	}
	tsjData, err := EncodeTSJ(a, l, o.name, filepath.Base(o.out))
	if err != nil {
		return err
	}
	tsjPath := strings.TrimSuffix(o.out, ".png") + ".tsj"
	if err := os.WriteFile(o.out, buf.Bytes(), 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(tsjPath, tsjData, 0o644); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "%s, %s: %d tiles of %dx%d, %d columns\n", o.out, tsjPath, l.Count, l.Tile.X, l.Tile.Y, l.Columns)
	return nil
}

// findImage returns the atlas PNG: -image, else the XML's imagePath next to
// it, else the XML's name with .png (the imagePath of an export is often
// stale).
func findImage(o options, a *Atlas) (string, error) {
	if o.image != "" {
		return o.image, nil
	}
	dir := filepath.Dir(o.atlas)
	candidates := []string{strings.TrimSuffix(o.atlas, filepath.Ext(o.atlas)) + ".png"}
	if a.ImagePath != "" {
		candidates = append([]string{filepath.Join(dir, a.ImagePath)}, candidates...)
	}
	for _, c := range candidates {
		if fi, err := os.Stat(c); err == nil && !fi.IsDir() {
			return c, nil
		}
	}
	return "", fmt.Errorf("%s: atlas image not found (tried %s); pass --image", o.atlas, strings.Join(candidates, ", "))
}

// prefixed puts the atlas file name in front of each line of err.
func prefixed(file string, err error) error {
	var out []error
	for line := range strings.SplitSeq(err.Error(), "\n") {
		out = append(out, fmt.Errorf("%s: %s", file, line))
	}
	return errors.Join(out...)
}
