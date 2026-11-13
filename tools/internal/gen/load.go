package gen

import (
	"encoding/json"
	"fmt"
	"go/ast"
	"go/token"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"golang.org/x/tools/go/packages"

	"github.com/weakpixel/engine/internal/marker"
)

const (
	outputName     = "zz__gen.go"
	jsonOutputName = "zz__gen.god.json"
	specSuffix     = ".god.json" // <group>.god.json in the specs folder
)

// typeDecl is the syntax of a named type declaration in any loaded package.
type typeDecl struct {
	pkg  *packages.Package
	spec *ast.TypeSpec
	doc  *ast.CommentGroup

	parsed  bool
	markers marker.Set
	diags   []diag
}

// docText is the documentation of the declaration (without marker lines).
func (d *typeDecl) docText() string {
	if doc := docText(d.doc); doc != "" {
		return doc
	}
	return docText(d.spec.Comment)
}

// structType returns the struct syntax of the declaration, or nil.
func (d *typeDecl) structType() *ast.StructType {
	st, _ := d.spec.Type.(*ast.StructType)
	return st
}

// typeMarkers parses (once) the declaration's markers.
func (d *typeDecl) typeMarkers() (marker.Set, []diag) {
	if !d.parsed {
		d.parsed = true
		d.markers, d.diags = registry.Collect(d.doc, marker.DescribesType)
	}
	return d.markers, d.diags
}

// loaded is the result of loading the target packages and their dependencies.
type loaded struct {
	dir     string
	fset    *token.FileSet
	targets []*packages.Package
	decls   map[token.Pos]*typeDecl  // keyed by the type name identifier position
	fields  map[token.Pos]*ast.Field // keyed by the field identifier position (see indexFields)
	consts  map[token.Pos]string     // doc of each package-level constant, keyed by its name position
	groups  map[*packages.Package]*groupInfo
}

// groupInfo is the cached +game:group:name of a package ("" when unset).
type groupInfo struct {
	name string
}

// load resolves patterns relative to dir. Existing generated files of the
// targets are blanked with an overlay so stale output never affects loading.
// Type errors are tolerated here; they only matter when they hit a marked
// declaration, which parse reports.
func load(dir string, patterns []string) (*loaded, error) {
	dir, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}

	roots, err := packages.Load(&packages.Config{
		Mode: packages.NeedName | packages.NeedFiles,
		Dir:  dir,
	}, patterns...)
	if err != nil {
		return nil, err
	}
	overlay := map[string][]byte{}
	for _, root := range roots {
		for _, file := range root.GoFiles {
			if filepath.Base(file) == outputName {
				overlay[file] = []byte("package " + root.Name + "\n")
			}
		}
	}

	fset := token.NewFileSet()
	targets, err := packages.Load(&packages.Config{
		Mode: packages.NeedName | packages.NeedFiles | packages.NeedSyntax |
			packages.NeedTypes | packages.NeedTypesInfo | packages.NeedImports | packages.NeedDeps,
		Dir:     dir,
		Fset:    fset,
		Overlay: overlay,
	}, patterns...)
	if err != nil {
		return nil, err
	}
	if len(targets) == 0 {
		return nil, fmt.Errorf("no packages match %s", strings.Join(patterns, " "))
	}

	l := &loaded{dir: dir, fset: fset, decls: map[token.Pos]*typeDecl{}, fields: map[token.Pos]*ast.Field{}, consts: map[token.Pos]string{}, groups: map[*packages.Package]*groupInfo{}}
	for _, target := range targets {
		for _, e := range target.Errors {
			if e.Kind == packages.ListError || e.Kind == packages.ParseError {
				return nil, fmt.Errorf("%s: %s", target.PkgPath, e.Msg)
			}
		}
		l.targets = append(l.targets, target)
	}
	slices.SortFunc(l.targets, func(a, b *packages.Package) int { return strings.Compare(a.PkgPath, b.PkgPath) })

	packages.Visit(targets, nil, l.index)
	return l, nil
}

