// Package assets implements `wpix assets`. It checks, lists, catalogs and packs an asset library
// laid out in the standard convention (doc/guide/engine/asset-library.md)
// without starting Ebiten.
//
// Usage:
//
//	wpix assets check [--json] [<root>]
//	wpix assets ls [<root>]
//	wpix assets catalog [<root>] [-o <file>|-]
//	wpix assets pack [<root>] [-o <out>]
//	wpix assets keys [<root>] [--pkg name] [-o file|-]
//	wpix assets embed [<root>] [--pkg name] [--var name] [--tags constraint] [-o file|-]
//
// check prints every finding of library.Scan, one line each
// ("<file>: error|warn: ..."), then a summary line. With --json it prints one
// JSON object instead, for tools such as the editor:
//
//	{"findings": [{"severity": "error", "path": "forest/forest.level.json", "msg": "..."}],
//	 "errors": 1, "warnings": 0}
//
// A library that cannot be read at all (an invalid library.json) is reported
// as {"fatal": "..."} with exit code 1.
//
// ls prints each package with its file count, size, entries and the
// packages it uses. With --ignored it prints instead every file an
// .assetignore ignores, with the line that ignores it.
//
// catalog writes assets.catalog.json for a loose library, so it can be
// served from a file system that cannot list directories (HTTP). The
// default output is <root>/assets.catalog.json; "-o -" writes to stdout.
//
// pack writes a packed build to <out> (default build/assets), which must not
// exist or be empty: one zip per package under packs/, files outside the packages copied loose,
// and the catalog naming the packs.
//
// keys writes an optional Go file of constants for the packages, the files
// of a known kind and the clip tags (default package "assets", default
// output stdout), for go:generate:
//
//	//go:generate go run github.com/weakpixel/engine/tools/wpix assets keys assets --pkg main -o zz__gen.assets.go
//
// embed writes a Go file that embeds the library with go:embed patterns that
// leave out what .assetignore files ignore, for go:generate (default package
// "main", variable "assets"):
//
//	//go:generate go tool wpix assets embed assets --tags "!editor" -o zz__gen.assets_embed.go
//
// <root> may be left out when a .weakpixel.json above the working directory
// sets "assets.root" (doc/guide/project-file.md); an empty Root of the options
// means the same.
//
// catalog, pack, keys and embed refuse a library with errors (run check). Exit codes:
// 0 ok (warnings allowed), 1 errors, 2 usage error.
package assets

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/weakpixel/engine/asset/library"
	"github.com/weakpixel/engine/internal/project"
	"github.com/weakpixel/engine/tools/internal/cli"
)

// DefaultPackOut is the output folder of pack without an option. build/ is
// git-ignored, and pack refuses a folder that is not empty, so the default
// cannot clobber anything.
const DefaultPackOut = "build/assets"

// CheckOptions are the arguments of `wpix assets check`.
type CheckOptions struct {
	Root string // library root; empty means assets.root of .weakpixel.json
	JSON bool   // print one JSON object instead of lines
}

// ListOptions are the arguments of `wpix assets ls`.
type ListOptions struct {
	Root    string // library root; empty means assets.root of .weakpixel.json
	Ignored bool   // list the ignored files and the rule of each instead of the packages
}

// CatalogOptions are the arguments of `wpix assets catalog`.
type CatalogOptions struct {
	Root string // library root; empty means assets.root of .weakpixel.json
	Out  string // output file, "-" for stdout; empty means <Root>/assets.catalog.json
}

// PackOptions are the arguments of `wpix assets pack`.
type PackOptions struct {
	Root string // library root; empty means assets.root of .weakpixel.json
	Out  string // output folder, which must not exist or be empty; empty means DefaultPackOut
}

// KeysOptions are the arguments of `wpix assets keys`.
type KeysOptions struct {
	Root string // library root; empty means assets.root of .weakpixel.json
	Pkg  string // package name of the generated file; empty means "assets"
	Out  string // output file, "-" or empty for stdout
}

