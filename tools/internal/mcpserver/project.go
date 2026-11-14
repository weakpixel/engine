package mcpserver

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/weakpixel/engine/asset/library"
	"github.com/weakpixel/engine/god"
	"github.com/weakpixel/engine/internal/project"
	"github.com/weakpixel/engine/tools/internal/goddoc"
)

// enginePath is the module whose godspecs folder holds the GOD files of the
// engine's own components (gfx, collision, physics, room). A game's specs
// folder lists only the engine components its own objects use, while its
// levels may add any registered one.
const enginePath = "github.com/weakpixel/engine"

// isLibraryRoot reports whether dir is an asset library root: it holds a
// library.json or a global/ folder. That marks the asset root when
// assets.root is not set (the VSCode extension's rule, see
// doc/guide/project-file.md).
func isLibraryRoot(entries []os.DirEntry) bool {
	for _, e := range entries {
		if e.IsDir() && e.Name() == library.GlobalDir || !e.IsDir() && e.Name() == library.ConfigFile {
			return true
		}
	}
	return false
}

// Project is the game project the server works on: its settings, its
// asset root and its merged GOD, which is reloaded when a GOD file changes.
type Project struct {
	// Root is the project directory: the folder of .weakpixel.json, else
	// the Go module root, else the directory the server started in.
	Root string
	// SettingsFile is the .weakpixel.json, "" without one.
	SettingsFile string
	Settings     *project.Settings
	// AssetRoot is the asset library root, "" when none was found.
	AssetRoot string
	// GODInputs are the files and folders the GOD is read from: god.specs
	// (else the project root) and the engine's godspecs folder.
	GODInputs []string
	// Game and Tags are the package and build tags of the editor build.
	Game string
	Tags []string
	// Warnings say what the server could not find; project_info lists them.
	Warnings []string

	mu    sync.Mutex
	stamp string
	god   *godData
}

// godData is one load of the merged GOD.
type godData struct {
	err        error
	components []god.ComponentSchema
	objects    []god.Schema
	compByKey  map[string]*god.ComponentSchema
	objByKey   map[string]*god.Schema
}

// ProjectOptions select the project.
type ProjectOptions struct {
	Dir  string   // where the lookup starts; "" is the working directory
	Game string   // editor package directory, overriding editor.game
	Tags []string // editor build tags, overriding editor.tags
}

// OpenProject finds the project from opts.Dir and reads its settings. Only
// an unreadable .weakpixel.json is an error; anything else missing becomes a
// warning, so the tools that do not need it still work.
func OpenProject(opts ProjectOptions) (*Project, error) {
	dir := opts.Dir
	if dir == "" {
		dir = "."
	}
	dir, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	settings, file, err := project.Load(dir)
	if err != nil {
		return nil, err
	}
	p := &Project{SettingsFile: file, Settings: settings, Root: dir}
	if settings != nil {
		p.Root = settings.Dir
	} else if mod := moduleRoot(dir); mod != "" {
		p.Root = mod
	}
	if settings == nil {
		p.Warnings = append(p.Warnings, "no .weakpixel.json found; defaults are used (doc/guide/project-file.md)")
		settings = &project.Settings{}
	}

	p.AssetRoot = settings.AssetsRoot
	if p.AssetRoot == "" {
		p.AssetRoot = findLibrary(p.Root)
	}
	if p.AssetRoot == "" {
		p.Warnings = append(p.Warnings, "no asset root: set assets.root in .weakpixel.json")
	} else if fi, err := os.Stat(p.AssetRoot); err != nil || !fi.IsDir() {
		p.Warnings = append(p.Warnings, fmt.Sprintf("asset root %s is not a folder", p.AssetRoot))
		p.AssetRoot = ""
	} else if real, err := filepath.EvalSymlinks(p.AssetRoot); err == nil {
		p.AssetRoot = real
	}

	if def := filepath.Join(p.Root, filepath.FromSlash(project.DefaultGODSpecs)); settings.GODSpecs != "" {
		p.GODInputs = append(p.GODInputs, settings.GODSpecs)
	} else if fi, err := os.Stat(def); err == nil && fi.IsDir() {
		p.GODInputs = append(p.GODInputs, def)
	} else {
		p.GODInputs = append(p.GODInputs, p.Root)
	}
	if engine, err := engineGOD(p.Root); err != nil {
		p.Warnings = append(p.Warnings, fmt.Sprintf("engine GOD files not found (%v); engine components outside god.specs are unknown", err))
	} else if !slices.Contains(p.GODInputs, engine) {
		p.GODInputs = append(p.GODInputs, engine)
	}

	p.Game = cmpOr(opts.Game, settings.EditorGame)
	if p.Game != "" && !filepath.IsAbs(p.Game) {
		p.Game = filepath.Join(dir, p.Game)
	}
	p.Tags = opts.Tags
	if len(p.Tags) == 0 {
		p.Tags = settings.EditorTags
	}
	if len(p.Tags) == 0 {
		p.Tags = []string{"editor"}
	}
	return p, nil
}

