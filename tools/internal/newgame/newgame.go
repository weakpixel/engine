// Package newgame implements "weakpixel new": it creates a game module that
// runs, tests, regenerates, builds for the browser and opens in the VSCode
// level editor (doc/guide/tools.md#new).
//
// The files come from template/, embedded in the tool: names ending in .tmpl
// are text/template files (the suffix is dropped), the rest are copied as
// they are. Every Go file and go.mod in template/ ends in .tmpl, so the
// engine's own builds never see them.
package newgame

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime/debug"
	"slices"
	"strings"

	"github.com/weakpixel/engine/tools/internal/cli"
)

const (
	// enginePath is the engine's module path, which the new module requires.
	enginePath = "github.com/weakpixel/engine"
	// ecsPath is the ECS module path: the templates import it directly, so
	// the new module requires it next to the engine.
	ecsPath = "github.com/weakpixel/ecs"
	// assetPath is the asset system's module path: the templates import its
	// httpfs package, so the new module requires it next to the engine.
	assetPath = "github.com/weakpixel/asset"
	// goVersion is the go line of the new module: the engine's own.
	goVersion = "1.27"
	// replaceVersion is the version required next to --replace when the
	// tool's own version is unknown (run from a checkout).
	replaceVersion = "v0.0.0-00010101000000-000000000000"
)

// reservedGroups are the engine's own Game Object groups; a game's group
// must differ, or its GOD file would clash with the engine's.
var reservedGroups = []string{"gfx"}

// Options are the arguments of `weakpixel new`.
type Options struct {
	Dir     string // target directory
	Module  string // module path; empty means the base name of Dir
	Name    string // Game Object group; empty means derived from the base name of Dir
	Replace string // engine checkout for a replace directive, or ""
	Force   bool   // write into a directory that is not empty
	NoGit   bool   // skip git init
	NoRun   bool   // only write the files

	title        string // window title and README heading: the base name of Dir
	version      string // engine version to require
	ecsVersion   string // ECS version to require, or "" to leave it to go mod tidy
	assetVersion string // asset module version to require, or "" to leave it to go mod tidy
}

// Run creates the project described by opts and prints what it did to stdout;
// the post-steps (git, go) also write to stderr. A bad Options is a usage
// error (exit 2); an existing file or a failed post-step is a plain error.
func Run(opts Options, stdout, stderr io.Writer) error {
	opts, err := resolve(opts)
	if err != nil {
		return cli.Usagef("%v", err)
	}
	files, err := render(opts)
	if err != nil { // a broken template is a bug of the tool
		return err
	}
	if err := write(opts, files); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "created %s (module %s, group %s)\n", opts.Dir, opts.Module, opts.Name)
	if !opts.NoRun {
		if err := postSteps(opts, stdout, stderr); err != nil {
			return err
		}
	}
	fmt.Fprintf(stdout, "\nnext:\n  cd %s\n  go run .\n", opts.Dir)
	if opts.NoRun {
		fmt.Fprintf(stdout, "  (first: go mod tidy && go generate ./...)\n")
	}
	return nil
}

// resolve cleans the directory, finds the engine version and fills in the
// defaults. It reports every problem at once.
func resolve(o Options) (Options, error) {
	o.Dir = filepath.Clean(o.Dir)
	o.version = moduleVersion(enginePath)
	o.ecsVersion = moduleVersion(ecsPath)
	o.assetVersion = moduleVersion(assetPath)
	return o, o.complete()
}

// complete fills in the defaults derived from the directory and checks the
// values, joining every problem.
func (o *Options) complete() error {
	var errs []error
	base := filepath.Base(o.Dir)
	o.title = base
	if o.Module == "" {
		o.Module = base
	}
	if err := checkModulePath(o.Module); err != nil {
		errs = append(errs, err)
	}
	if o.Name == "" {
		o.Name = groupName(base)
	}
	if !groupRE.MatchString(o.Name) {
		errs = append(errs, fmt.Errorf("group %q: want [a-z][a-z0-9_]* (set --name)", o.Name))
	} else if slices.Contains(reservedGroups, o.Name) {
		errs = append(errs, fmt.Errorf("group %q is the engine's own (set --name)", o.Name))
	}
	if o.Replace != "" {
		abs, err := filepath.Abs(o.Replace)
		if err != nil {
			errs = append(errs, fmt.Errorf("--replace: %w", err))
		} else if _, err := os.Stat(filepath.Join(abs, "go.mod")); err != nil {
			errs = append(errs, fmt.Errorf("--replace %s: no go.mod there", o.Replace))
		}
		o.Replace = abs
		if o.version == "" {
			o.version = replaceVersion
		}
	}
	if o.version == "" {
		errs = append(errs, errors.New("the engine version is unknown (the tool runs from a checkout): set --replace to the checkout, or run go run github.com/weakpixel/engine/tools/weakpixel@latest new"))
	}
	return errors.Join(errs...)
}

// groupRE is a valid group: an identifier in lower case, so it is also a
// valid +game:group:name.
var groupRE = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

// groupName derives a group from a directory name: lower case, with every
// other character turned into '_' ("My-Game" is "my_game").
func groupName(base string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(base) {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '_' {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	return b.String()
}

// modulePathRE is the character set of a module path element.
var modulePathRE = regexp.MustCompile(`^[A-Za-z0-9._~+-]+$`)

// checkModulePath checks the module path the way go mod init would, in
// short: slash-separated elements of letters, digits and ._~+-, none of
// them "." or "..".
func checkModulePath(p string) error {
	if p == "" {
		return errors.New("module path is empty (set --module)")
	}
	for _, elem := range strings.Split(p, "/") {
		if !modulePathRE.MatchString(elem) || elem == "." || elem == ".." {
			return fmt.Errorf("module path %q: bad element %q (set --module)", p, elem)
		}
	}
	return nil
}

// moduleVersion is the version of module path that this binary was built
// with: the main module's version under go run ...@version, the dependency's
// version under go tool weakpixel in a game module, or "" when unknown (a
// checkout, a replaced dependency, a workspace module, a test binary).
func moduleVersion(path string) string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return ""
	}
	v := ""
	if info.Main.Path == path {
		v = info.Main.Version
	}
	for _, d := range info.Deps {
		if d.Path == path && d.Replace == nil {
			v = d.Version
		}
	}
	if v == "(devel)" {
		return ""
	}
	return v
}
