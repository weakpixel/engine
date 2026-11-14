package marker

import (
	"fmt"
	"go/ast"
	"go/token"
	"maps"
	"slices"
	"strings"
)

// Diag is a problem found while collecting markers, anchored at the comment
// that holds it.
type Diag struct {
	Pos token.Pos
	Msg string
	// Related is a second position the message refers to (for example the
	// marker that already took a slot), or token.NoPos. The library has no
	// FileSet, so the caller resolves it when it words the message.
	Related token.Pos
}

// Marker is one collected marker: its full name, where it was written and its
// parsed value (the definition's struct, or PathArgs for a path marker).
//
// A repeatable marker (Registry.DefinePaths) may appear on several lines. Its
// Lines hold one Marker per line, in source order; Pos is the first line's and
// Value is nil.
type Marker struct {
	Name  string
	Pos   token.Pos
	Value any
	Lines []Marker
}

// Set holds the markers collected from one comment group, keyed by full name.
// The zero Marker (invalid Pos) is returned for a name that is absent.
type Set map[string]Marker

// Has reports whether the set holds the marker name.
func (s Set) Has(name string) bool { return s[name].Pos.IsValid() }

// Lines returns the lines of the repeatable marker name, or nil.
func (s Set) Lines(name string) []Marker { return s[name].Lines }

// Values returns the values of the repeatable struct marker name as Ts, one per
// line in source order. Lines whose value is another type are skipped.
func Values[T any](s Set, name string) []T {
	var out []T
	for _, line := range s[name].Lines {
		if v, ok := line.Value.(T); ok {
			out = append(out, v)
		}
	}
	return out
}

// Value returns the value of the marker name as a T. It reports false when the
// marker is absent or was defined with a different struct type.
func Value[T any](s Set, name string) (T, bool) {
	v, ok := s[name].Value.(T)
	return v, ok
}

// Option configures a Registry.
type Option func(*Registry)

// WithPrefix restricts Collect to markers whose full name starts with prefix
// (for example "game:"). Every other `+...` comment, such as `+build` or a
// foreign tool's markers, is ignored silently. Without the option all `+name`
// comments are markers and unknown names are reported.
func WithPrefix(prefix string) Option {
	return func(r *Registry) { r.prefix = prefix }
}

// Collect reads the whole-line `// +name...` markers of a comment group
// attached to a declaration of the given single target. Comments that are not
// markers (see WithPrefix) are skipped. Unknown markers, markers not valid on
// target, bad arguments and duplicates are returned as diagnostics; the
// remaining markers are still collected. group may be nil.
func (r *Registry) Collect(group *ast.CommentGroup, target TargetType) (Set, []Diag) {
	set := Set{}
	if group == nil {
		return set, nil
	}
	var diags []Diag
	for _, comment := range group.List {
		text, ok := strings.CutPrefix(comment.Text, "//")
		if !ok {
			continue
		}
		text = strings.TrimSpace(text)
		if !strings.HasPrefix(text, "+"+r.prefix) {
			continue
		}
		name, value, err := r.Parse(target, text)
		if err != nil {
			diags = append(diags, Diag{Pos: comment.Pos(), Msg: err.Error()})
			continue
		}
		if def, _ := r.Lookup(name); def.Repeatable() {
			m := set[name]
			if len(m.Lines) == 0 {
				m = Marker{Name: name, Pos: comment.Pos()}
			}
			m.Lines = append(m.Lines, Marker{Name: name, Pos: comment.Pos(), Value: value})
			set[name] = m
			continue
		}
		if set.Has(name) {
			diags = append(diags, Diag{Pos: comment.Pos(), Msg: fmt.Sprintf("duplicate marker +%s", name)})
			continue
		}
		set[name] = Marker{Name: name, Pos: comment.Pos(), Value: value}
	}
	for _, name := range slices.Sorted(maps.Keys(set)) {
		if def, _ := r.Lookup(name); def.check != nil {
			diags = append(diags, def.check(set[name].Lines)...)
		}
	}
	return set, diags
}

// Stray reports every marker comment of file that is not in one of the
// attached comment groups: a marker inside a function body, in a floating
// comment, or on a declaration the caller did not collect. Use it after
// collecting the declarations that may carry markers, passing their groups
// (nil groups are ignored). Comments that are not markers of this registry (see
// WithPrefix) are skipped. Groups are matched by identity, so pass the groups
// of file itself (`decl.Doc`, `file.Doc`).
func (r *Registry) Stray(file *ast.File, attached ...*ast.CommentGroup) []Diag {
	var diags []Diag
	for _, group := range file.Comments {
		if slices.Contains(attached, group) {
			continue
		}
		for _, comment := range group.List {
			text, ok := strings.CutPrefix(comment.Text, "//")
			if !ok {
				continue
			}
			text = strings.TrimSpace(text)
			if !strings.HasPrefix(text, "+"+r.prefix) {
				continue
			}
			name := strings.TrimPrefix(text, "+")
			if def, _ := r.match(name); def != nil {
				name = def.Name
			} else {
				if i := strings.IndexAny(name, " \t="); i >= 0 {
					name = name[:i]
				}
			}
			diags = append(diags, Diag{Pos: comment.Pos(), Msg: fmt.Sprintf("marker +%s: not attached to a declaration", name)})
		}
	}
	return diags
}

// PathSetting is one `path=value` assignment of a path marker. Path is ""
// for the bare `=value` form (the declaration itself).
type PathSetting struct {
	Pos         token.Pos
	Path, Value string
}

// PathIssueKind tells what is wrong with the settings of a path marker.
type PathIssueKind int

const (
	// PathDuplicate: the same Path is set more than once.
	PathDuplicate PathIssueKind = iota + 1
	// PathMixedForms: the bare form (=value) is combined with the path form
	// (:path=value). No settings are returned.
	PathMixedForms
)

// PathIssue is a problem in the settings of a path marker; the caller words
// the message.
type PathIssue struct {
	Pos  token.Pos
	Kind PathIssueKind
	Path string // the repeated path, for PathDuplicate
}

// PathSettings flattens the lines of a path marker (Set.Lines) into settings,
// in source order. A path set twice is reported (and still returned); mixing the
// bare and the path form is reported and yields no settings.
func PathSettings(lines []Marker) ([]PathSetting, []PathIssue) {
	var out []PathSetting
	var issues []PathIssue
	hasBare, hasPath := false, false
	for _, line := range lines {
		args, _ := line.Value.(PathArgs)
		if args.HasBare {
			out = append(out, PathSetting{line.Pos, "", args.Bare})
			hasBare = true
		}
		for _, pv := range args.Paths {
			out = append(out, PathSetting{line.Pos, pv.Path, pv.Value})
			hasPath = true
		}
	}
	seen := map[string]bool{}
	for _, d := range out {
		if seen[d.Path] {
			issues = append(issues, PathIssue{d.Pos, PathDuplicate, d.Path})
		}
		seen[d.Path] = true
	}
	if hasBare && hasPath {
		issues = append(issues, PathIssue{lines[0].Pos, PathMixedForms, ""})
		return nil, issues
	}
	return out, issues
}
