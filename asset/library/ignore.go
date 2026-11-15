package library

import (
	"errors"
	"fmt"
	"io/fs"
	"path"
	"slices"
	"strings"
)

// IgnoreFile is the name of the files that keep files out of a library:
// one in the root or in any folder below it, with .gitignore syntax and
// meaning, relative to the folder that holds it.
const IgnoreFile = ".assetignore"

// Ignore holds the rules of every [IgnoreFile] of a library. Ignored files
// are not part of the library: they are not listed, packed, embedded or
// put in the catalog, and [Open] refuses to read them from a base it
// lists itself. The nil *Ignore ignores nothing.
//
// An Ignore is read-only once built and safe for concurrent use.
type Ignore struct {
	// rules by the folder of their file ("" for the root), each in file
	// order. A folder's rules apply to the paths below it.
	rules map[string][]ignoreRule
}

// ignoreRule is one pattern line of an ignore file.
type ignoreRule struct {
	dir      string   // folder of the file, "" for the root
	line     int      // 1-based line in the file
	negate   bool     // "!pattern": re-include
	dirOnly  bool     // "pattern/": folders only
	anchored bool     // a "/" at the start or in the middle: relative to dir, not any depth
	segs     []string // the pattern split at "/", each a path.Match pattern or "**"
}

// by names the rule's file and line: "forest/.assetignore:3".
func (r ignoreRule) by() string {
	return fmt.Sprintf("%s:%d", path.Join(r.dir, IgnoreFile), r.line)
}

// IgnoreError is a bad line of an [IgnoreFile]. The line is skipped; the
// other lines of the file still apply.
type IgnoreError struct {
	File string // the ignore file: "forest/.assetignore"
	Line int    // 1-based
	Msg  string // "bad pattern \"a[\""
}

func (e *IgnoreError) Error() string { return fmt.Sprintf("%s:%d: %s", e.File, e.Line, e.Msg) }

// parseIgnore parses the ignore file of folder dir. It returns the valid
// rules and one *IgnoreError per bad line, so a typo on one line does not
// drop the others.
func parseIgnore(dir string, data []byte) ([]ignoreRule, []error) {
	var rules []ignoreRule
	var errs []error
	file := path.Join(dir, IgnoreFile)
	for i, line := range strings.Split(string(data), "\n") {
		r, ok, err := parseIgnoreLine(dir, i+1, line)
		if err != nil {
			errs = append(errs, &IgnoreError{File: file, Line: i + 1, Msg: err.Error()})
			continue
		}
		if ok {
			rules = append(rules, r)
		}
	}
	return rules, errs
}

// parseIgnoreLine parses one line with git's rules: blank lines and "#"
// comments are nothing (ok false); "\#" and "\!" escape the first
// character; trailing spaces go unless escaped with "\"; "!" negates; a
// trailing "/" matches folders only; a "/" at the start or in the middle
// anchors the pattern to dir.
func parseIgnoreLine(dir string, n int, line string) (ignoreRule, bool, error) {
	line = strings.TrimSuffix(line, "\r")
	for strings.HasSuffix(line, " ") && !strings.HasSuffix(line, `\ `) {
		line = line[:len(line)-1]
	}
	if line == "" || strings.HasPrefix(line, "#") {
		return ignoreRule{}, false, nil
	}
	r := ignoreRule{dir: dir, line: n}
	if rest, ok := strings.CutPrefix(line, "!"); ok {
		r.negate, line = true, rest
	}
	if rest, ok := strings.CutSuffix(line, "/"); ok {
		r.dirOnly, line = true, rest
	}
	if strings.Contains(line, "/") {
		r.anchored = true
	}
	for _, seg := range strings.Split(line, "/") {
		if seg == "" {
			continue // a leading "/" or "a//b"
		}
		if seg != "**" {
			// path.Match knows *, ?, [...] and \ like git; git also
			// negates a class with "!", path.Match only with "^".
			seg = strings.ReplaceAll(seg, "[!", "[^")
			if _, err := path.Match(seg, ""); err != nil {
				return ignoreRule{}, false, fmt.Errorf("bad pattern %q", line)
			}
		}
		r.segs = append(r.segs, seg)
	}
	if len(r.segs) == 0 {
		return ignoreRule{}, false, errors.New("empty pattern")
	}
	return r, true, nil
}

// matches reports whether the rule matches rel, the path relative to the
// rule's folder.
func (r ignoreRule) matches(rel string, isDir bool) bool {
	if r.dirOnly && !isDir {
		return false
	}
	if !r.anchored {
		// A pattern without a slash matches the name at any depth.
		return matchSegment(r.segs[0], path.Base(rel))
	}
	return matchSegments(r.segs, rel)
}

// matchSegments matches the pattern segments against the "/"-separated
// name. "**" matches zero or more whole segments, except as the last
// segment, where it needs at least one ("abc/**" is what is inside abc,
// not abc). It walks the name in place, so it does not allocate.
func matchSegments(segs []string, name string) bool {
	for i, seg := range segs {
		if seg == "**" {
			rest := segs[i+1:]
			if len(rest) == 0 {
				return name != ""
			}
			for {
				if matchSegments(rest, name) {
					return true
				}
				_, after, ok := strings.Cut(name, "/")
				if !ok {
					return false
				}
				name = after
			}
		}
		if name == "" {
			return false
		}
		head, after, _ := strings.Cut(name, "/")
		if !matchSegment(seg, head) {
			return false
		}
		name = after
	}
	return name == ""
}