// index records every top-level type declaration of pkg.
func (l *loaded) index(pkg *packages.Package) {
	for _, file := range pkg.Syntax {
		indexFields(file, l.fields)
		for _, decl := range file.Decls {
			gd, ok := decl.(*ast.GenDecl)
			if ok && gd.Tok == token.CONST {
				l.indexConsts(gd)
			}
			if !ok || gd.Tok != token.TYPE {
				continue
			}
			for _, spec := range gd.Specs {
				ts := spec.(*ast.TypeSpec)
				doc := ts.Doc
				if doc == nil && !gd.Lparen.IsValid() {
					doc = gd.Doc
				}
				l.decls[ts.Name.Pos()] = &typeDecl{pkg: pkg, spec: ts, doc: doc}
			}
		}
	}
}

// indexConsts records the documentation of each constant of gd: its spec's
// doc comment (or the declaration's, when not parenthesized), else its
// trailing line comment, like fields.
func (l *loaded) indexConsts(gd *ast.GenDecl) {
	for _, spec := range gd.Specs {
		vs := spec.(*ast.ValueSpec)
		doc := vs.Doc
		if doc == nil && !gd.Lparen.IsValid() {
			doc = gd.Doc
		}
		text := docText(doc)
		if text == "" {
			text = docText(vs.Comment)
		}
		for _, name := range vs.Names {
			l.consts[name.Pos()] = text
		}
	}
}

// position formats pos as a path relative to the load directory.
func (l *loaded) position(pos token.Pos) string {
	p := l.fset.Position(pos)
	if rel, err := filepath.Rel(l.dir, p.Filename); err == nil {
		p.Filename = filepath.ToSlash(rel)
	}
	return fmt.Sprintf("%s:%d:%d", p.Filename, p.Line, p.Column)
}

// packageDir returns the directory the target's generated file belongs in.
func packageDir(pkg *packages.Package) string {
	if pkg.Dir != "" {
		return pkg.Dir
	}
	if len(pkg.GoFiles) > 0 {
		return filepath.Dir(pkg.GoFiles[0])
	}
	return ""
}

// removeStale deletes the named generated files in dir if this tool wrote them.
func removeStale(dir string, names ...string) error {
	for _, name := range names {
		path := filepath.Join(dir, name)
		data, err := os.ReadFile(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		if !writtenByGenerator(name, data) {
			continue
		}
		if err := os.Remove(path); err != nil {
			return err
		}
	}
	return nil
}

// removeSpecs deletes the GOD files in the specs folder that this tool wrote
// for package pkgPath, except keep (a file name, or "").
func removeSpecs(specs, pkgPath, keep string) error {
	entries, err := os.ReadDir(specs)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.IsDir() || e.Name() == keep || !strings.HasSuffix(e.Name(), specSuffix) {
			continue
		}
		path := filepath.Join(specs, e.Name())
		if pkg, ok := specPackage(path); ok && pkg == pkgPath {
			if err := os.Remove(path); err != nil {
				return err
			}
		}
	}
	return nil
}

// specPackage returns the "package" of a GOD file this tool wrote, and false
// when path is missing or not such a file.
func specPackage(path string) (string, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	var f struct {
		Generator string `json:"generator"`
		Package   string `json:"package"`
	}
	if json.Unmarshal(data, &f) != nil || f.Generator != godGenerator {
		return "", false
	}
	return f.Package, true
}

// writtenByGenerator reports whether data is a file this tool generated: the
// Go file by its header, the header-less JSON file by its "generator" field.
func writtenByGenerator(name string, data []byte) bool {
	if name == outputName {
		return strings.HasPrefix(string(data), generatedHeader)
	}
	var f struct {
		Generator string `json:"generator"`
	}
	return json.Unmarshal(data, &f) == nil && f.Generator == godGenerator
}
