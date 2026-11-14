package library

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"path"
	"slices"
	"strings"
)

// File names the library reads at its root.
const (
	// CatalogFile is the catalog written by weakpixel assets. When it exists,
	// [Open] reads packages from it instead of listing directories.
	CatalogFile = "assets.catalog.json"
	// ConfigFile holds optional overrides (see [Config]).
	ConfigFile = "library.json"
)

// CatalogVersion is the catalog format [Scan] writes and [Open] reads. It
// stays 0 until the engine is released.
const CatalogVersion = 0

// Catalog lists a library's packages. It is a build output of weakpixel
// assets (catalog, pack); without one, [Open] builds it by listing the FS.
type Catalog struct {
	Version  int       `json:"version"`
	Packages []Package `json:"packages"`
}

// Package is one top folder of the library root: a collection of assets
// that loads, mounts and packs together. Its lifetime is the scope of
// whoever loads it.
type Package struct {
	// Name is the folder: "global", "forest".
	Name string `json:"name"`
	// Files are the package's files as keys (root-relative paths), sorted.
	Files []string `json:"files"`
	// Entries are the keys [Library.LoadPackage] loads: files of a loadable
	// kind that no other file of the package references, plus the entries
	// listed in library.json. Their dependencies load with them.
	Entries []string `json:"entries"`
	// Uses are the other packages the package's files reference. Their
	// packs are mounted with the package's own.
	Uses []string `json:"uses,omitempty"`
	// Pack is the package's pack file (a key, "packs/forest.<hash>.zip") in
	// a packed build; empty when the files are loose.
	Pack string `json:"pack,omitempty"`
	// PackSize and PackSHA256 describe the pack file.
	PackSize   int64  `json:"packSize,omitempty"`
	PackSHA256 string `json:"packSha256,omitempty"`
}

// Config is the optional library.json at the library root.
type Config struct {
	// Entries adds keys to a package's entries, by package name: a font
	// size to preload ("global/fonts/ui.ttf#16"), a level or a file that is
	// also referenced but should load on its own.
	Entries map[string][]string `json:"entries,omitempty"`
	// Packages holds per-package settings, by package name.
	Packages map[string]PackageConfig `json:"packages,omitempty"`
}

// PackageConfig is one package's settings in library.json.
type PackageConfig struct {
	// Uses, when set, fences the package: a reference to a package not
	// listed here (other than global and boot) is an error. nil leaves the
	// package free to use any package.
	Uses []string `json:"uses,omitempty"`
}

// Severity grades a [Finding].
type Severity uint8

const (
	// Warning is a likely problem that does not break loading here (a name
	// that breaks on another file system, an unused file).
	Warning Severity = iota
	// Error breaks loading: a missing file, a reference that outlives its
	// scope.
	Error
)

func (s Severity) String() string {
	if s == Error {
		return "error"
	}
	return "warn"
}

// Finding is one problem [Scan] found.
type Finding struct {
	Severity Severity
	// Path is the file the finding is about.
	Path string
	Msg  string
}

func (f Finding) String() string { return fmt.Sprintf("%s: %s: %s", f.Path, f.Severity, f.Msg) }

// maxTextureSide is the texture size browsers commonly support; larger
// textures fail there (doc/guide/engine/web.md).
const maxTextureSide = 8192