// matchSegment matches one segment; the patterns were checked when parsed.
func matchSegment(pattern, name string) bool {
	ok, _ := path.Match(pattern, name)
	return ok
}

// add adds the rules of the ignore file of dir.
func (ig *Ignore) add(dir string, rules []ignoreRule) {
	if len(rules) == 0 {
		return
	}
	if ig.rules == nil {
		ig.rules = map[string][]ignoreRule{}
	}
	ig.rules[dir] = append(ig.rules[dir], rules...)
}

// Match reports whether the file (or, with isDir, the folder) at p, a
// path from the library root, is ignored, and names the rule that decided
// it ("forest/.assetignore:3"), also for a re-include. As in git, a path
// inside an ignored folder is ignored whatever the rules below say, and
// of the rules that match a path the one in the deepest file, and there
// the last line, wins.
func (ig *Ignore) Match(p string, isDir bool) (ignored bool, by string) {
	if ig == nil || len(ig.rules) == 0 {
		return false, ""
	}
	// Each parent folder first, top down: an ignored folder ignores all
	// below it, and re-including inside it is impossible.
	for i := 0; i < len(p); i++ {
		if p[i] == '/' {
			if ok, by := ig.matchOne(p[:i], true); ok {
				return true, by
			}
		}
	}
	return ig.matchOne(p, isDir)
}

// matchOne decides p by the rules of the folders above it alone.
func (ig *Ignore) matchOne(p string, isDir bool) (bool, string) {
	dir := p
	for {
		// The folder of the next ignore file up: the parent of dir.
		if i := strings.LastIndexByte(dir, '/'); i >= 0 {
			dir = dir[:i]
		} else if dir != "" {
			dir = ""
		} else {
			return false, ""
		}
		rules := ig.rules[dir]
		rel := p
		if dir != "" {
			rel = p[len(dir)+1:]
		}
		for i := len(rules) - 1; i >= 0; i-- {
			if rules[i].matches(rel, isDir) {
				return !rules[i].negate, rules[i].by()
			}
		}
		if dir == "" {
			return false, ""
		}
	}
}

// Empty reports whether ig has no rules: nothing is ignored.
func (ig *Ignore) Empty() bool { return ig == nil || len(ig.rules) == 0 }

// Files returns the folders that hold an [IgnoreFile] with at least one
// rule, as paths of the ignore files, sorted.
func (ig *Ignore) Files() []string {
	if ig == nil {
		return nil
	}
	var out []string
	for dir := range ig.rules {
		out = append(out, path.Join(dir, IgnoreFile))
	}
	slices.Sort(out)
	return out
}

// ReadIgnore reads every [IgnoreFile] of fsys, skipping those inside
// ignored folders (they cannot change anything). The error joins one
// error per bad line ("forest/.assetignore:3: bad pattern ..."); the
// returned Ignore holds the valid lines even then. A file system that
// cannot be listed gives the error of the walk and a nil Ignore.
func ReadIgnore(fsys fs.FS) (*Ignore, error) {
	ig, bad, err := walkLibrary(fsys, nil)
	if err != nil {
		return nil, err
	}
	return ig, errors.Join(bad...)
}

// hasIgnoreFile reports whether the listing of folder dir ("" for the
// root) has an [IgnoreFile].
func hasIgnoreFile(fsys fs.FS, dir string) bool {
	if dir == "" {
		dir = "."
	}
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		return false
	}
	_, found := slices.BinarySearchFunc(entries, IgnoreFile, func(e fs.DirEntry, name string) int {
		return strings.Compare(e.Name(), name)
	})
	return found
}

// walkLibrary walks fsys top down, reading each folder's ignore file
// before the folder's entries, so the rules apply while walking. It calls
// file (if not nil) for every regular file that is not ignored, not an
// ignore file and not in a reserved top folder (packs/, names starting
// with "." or "_"), in lexical order. Ignored folders are not entered: git
// cannot re-include below an ignored folder, so pruning is exact. bad
// holds the errors of bad ignore lines.
func walkLibrary(fsys fs.FS, file func(p string)) (ig *Ignore, bad []error, err error) {
	ig = &Ignore{}
	err = fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if p == "." {
				p = ""
			} else if !strings.Contains(p, "/") && reservedFolder(p) {
				return fs.SkipDir
			} else if ok, _ := ig.Match(p, true); ok {
				return fs.SkipDir
			}
			// Look in the listing before reading: over HTTP (the editor)
			// a read of a missing file is a request per folder, and a
			// server may answer it with something other than 404.
			if !hasIgnoreFile(fsys, p) {
				return nil
			}
			data, err := fs.ReadFile(fsys, path.Join(p, IgnoreFile))
			if err != nil {
				return err
			}
			rules, errs := parseIgnore(p, data)
			ig.add(p, rules)
			bad = append(bad, errs...)
			return nil
		}
		if !d.Type().IsRegular() || path.Base(p) == IgnoreFile {
			return nil
		}
		if ok, _ := ig.Match(p, false); ok {
			return nil
		}
		if file != nil {
			file(p)
		}
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	return ig, bad, nil
}
