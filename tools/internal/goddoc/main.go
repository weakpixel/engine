// Package goddoc implements `wpix doc`. It merges the GOD files of a project into one document with
// an index of all Game Objects and an index of all Components, or into one
// merged GOD.
//
// Usage:
//
//	go run github.com/weakpixel/engine/tools/wpix doc [flags] [inputs...]
//
// Inputs are GOD files (*.god.json), directories (searched recursively) or
// patterns such as ./... and dir/...; the default is god.specs of the
// project's .weakpixel.json, else ./... . Directories named vendor, testdata,
// node_modules and those starting with "." or "_" are skipped below the root
// of a search, and so are merged GOD files that wpix doc wrote. All inputs are
// merged into one document: identical definitions collapse, a different
// definition under the same key is an error.
//
// Flags:
//
//	--format html|md|god  output format (default: from the -o extension, else html);
//	                 god the merged GOD (default output: god.out of .weakpixel.json)
//	-o, --output path  output file; "-" is stdout (default: index.html or god.md in
//	                 godDoc.out of .weakpixel.json, else god-doc.html or god-doc.md)
//	--split          Markdown only: -o is a directory (default godDoc.out, else god-doc/) that
//	                 gets index.md, components.md and one group-<name>.md per group
//	--title text     document title (default "Game Objects")
//
// Flags override .weakpixel.json (doc/guide/project-file.md).
package goddoc

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/weakpixel/engine/internal/project"
	"github.com/weakpixel/engine/tools/internal/cli"
)

// Options are the arguments of `wpix doc`. Empty fields take their
// default from the project settings (.weakpixel.json) or from the other
// fields.
type Options struct {
	Format string   // "html", "md" or "god"; empty means from the extension of Output, else html
	Output string   // output file, "-" for stdout
	Split  bool     // Markdown only: Output is a directory
	Title  string   // document title; empty means "Game Objects"
	Inputs []string // GOD files, directories or patterns; empty means god.specs, else ./...
}

// Run merges the GOD files of opts.Inputs and writes the document. A wrong
// combination of options is a usage error; everything else, such as inputs
// that match no GOD file, is a plain error. Nothing is written on failure.
func Run(opts Options, stdout io.Writer) error {
	resolved, err := resolve(opts)
	if err != nil {
		return cli.Usagef("%v", err)
	}
	return generate(resolved, stdout)
}

// resolve fills in the defaults from the project settings and checks the
// combination of options.
func resolve(opts Options) (*Options, error) {
	settings, _, err := project.Load(".")
	if err != nil {
		return nil, err
	}
	if settings == nil {
		settings = &project.Settings{}
	}
	if opts.Title == "" {
		opts.Title = "Game Objects"
	}
	switch {
	case len(opts.Inputs) > 0:
	case settings.GODSpecs != "":
		opts.Inputs = []string{settings.GODSpecs}
	case isDir(project.DefaultSpecs(".")):
		opts.Inputs = []string{project.DefaultSpecs(".")}
	default:
		opts.Inputs = []string{"./..."}
	}

	if opts.Format == "" {
		ext := strings.ToLower(filepath.Ext(opts.Output))
		switch {
		case opts.Split || ext == ".md" || ext == ".markdown":
			opts.Format = "md"
		case strings.HasSuffix(opts.Output, ".god.json"):
			opts.Format = "god"
		default:
			opts.Format = "html"
		}
	}
	switch {
	case !slices.Contains(Formats, opts.Format):
		return nil, fmt.Errorf("unknown format %q (want html, md or god)", opts.Format)
	case opts.Split && opts.Format != "md":
		return nil, fmt.Errorf("--split needs --format md")
	case opts.Split && opts.Output == "-":
		return nil, fmt.Errorf("--split writes a directory, not stdout")
	}
	if opts.Output == "" {
		opts.Output, err = defaultOutput(&opts, settings)
		if err != nil {
			return nil, err
		}
	}
	return &opts, nil
}

// Formats are the values of Options.Format.
var Formats = []string{"html", "md", "god"}

// docFileNames are the names of the outputs in godDoc.out, by format.
var docFileNames = map[string]string{"html": "index.html", "md": "god.md"}

// defaultOutput is the output of opts without -o: from the settings, else
// god-doc.<format> (god-doc/ with --split).
func defaultOutput(opts *Options, settings *project.Settings) (string, error) {
	switch {
	case opts.Format == "god" && settings.GODOut == "":
		return "", errors.New("--format god needs -o or god.out in .weakpixel.json")
	case opts.Format == "god":
		return settings.GODOut, nil
	case opts.Split && settings.GODDocOut != "":
		return settings.GODDocOut, nil
	case opts.Split:
		return "god-doc", nil
	case settings.GODDocOut != "":
		return filepath.Join(settings.GODDocOut, docFileNames[opts.Format]), nil
	}
	return "god-doc." + opts.Format, nil
}

// generate collects the inputs, renders the document and writes it. Nothing is
// written when rendering fails.
func generate(opts *Options, stdout io.Writer) error {
	files, err := collect(opts.Inputs)
	if err != nil {
		return err
	}
	doc, err := loadDocument(opts.Title, files)
	if err != nil {
		return err
	}

	// out maps a file name inside the output directory (split mode) or "" to its content.
	out := map[string][]byte{}
	switch {
	case opts.Split:
		for name, content := range renderMarkdownSplit(doc) {
			out[name] = []byte(content)
		}
	case opts.Format == "md":
		out[""] = []byte(renderMarkdown(doc))
	case opts.Format == "god":
		data, err := json.MarshalIndent(doc.merged(), "", "  ")
		if err != nil {
			return err
		}
		out[""] = append(data, '\n')
	default:
		var b bytes.Buffer
		if err := renderHTML(&b, doc); err != nil {
			return err
		}
		out[""] = b.Bytes()
	}

	if opts.Split {
		if err := os.MkdirAll(opts.Output, 0o755); err != nil {
			return err
		}
		for name, content := range out {
			if err := os.WriteFile(filepath.Join(opts.Output, name), content, 0o644); err != nil {
				return err
			}
		}
		return nil
	}
	if opts.Output == "-" {
		_, err := stdout.Write(out[""])
		return err
	}
	if err := os.MkdirAll(filepath.Dir(opts.Output), 0o755); err != nil {
		return err
	}
	return os.WriteFile(opts.Output, out[""], 0o644)
}

// isDir reports whether path is an existing folder.
func isDir(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && fi.IsDir()
}