// libraryError is the error of a command that refuses a library with errors.
type libraryError struct{ findings []library.Finding }

func (e *libraryError) Error() string {
	var b strings.Builder
	for _, f := range e.findings {
		if f.Severity == library.Error {
			fmt.Fprintln(&b, f)
		}
	}
	b.WriteString("the library has errors; fix them first (wpix assets check)")
	return b.String()
}

// refuseErrors returns a *libraryError if findings has errors.
func refuseErrors(findings []library.Finding) error {
	for _, f := range findings {
		if f.Severity == library.Error {
			return &libraryError{findings}
		}
	}
	return nil
}

// resolveRoot returns root, or assets.root of the project when root is empty.
// A project without it is a usage error; a root that is not a folder is a
// plain error.
func resolveRoot(root string) (string, error) {
	if root == "" {
		var err error
		if root, err = projectRoot(); err != nil {
			return "", cli.Usagef("%v", err)
		}
	}
	if fi, err := os.Stat(root); err != nil || !fi.IsDir() {
		return "", fmt.Errorf("%s is not a directory", root)
	}
	return root, nil
}

// scan reads the library at root. With asJSON a library that cannot be read is
// also printed as {"fatal": "..."} to stdout.
func scan(root string, asJSON bool, stdout io.Writer) (*library.Catalog, []library.Finding, error) {
	cat, findings, err := library.Scan(os.DirFS(root))
	if err != nil && asJSON {
		writeJSON(stdout, map[string]string{"fatal": err.Error()})
	}
	return cat, findings, err
}

// Check prints the findings of the library. It returns an error when there
// are errors; the findings are already printed.
func Check(opts CheckOptions, stdout io.Writer) error {
	root, err := resolveRoot(opts.Root)
	if err != nil {
		return err
	}
	_, findings, err := scan(root, opts.JSON, stdout)
	if err != nil {
		return err
	}
	if opts.JSON {
		return reportJSON(stdout, findings)
	}
	if ignored, err := library.IgnoredFiles(os.DirFS(root)); err == nil && len(ignored) > 0 {
		fmt.Fprintf(stdout, "%d files ignored by %s (wpix assets ls --ignored)\n", len(ignored), library.IgnoreFile)
	}
	return report(stdout, findings)
}

// List prints the packages of the library.
func List(opts ListOptions, stdout io.Writer) error {
	root, err := resolveRoot(opts.Root)
	if err != nil {
		return err
	}
	if opts.Ignored {
		return listIgnored(stdout, root)
	}
	cat, _, err := scan(root, false, stdout)
	if err != nil {
		return err
	}
	list(stdout, root, cat)
	return nil
}

// listIgnored prints each ignored file with the rule that ignores it,
// aligned: "forest/raw/hero.psd  forest/.assetignore:3".
func listIgnored(w io.Writer, root string) error {
	files, err := library.IgnoredFiles(os.DirFS(root))
	if err != nil {
		return err
	}
	width := 0
	for _, f := range files {
		width = max(width, len(f.Path))
	}
	for _, f := range files {
		fmt.Fprintf(w, "%-*s  %s\n", width, f.Path, f.By)
	}
	return nil
}

// Catalog writes the catalog of a library without errors.
func Catalog(opts CatalogOptions, stdout io.Writer) error {
	root, err := resolveRoot(opts.Root)
	if err != nil {
		return err
	}
	cat, findings, err := scan(root, false, stdout)
	if err != nil {
		return err
	}
	if err := refuseErrors(findings); err != nil {
		return err
	}
	out := opts.Out
	if out == "" {
		out = filepath.Join(root, library.CatalogFile)
	}
	if out == "-" {
		_, err = stdout.Write(cat.Encode())
		return err
	}
	return os.WriteFile(out, cat.Encode(), 0o644)
}

// Pack writes the packed build of a library without errors.
func Pack(opts PackOptions, stdout io.Writer) error {
	root, err := resolveRoot(opts.Root)
	if err != nil {
		return err
	}
	_, findings, err := scan(root, false, stdout)
	if err != nil {
		return err
	}
	if err := refuseErrors(findings); err != nil {
		return err
	}
	out := opts.Out
	if out == "" {
		out = DefaultPackOut
	}
	return pack(stdout, root, out)
}