func cmpOr(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// moduleRoot returns the first directory at or above dir with a go.mod, or "".
func moduleRoot(dir string) string {
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

// findLibrary returns the first folder at or below root (breadth first, at
// most three levels down) that is a library root (isLibraryRoot).
func findLibrary(root string) string {
	level := []string{root}
	for depth := 0; depth <= 3 && len(level) > 0; depth++ {
		var next []string
		for _, dir := range level {
			entries, err := os.ReadDir(dir)
			if err != nil {
				continue
			}
			if isLibraryRoot(entries) {
				return dir
			}
			for _, e := range entries {
				if e.IsDir() && !skippedDir(e.Name()) {
					next = append(next, filepath.Join(dir, e.Name()))
				}
			}
		}
		level = next
	}
	return ""
}

// skippedDir reports folders no search walks into, as the go tool skips them.
func skippedDir(name string) bool {
	switch name {
	case "vendor", "testdata", "node_modules", "build":
		return true
	}
	return strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_")
}

// engineGOD returns the godspecs folder of the engine module the project
// uses, as `go list -m` reports it (a local replace or the module cache).
func engineGOD(root string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "go", "list", "-m", "-f", "{{.Dir}}", enginePath)
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) && len(exit.Stderr) > 0 {
			return "", fmt.Errorf("go list -m %s: %s", enginePath, strings.TrimSpace(string(exit.Stderr)))
		}
		return "", fmt.Errorf("go list -m %s: %w", enginePath, err)
	}
	dir := filepath.Join(strings.TrimSpace(string(out)), "godspecs")
	if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
		return "", fmt.Errorf("%s is not a folder", dir)
	}
	return dir, nil
}

// GOD returns the merged GOD, reloading it when a GOD file was added,
// removed or changed since the last call.
func (p *Project) GOD() *godData {
	stamp := godStamp(p.GODInputs)
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.god != nil && stamp == p.stamp {
		return p.god
	}
	g := &godData{compByKey: map[string]*god.ComponentSchema{}, objByKey: map[string]*god.Schema{}}
	g.components, g.objects, g.err = goddoc.Load(p.GODInputs)
	for i := range g.components {
		g.compByKey[g.components[i].Key()] = &g.components[i]
	}
	for i := range g.objects {
		g.objByKey[g.objects[i].Key()] = &g.objects[i]
	}
	p.god, p.stamp = g, stamp
	return g
}

// godStamp lists every GOD file of inputs with its size and time, so a
// change to any of them changes the stamp.
func godStamp(inputs []string) string {
	var b strings.Builder
	for _, in := range inputs {
		_ = filepath.WalkDir(in, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if d.IsDir() {
				if p != in && skippedDir(d.Name()) {
					return filepath.SkipDir
				}
				return nil
			}
			if strings.HasSuffix(p, ".god.json") {
				if fi, err := d.Info(); err == nil {
					fmt.Fprintf(&b, "%s %d %d\n", p, fi.Size(), fi.ModTime().UnixNano())
				}
			}
			return nil
		})
	}
	return b.String()
}

// errNoAssetRoot is returned by the tools that need an asset root.
var errNoAssetRoot = errors.New("the project has no asset root: set assets.root in .weakpixel.json")

// AssetPath resolves key, an asset key ("forest/forest.level.json") or an
// absolute path inside the asset root, to an absolute path and the clean
// key. It refuses paths that leave the asset root: "..", absolute paths
// elsewhere and symbolic links that point outside it.
func (p *Project) AssetPath(key string) (abs, clean string, err error) {
	if p.AssetRoot == "" {
		return "", "", errNoAssetRoot
	}
	return within(p.AssetRoot, key, "the asset root")
}

// within resolves name inside root (see AssetPath); what names root in errors.
func within(root, name, what string) (abs, clean string, err error) {
	if name == "" {
		return "", "", errors.New("empty path")
	}
	rel := name
	if filepath.IsAbs(name) {
		if rel, err = filepath.Rel(root, filepath.Clean(name)); err != nil {
			return "", "", fmt.Errorf("%s is outside %s", name, what)
		}
	}
	rel = path.Clean(filepath.ToSlash(rel))
	if rel == "." || rel == ".." || strings.HasPrefix(rel, "../") || path.IsAbs(rel) {
		return "", "", fmt.Errorf("%s is outside %s (paths are relative to %s, without ..)", name, what, root)
	}
	abs = filepath.Join(root, filepath.FromSlash(rel))
	// Resolve symbolic links of the longest existing prefix, so a link
	// inside the root cannot point the tools outside it.
	existing := abs
	for {
		if _, err := os.Lstat(existing); err == nil {
			break
		}
		parent := filepath.Dir(existing)
		if parent == existing {
			break
		}
		existing = parent
	}
	real, err := filepath.EvalSymlinks(existing)
	if err != nil {
		return "", "", err
	}
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", "", err
	}
	if r, err := filepath.Rel(realRoot, real); err != nil || r == ".." || strings.HasPrefix(r, ".."+string(filepath.Separator)) {
		return "", "", fmt.Errorf("%s leads outside %s through a symbolic link", name, what)
	}
	return abs, rel, nil
}