// Scan lists fsys and builds its catalog: packages, their entries and the
// packages they use. It also returns every problem it finds, all of them,
// sorted by path: a missing global/ package, references to missing files,
// case mismatches, references that leave the root, global/ or boot/ using
// another package, references outside a package's fence, cycles between
// packages (a warning), names outside [a-z0-9._-], unused files of no
// known kind, and textures over 8192 pixels. The error is for a file
// system that cannot be listed or read, or an invalid library.json.
func Scan(fsys fs.FS) (*Catalog, []Finding, error) {
	files, err := listFiles(fsys)
	if err != nil {
		return nil, nil, err
	}
	cfg, err := readConfig(fsys)
	if err != nil {
		return nil, nil, err
	}
	s := &scanner{fsys: fsys, cfg: cfg, files: map[string]bool{}, lower: map[string]string{}, referenced: map[string]bool{}}
	for _, f := range files {
		s.files[f] = true
		s.lower[strings.ToLower(f)] = f
	}
	pkgs := map[string]*Package{}
	for _, f := range files {
		name := PackageOf(f)
		if name == "" {
			continue
		}
		g := pkgs[name]
		if g == nil {
			g = &Package{Name: name}
			pkgs[name] = g
		}
		g.Files = append(g.Files, f)
	}
	if pkgs[GlobalDir] == nil {
		s.add(Error, GlobalDir, "missing: a library needs a global/ package (loaded at boot)")
	}
	refs := map[string][]ref{} // file -> what it references
	for _, f := range files {
		s.checkName(f)
		if jsonFile(f) {
			data, err := fs.ReadFile(fsys, f)
			if err != nil {
				return nil, nil, err
			}
			refs[f] = scanRefs(f, data)
		}
		if KindOf(f) == KindTexture {
			s.checkTexture(f)
		}
	}
	for _, f := range files {
		for _, r := range refs[f] {
			s.checkRef(f, r)
		}
	}
	for _, name := range slices.Sorted(maps.Keys(cfg.Packages)) {
		if pkgs[name] == nil {
			s.add(Error, ConfigFile, fmt.Sprintf("settings for unknown package %q", name))
		}
		for _, u := range cfg.Packages[name].Uses {
			if pkgs[u] == nil {
				s.add(Error, ConfigFile, fmt.Sprintf("package %q: uses unknown package %q", name, u))
			}
		}
	}
	for _, name := range slices.Sorted(maps.Keys(cfg.Entries)) {
		g := pkgs[name]
		if g == nil {
			s.add(Error, ConfigFile, fmt.Sprintf("entries for unknown package %q", name))
			continue
		}
		for _, key := range cfg.Entries[name] {
			p, _, _ := strings.Cut(key, "#")
			if !s.files[p] {
				s.add(Error, ConfigFile, fmt.Sprintf("entry %q: no such file", key))
				continue
			}
			g.Entries = append(g.Entries, key)
		}
	}
	cat := &Catalog{Version: CatalogVersion}
	for _, name := range slices.Sorted(maps.Keys(pkgs)) {
		g := pkgs[name]
		uses := map[string]bool{}
		local := map[string]bool{} // referenced from inside the package
		for _, f := range g.Files {
			for _, r := range refs[f] {
				if r.Path == "" || !s.files[r.Path] {
					continue
				}
				if other := PackageOf(r.Path); other != "" && other != name {
					uses[other] = true
				}
				local[r.Path] = true
			}
		}
		for _, f := range g.Files {
			if entryKind(KindOf(f)) && !local[f] && !slices.Contains(g.Entries, f) {
				g.Entries = append(g.Entries, f)
			}
		}
		slices.Sort(g.Entries)
		g.Uses = slices.Sorted(maps.Keys(uses))
		cat.Packages = append(cat.Packages, *g)
	}
	s.checkCycles(cat)
	for _, f := range files {
		if KindOf(f) == KindUnknown && !s.referenced[f] && f != CatalogFile && f != ConfigFile {
			s.add(Warning, f, "unused: no known kind and nothing references it (keep source files outside the library)")
		}
	}
	slices.SortStableFunc(s.findings, func(a, b Finding) int { return strings.Compare(a.Path, b.Path) })
	return cat, s.findings, nil
}

// scanner collects findings while Scan runs.
type scanner struct {
	fsys       fs.FS
	cfg        Config
	files      map[string]bool
	lower      map[string]string // lower-case path -> path, for case mismatches
	referenced map[string]bool
	findings   []Finding
}

func (s *scanner) add(sev Severity, p, msg string) {
	s.findings = append(s.findings, Finding{Severity: sev, Path: p, Msg: msg})
}

// checkRef reports a reference from file f that does not resolve, or that
// crosses into a package f's package may not use: global/ uses only boot/,
// boot/ uses nothing, and a fenced package only what its fence lists.
func (s *scanner) checkRef(f string, r ref) {
	if r.Path == "" {
		s.add(Error, f, fmt.Sprintf("reference %q leaves the library root", r.Raw))
		return
	}
	if !s.files[r.Path] {
		if actual, ok := s.lower[strings.ToLower(r.Path)]; ok {
			s.add(Error, f, fmt.Sprintf("reference %q: case mismatch, the file is %s (embed, zip and HTTP are case-sensitive)", r.Raw, actual))
			s.referenced[actual] = true
			return
		}
		s.add(Error, f, fmt.Sprintf("reference %q: no such file %s", r.Raw, r.Path))
		return
	}
	s.referenced[r.Path] = true
	from, to := PackageOf(f), PackageOf(r.Path)
	if from == "" || to == "" || from == to {
		return
	}
	switch {
	case from == BootDir:
		s.add(Error, f, fmt.Sprintf("reference %q: boot/ uses package %s, which is not loaded yet when boot/ is; keep boot/ self-contained", r.Raw, to))
	case from == GlobalDir && to != BootDir:
		s.add(Error, f, fmt.Sprintf("reference %q: global/ uses package %s, which would stay mounted for good; move the file to global/", r.Raw, to))
	case to == GlobalDir || to == BootDir:
	default:
		if pc, ok := s.cfg.Packages[from]; ok && pc.Uses != nil && !slices.Contains(pc.Uses, to) {
			s.add(Error, f, fmt.Sprintf("reference %q: package %s uses package %s, which its uses in %s do not list", r.Raw, from, to, ConfigFile))
		}
	}
}