// projectRoot returns "assets.root" of the .weakpixel.json found from the
// working directory.
func projectRoot() (string, error) {
	settings, path, err := project.Load(".")
	switch {
	case err != nil:
		return "", err
	case settings == nil || settings.AssetsRoot == "":
		where := "no .weakpixel.json found"
		if path != "" {
			where = path + " has no assets.root"
		}
		return "", fmt.Errorf("no <root> given and %s", where)
	}
	return settings.AssetsRoot, nil
}

// report prints the findings and a summary line.
func report(w io.Writer, findings []library.Finding) error {
	errs, warns := 0, 0
	for _, f := range findings {
		fmt.Fprintln(w, f)
		if f.Severity == library.Error {
			errs++
		} else {
			warns++
		}
	}
	fmt.Fprintf(w, "%d errors, %d warnings\n", errs, warns)
	if errs > 0 {
		return fmt.Errorf("%d errors", errs)
	}
	return nil
}

// jsonFinding is one finding in the --json output.
type jsonFinding struct {
	Severity string `json:"severity"`
	Path     string `json:"path"`
	Msg      string `json:"msg"`
}

// reportJSON prints the findings as one JSON object and returns an error if
// there are errors.
func reportJSON(w io.Writer, findings []library.Finding) error {
	out := struct {
		Findings []jsonFinding `json:"findings"`
		Errors   int           `json:"errors"`
		Warnings int           `json:"warnings"`
	}{Findings: []jsonFinding{}}
	for _, f := range findings {
		out.Findings = append(out.Findings, jsonFinding{f.Severity.String(), f.Path, f.Msg})
		if f.Severity == library.Error {
			out.Errors++
		} else {
			out.Warnings++
		}
	}
	writeJSON(w, out)
	if out.Errors > 0 {
		return fmt.Errorf("%d errors", out.Errors)
	}
	return nil
}

// writeJSON prints v as indented JSON; plain strings and numbers cannot fail.
func writeJSON(w io.Writer, v any) {
	data, _ := json.MarshalIndent(v, "", "  ")
	fmt.Fprintf(w, "%s\n", data)
}

// list prints the packages of cat.
func list(w io.Writer, root string, cat *library.Catalog) {
	for _, g := range cat.Packages {
		var size int64
		for _, f := range g.Files {
			if fi, err := os.Stat(filepath.Join(root, filepath.FromSlash(f))); err == nil {
				size += fi.Size()
			}
		}
		fmt.Fprintf(w, "%s: %d files, %s\n", g.Name, len(g.Files), humanSize(size))
		for _, e := range g.Entries {
			fmt.Fprintf(w, "  entry %s\n", e)
		}
		if len(g.Uses) > 0 {
			fmt.Fprintf(w, "  uses %s\n", strings.Join(g.Uses, ", "))
		}
	}
}

// pack writes the packed build of root to out.
func pack(stdout io.Writer, root, out string) error {
	entries, err := os.ReadDir(out)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	if len(entries) > 0 {
		// An old build would leave stale packs next to the new ones.
		return fmt.Errorf("%s is not empty", out)
	}
	cat, _, err := library.WritePacks(os.DirFS(root), out)
	if err != nil {
		return err
	}
	for _, g := range cat.Packages {
		fmt.Fprintf(stdout, "%s -> %s (%s)\n", g.Name, g.Pack, humanSize(g.PackSize))
	}
	return nil
}

// humanSize formats n bytes as "123 B", "4.5 KiB" or "6.7 MiB".
func humanSize(n int64) string {
	switch {
	case n < 1024:
		return fmt.Sprintf("%d B", n)
	case n < 1024*1024:
		return fmt.Sprintf("%.1f KiB", float64(n)/1024)
	default:
		return fmt.Sprintf("%.1f MiB", float64(n)/(1024*1024))
	}
}
