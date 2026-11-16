package assets

import (
	"bytes"
	"errors"
	"fmt"
	"go/format"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/weakpixel/engine/asset/library"
)

// EmbedOptions are the arguments of `wpix assets embed`.
type EmbedOptions struct {
	Root string // library root, relative to the package folder; empty means assets.root of .weakpixel.json
	Pkg  string // package name of the generated file; empty means "main"
	Var  string // name of the embed.FS variable; empty means "assets"
	Tags string // build constraint of the generated file, without "//go:build"; empty for none
	Out  string // output file, "-" or empty for stdout
}

// Embed writes a Go file that embeds the library at Root with go:embed,
// leaving out what the library leaves out: the files its .assetignore files
// ignore, the ignore files themselves and the reserved top folders. go:embed
// patterns are fixed at compile time and a plain "//go:embed assets" cannot
// skip files, so the patterns are generated, for go:generate. A library with
// errors is refused.
func Embed(opts EmbedOptions, stdout io.Writer) error {
	root, err := resolveRoot(opts.Root)
	if err != nil {
		return err
	}
	fsys := os.DirFS(root)
	_, findings, err := library.Scan(fsys)
	if err != nil {
		return err
	}
	if err := refuseErrors(findings); err != nil {
		return err
	}
	patterns, err := embedPatterns(fsys, root)
	if err != nil {
		return err
	}
	src, err := generateEmbed(opts, root, patterns)
	if err != nil {
		return err
	}
	if opts.Out == "" || opts.Out == "-" {
		_, err = stdout.Write(src)
		return err
	}
	return os.WriteFile(opts.Out, src, 0o644)
}

// embedPatterns returns the fewest go:embed patterns, below the pattern
// prefix root, that embed exactly the library's files: a folder where
// go:embed's own walk of it gives exactly the library's files below it (so
// a file added there later embeds without regenerating), single files
// elsewhere. go:embed skips names starting with "." or "_" when it walks a
// folder, while the library lists them below the top folder, so a folder
// with such a file is listed file by file too, which embeds it.
func embedPatterns(fsys fs.FS, root string) ([]string, error) {
	prefix, err := embedPrefix(root)
	if err != nil {
		return nil, err
	}
	lib, err := library.Files(fsys)
	if err != nil {
		return nil, err
	}
	if len(lib) == 0 {
		return nil, fmt.Errorf("%s has no files to embed", root)
	}
	var bad []error
	for _, f := range lib {
		if strings.ContainsAny(f, "*?[\\\"`") {
			bad = append(bad, fmt.Errorf("%s: the name has a character go:embed cannot match (*?[\\\"`); rename it or ignore it", f))
		}
	}
	if len(bad) > 0 {
		return nil, errors.Join(bad...)
	}
	// walked holds, per folder, what go:embed's walk of that folder gives.
	var all []string
	err = fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.Type().IsRegular() {
			all = append(all, p)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	var out []string
	var visit func(dir string)
	visit = func(dir string) {
		if embedWalkMatches(dir, all, lib) {
			out = append(out, path.Join(prefix, dir))
			return
		}
		var subdirs []string
		for _, f := range lib {
			rel, ok := below(f, dir)
			if !ok {
				continue
			}
			if sub, _, nested := strings.Cut(rel, "/"); nested {
				if d := path.Join(dir, sub); !slices.Contains(subdirs, d) {
					subdirs = append(subdirs, d)
				}
			} else {
				out = append(out, path.Join(prefix, f))
			}
		}
		for _, d := range subdirs {
			visit(d)
		}
	}
	visit(".")
	slices.Sort(out)
	return out, nil
}

// embedPrefix checks that root can be named in a go:embed pattern: a
// relative path inside the package folder.
func embedPrefix(root string) (string, error) {
	p := filepath.ToSlash(filepath.Clean(root))
	if filepath.IsAbs(root) || p == "." || p == ".." || strings.HasPrefix(p, "../") {
		return "", fmt.Errorf("embed: the root %s must be a folder below the package folder (go:embed names it relative to the Go file)", root)
	}
	return p, nil
}

// below returns f relative to dir ("." is the root), if f is inside it.
func below(f, dir string) (string, bool) {
	if dir == "." {
		return f, true
	}
	return strings.CutPrefix(f, dir+"/")
}

// embedWalkMatches reports whether a go:embed of folder dir gives exactly
// the library files below it: every file below dir without a "." or "_"
// name (relative to dir) is a library file, and every library file below
// dir is such a file.
func embedWalkMatches(dir string, all, lib []string) bool {
	n := 0
	for _, f := range all {
		rel, ok := below(f, dir)
		if !ok || embedSkips(rel) {
			continue
		}
		if _, ok := slices.BinarySearch(lib, f); !ok {
			return false
		}
		n++
	}
	count := 0
	for _, f := range lib {
		if _, ok := below(f, dir); ok {
			count++
		}
	}
	return n == count && n > 0
}

// embedSkips reports whether go:embed's walk skips rel: a name on the way
// starts with "." or "_".
func embedSkips(rel string) bool {
	for seg := range strings.SplitSeq(rel, "/") {
		if strings.HasPrefix(seg, ".") || strings.HasPrefix(seg, "_") {
			return true
		}
	}
	return false
}

// generateEmbed returns the gofmt'ed source of the embed file.
func generateEmbed(opts EmbedOptions, root string, patterns []string) ([]byte, error) {
	pkg, name := opts.Pkg, opts.Var
	if pkg == "" {
		pkg = "main"
	}
	if name == "" {
		name = "assets"
	}
	var b bytes.Buffer
	b.WriteString("// Code generated by wpix assets embed; DO NOT EDIT.\n\n")
	if opts.Tags != "" {
		fmt.Fprintf(&b, "//go:build %s\n\n", opts.Tags)
	}
	fmt.Fprintf(&b, "package %s\n\nimport \"embed\"\n\n", pkg)
	fmt.Fprintf(&b, "// %s is the asset library at %s, without what its %s files ignore.\n//\n", name, filepath.ToSlash(root), library.IgnoreFile)
	for _, p := range patterns {
		if strings.ContainsAny(p, " \t") {
			p = strconv.Quote(p)
		}
		fmt.Fprintf(&b, "//go:embed %s\n", p)
	}
	fmt.Fprintf(&b, "var %s embed.FS\n", name)
	src, err := format.Source(b.Bytes())
	if err != nil {
		return nil, fmt.Errorf("embed: %w", err)
	}
	return src, nil
}