// checkCycles warns about each package that uses itself through other
// packages. Mounting handles cycles fine; they usually mean two packages
// should be one.
func (s *scanner) checkCycles(cat *Catalog) {
	uses := map[string][]string{}
	for _, p := range cat.Packages {
		uses[p.Name] = p.Uses
	}
	for _, p := range cat.Packages {
		if path := cyclePath(uses, p.Name); path != nil {
			s.add(Warning, p.Name, "package cycle: "+strings.Join(path, " -> "))
		}
	}
}

// cyclePath returns a path start -> ... -> start through uses, or nil. It
// searches breadth-first, so the path is a shortest one.
func cyclePath(uses map[string][]string, start string) []string {
	prev := map[string]string{}
	queue := []string{start}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for _, next := range uses[cur] {
			if next == start {
				var mid []string
				for n := cur; n != start; n = prev[n] {
					mid = append(mid, n)
				}
				slices.Reverse(mid)
				return append(append([]string{start}, mid...), start)
			}
			if _, seen := prev[next]; !seen {
				prev[next] = cur
				queue = append(queue, next)
			}
		}
	}
	return nil
}

// checkName warns about names that break on case-sensitive file systems
// or in URLs.
func (s *scanner) checkName(f string) {
	for _, seg := range strings.Split(f, "/") {
		for _, c := range seg {
			if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '.' || c == '_' || c == '-') {
				s.add(Warning, f, fmt.Sprintf("name %q: use lower-case [a-z0-9._-] only", seg))
				return
			}
		}
	}
}

// checkTexture warns about images larger than browsers support.
func (s *scanner) checkTexture(f string) {
	w, h, ok := imageSize(s.fsys, f)
	if ok && (w > maxTextureSide || h > maxTextureSide) {
		s.add(Warning, f, fmt.Sprintf("texture %dx%d is larger than %d, which fails in most browsers", w, h, maxTextureSide))
	}
}

// listFiles returns every regular file of fsys, sorted, without the
// reserved top folders (packs/, and names starting with "." or "_").
func listFiles(fsys fs.FS) ([]string, error) {
	var files []string
	err := fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && p != "." && !strings.Contains(p, "/") && reservedFolder(p) {
			return fs.SkipDir
		}
		if d.Type().IsRegular() {
			files = append(files, p)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("library: list files: %w", err)
	}
	return files, nil
}

// readConfig reads library.json; a missing file is the zero Config.
func readConfig(fsys fs.FS) (Config, error) {
	var cfg Config
	data, err := fs.ReadFile(fsys, ConfigFile)
	if errors.Is(err, fs.ErrNotExist) {
		return cfg, nil
	}
	if err != nil {
		return cfg, fmt.Errorf("library: %s: %w", ConfigFile, err)
	}
	if err := decodeStrict(data, &cfg); err != nil {
		return cfg, fmt.Errorf("library: %s: %w", ConfigFile, err)
	}
	return cfg, nil
}

// ReadCatalog parses a catalog written by [Catalog.Encode].
func ReadCatalog(data []byte) (*Catalog, error) {
	var c Catalog
	if err := decodeStrict(data, &c); err != nil {
		return nil, fmt.Errorf("library: %s: %w", CatalogFile, err)
	}
	if c.Version != CatalogVersion {
		return nil, fmt.Errorf("library: %s: version %d, want %d", CatalogFile, c.Version, CatalogVersion)
	}
	var errs []error
	for _, p := range c.Packages {
		if PackageOf(p.Name+"/x") != p.Name || strings.Contains(p.Name, "/") {
			errs = append(errs, fmt.Errorf("library: %s: %q is not a package name", CatalogFile, p.Name))
		}
	}
	return &c, errors.Join(errs...)
}

// Encode writes c as indented JSON.
func (c *Catalog) Encode() []byte {
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		panic("library: encode catalog: " + err.Error()) // plain strings and numbers cannot fail
	}
	return append(data, '\n')
}

// Package returns the package named name.
func (c *Catalog) Package(name string) (*Package, bool) {
	for i := range c.Packages {
		if c.Packages[i].Name == name {
			return &c.Packages[i], true
		}
	}
	return nil, false
}

func decodeStrict(data []byte, v any) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}

// PacksDir is the folder of pack files in a packed build.
const PacksDir = "packs"

// packKey returns the key of a package's pack file with its content hash:
// "packs/forest.<hash>.zip".
func packKey(pkg, sum string) string {
	return path.Join(PacksDir, pkg+"."+sum[:12]+".zip")
}
