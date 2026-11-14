// Package gen implements `wpix gen`. It generates GameObject schemas from
// +game:* markers.
//
// Usage: go run github.com/weakpixel/engine/tools/wpix gen [package patterns]
//
// The default target is the current package. Only the target packages'
// generated files are written or removed: zz__gen.go next to the sources,
// and the package's GOD either as zz__gen.god.json next to them or, when the
// project's .weakpixel.json sets god.specs, as <specs>/<group>.god.json.
package gen

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/tools/imports"

	"github.com/weakpixel/engine/internal/project"
)

// output is the generated files of one target package. Content (zz__gen.go)
// and JSON (the GOD) are nil when the package has nothing to generate and
// stale files should be removed.
type output struct {
	Dir     string
	PkgPath string
	Group   string // package group: +game:group:name, else the package name
	Content []byte
	JSON    []byte
}

// Options are the arguments of `wpix gen`.
type Options struct {
	Patterns []string // package patterns; none means the current package
}

// Run generates the files of the packages matching opts.Patterns, relative to
// the current directory.
func Run(opts Options) error {
	patterns := opts.Patterns
	if len(patterns) == 0 {
		patterns = []string{"."}
	}
	return run(".", patterns)
}

func run(dir string, patterns []string) error {
	settings, _, err := project.Load(dir)
	if err != nil {
		return err
	}
	// The GOD is build output: without god.specs it goes to build/god.
	specs := project.DefaultSpecs(dir)
	if settings != nil && settings.GODSpecs != "" {
		specs = settings.GODSpecs
	}
	outputs, err := generate(dir, patterns)
	if err != nil {
		return err
	}
	for _, out := range outputs {
		if err := write(out, specs); err != nil {
			return fmt.Errorf("write %s: %w", out.PkgPath, err)
		}
	}
	return nil
}

// generate loads the target packages and renders their generated files
// without touching the file system. All validation errors are collected.
func generate(dir string, patterns []string) ([]output, error) {
	l, err := load(dir, patterns)
	if err != nil {
		return nil, err
	}

	var diags []diag
	var models []*Package
	groups := map[*Package]string{}
	for _, target := range l.targets {
		model, d := parsePackage(l, target)
		diags = append(diags, d...)
		models = append(models, model)
		p := &parser{l: l, target: target}
		if groups[model] = p.packageGroup(target); groups[model] == "" {
			groups[model] = target.Name
		}
	}
	if len(diags) > 0 {
		return nil, errors.New(strings.Join(l.formatDiags(diags), "\n"))
	}

	var outputs []output
	for _, model := range models {
		pkgDir := packageDir(model.Pkg)
		if pkgDir == "" {
			continue
		}
		src, err := render(model)
		if err != nil {
			return nil, fmt.Errorf("generate %s: %w", model.Pkg.PkgPath, err)
		}
		if src != nil {
			src, err = imports.Process(filepath.Join(pkgDir, outputName), src, nil)
			if err != nil {
				return nil, fmt.Errorf("format %s: %w", model.Pkg.PkgPath, err)
			}
		}
		data, err := renderJSON(model)
		if err != nil {
			return nil, fmt.Errorf("generate %s: %w", model.Pkg.PkgPath, err)
		}
		outputs = append(outputs, output{Dir: pkgDir, PkgPath: model.Pkg.PkgPath, Group: groups[model], Content: src, JSON: data})
	}
	return outputs, nil
}

// write writes (or removes) the files of out. specs is the GOD specs folder
// of the project, or "" to keep the GOD next to the sources (tests only: run
// always passes a folder).
func write(out output, specs string) error {
	if out.Content == nil {
		if err := removeStale(out.Dir, outputName, jsonOutputName); err != nil {
			return err
		}
		if specs == "" {
			return nil
		}
		return removeSpecs(specs, out.PkgPath, "")
	}
	if err := writeFile(filepath.Join(out.Dir, outputName), out.Content); err != nil {
		return err
	}
	if specs == "" {
		return writeFile(filepath.Join(out.Dir, jsonOutputName), out.JSON)
	}
	// The GOD lives in the specs folder; one next to the sources is stale.
	if err := removeStale(out.Dir, jsonOutputName); err != nil {
		return err
	}
	return writeSpecs(specs, out)
}

// writeSpecs writes the GOD of out to <specs>/<group>.god.json. The file
// must be free or already belong to out's package: two packages with the
// same group would silently overwrite each other's GOD. Files the package
// wrote under another name (its group changed) are removed.
func writeSpecs(specs string, out output) error {
	name := out.Group + specSuffix
	path := filepath.Join(specs, name)
	if pkg, ok := specPackage(path); ok && pkg != out.PkgPath {
		return fmt.Errorf("%s is generated from package %s; package %s has the same group %q; set +game:group:name",
			path, pkg, out.PkgPath, out.Group)
	}
	if err := removeSpecs(specs, out.PkgPath, name); err != nil {
		return err
	}
	if err := os.MkdirAll(specs, 0o755); err != nil {
		return err
	}
	return writeFile(path, out.JSON)
}

// writeFile writes content to path unless the file already has it.
func writeFile(path string, content []byte) error {
	if old, err := os.ReadFile(path); err == nil && string(old) == string(content) {
		return nil
	}
	return os.WriteFile(path, content, 0o644)
}
