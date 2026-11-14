package newgame

import (
	"bytes"
	"embed"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io/fs"
	"maps"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"text/template"
)

// templates is the project template. "all:" keeps the dot files
// (.weakpixel.json, .vscode/, .gitkeep).
//
//go:embed all:template
var templates embed.FS

// renames are template files whose name cannot be their output name: a
// .gitignore in template/ would apply to the engine repository itself.
var renames = map[string]string{"gitignore": ".gitignore"}

// templateData is what the .tmpl files see.
type templateData struct {
	Module       string // module path
	Name         string // Game Object group
	Title        string // window title and README heading
	Version      string // engine version in require
	ECSVersion   string // ECS version in require, or "" (go mod tidy adds it)
	AssetVersion string // asset module version in require, or "" (go mod tidy adds it)
	Replace      string // engine checkout for a replace directive, or ""
	GoVersion    string // go line
}

// render returns every file of the new project by its slash-separated path
// below the project directory.
func render(o Options) (map[string][]byte, error) {
	data := templateData{
		Module:       o.Module,
		Name:         o.Name,
		Title:        o.title,
		Version:      o.version,
		ECSVersion:   o.ecsVersion,
		AssetVersion: o.assetVersion,
		Replace:      o.Replace,
		GoVersion:    goVersion,
	}
	files := map[string][]byte{}
	err := fs.WalkDir(templates, "template", func(name string, de fs.DirEntry, err error) error {
		if err != nil || de.IsDir() {
			return err
		}
		src, err := templates.ReadFile(name)
		if err != nil {
			return err
		}
		out := strings.TrimPrefix(name, "template/")
		if base, ok := renames[path.Base(out)]; ok {
			out = path.Join(path.Dir(out), base)
		}
		if strings.HasSuffix(out, ".tmpl") {
			out = strings.TrimSuffix(out, ".tmpl")
			t, err := template.New(name).Option("missingkey=error").Parse(string(src))
			if err != nil {
				return err
			}
			var b bytes.Buffer
			if err := t.Execute(&b, data); err != nil {
				return err
			}
			src = b.Bytes()
		}
		files[out] = src
		return nil
	})
	if err != nil {
		return nil, err
	}
	files["assets/global/sprites/player.png"], err = placeholderPNG()
	return files, err
}

// write writes files into o.Dir. It writes nothing when one of them exists
// (naming every one), or when the directory is not empty and o.Force is not
// set.
func write(o Options, files map[string][]byte) error {
	names := slices.Sorted(maps.Keys(files))
	var exist []string
	for _, name := range names {
		if _, err := os.Lstat(filepath.Join(o.Dir, filepath.FromSlash(name))); err == nil {
			exist = append(exist, name)
		}
	}
	if len(exist) > 0 {
		return fmt.Errorf("%s: these files exist, nothing was written:\n  %s", o.Dir, strings.Join(exist, "\n  "))
	}
	if entries, err := os.ReadDir(o.Dir); err == nil && len(entries) > 0 && !o.Force {
		return fmt.Errorf("%s is not empty; --force writes into it (it never overwrites a file)", o.Dir)
	} else if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	for _, name := range names {
		dst := filepath.Join(o.Dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(dst, files[name], 0o644); err != nil {
			return err
		}
	}
	return nil
}

// placeholderPNG draws the player's 16x16 placeholder sprite: a round
// teal body with an outline and two eyes. Replace the file with your art;
// the key stays the same.
func placeholderPNG() ([]byte, error) {
	const size = 16
	var (
		none    = color.NRGBA{}
		outline = color.NRGBA{R: 0x1a, G: 0x1c, B: 0x2c, A: 0xff}
		body    = color.NRGBA{R: 0x38, G: 0xb7, B: 0x64, A: 0xff}
		shine   = color.NRGBA{R: 0xa7, G: 0xf0, B: 0x70, A: 0xff}
		eye     = color.NRGBA{R: 0xf4, G: 0xf4, B: 0xf4, A: 0xff}
	)
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	// Distances are squared, from the center of the 16x16 grid (7.5, 7.5).
	for y := range size {
		for x := range size {
			dx, dy := float64(x)-7.5, float64(y)-7.5
			d := dx*dx + dy*dy
			c := none
			switch {
			case d <= 36: // radius 6
				c = body
			case d <= 56: // the outline ring, up to radius 7.5
				c = outline
			}
			img.SetNRGBA(x, y, c)
		}
	}
	for _, p := range []image.Point{{4, 4}, {5, 4}, {4, 5}} {
		img.SetNRGBA(p.X, p.Y, shine)
	}
	for _, p := range []image.Point{{6, 6}, {6, 7}, {9, 6}, {9, 7}} {
		img.SetNRGBA(p.X, p.Y, eye)
	}
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}
