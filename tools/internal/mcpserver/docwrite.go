package mcpserver

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// editFunc turns the text of a document into its new text, usually with
// internal/jsonedit so that only the edited value changes. doc is the
// parsed current text.
type editFunc func(text []byte, doc *document, info docInfo) ([]byte, error)

// docInfo says which document an edit is for.
type docInfo struct {
	key  string // clean asset key
	kind string // "level" or "prefab"
}

// chain is the prefab chain to start expansions with: a prefab expands
// from itself, so a variant that names itself is a cycle.
func (i docInfo) chain() []string {
	if i.kind == "prefab" {
		return []string{i.key}
	}
	return nil
}

// editDocument is how every edit tool changes a document (plan D3, D5): it
// reads the file, refuses a stale expect (the sha read_document returned),
// applies edit to the current text, runs the offline checks on the result
// and refuses it when it has a problem the old text did not have, then
// writes it atomically. It answers with the new sha, the changed lines and
// the problems that remain. With a game running on the document, the new text
// also goes to the game, which can refuse it (see [tools.writeDocument]).
func (t *tools) editDocument(key, expect string, edit editFunc) (map[string]any, error) {
	t.docMu.Lock()
	defer t.docMu.Unlock()
	clean, kind, data, err := t.readDoc(key)
	if err != nil {
		return nil, err
	}
	if err := checkExpect(clean, expect, data); err != nil {
		return nil, err
	}
	doc, before := t.checkDocument(clean, data)
	if doc == nil {
		return nil, fmt.Errorf("%s does not parse, so no edit is made: %s", clean, strings.Join(before, "; "))
	}
	next, err := edit(data, doc, docInfo{clean, kind})
	if err != nil {
		return nil, fmt.Errorf("%s: %w", clean, err)
	}
	return t.commit(clean, data, next, before)
}

// checkExpect refuses an edit based on an older text than the file's.
func checkExpect(key, expect string, data []byte) error {
	if expect != "" && expect != sha(data) {
		return fmt.Errorf("%s changed since it was read (sha is %s, expected %s): read_document again and redo the edit", key, sha(data), expect)
	}
	return nil
}

// commit checks next, the new text of the document key whose current text
// is data (with the problems `before` found in it), and writes it.
func (t *tools) commit(key string, data, next []byte, before []string) (map[string]any, error) {
	_, after := t.checkDocument(key, next)
	if added := newProblems(before, after); len(added) > 0 {
		return nil, fmt.Errorf("the edit would leave %s with problems, so nothing was written:\n- %s", key, strings.Join(added, "\n- "))
	}
	out := map[string]any{"path": key, "problems": nonNil(after)}
	if bytes.Equal(data, next) {
		out["changed"], out["sha"], out["diff"] = false, sha(data), ""
		return out, nil
	}
	abs, _, err := t.p.AssetPath(key)
	if err != nil {
		return nil, err
	}
	if err := t.writeDocument(abs, key, data, next); err != nil {
		return nil, err
	}
	out["changed"], out["sha"], out["diff"] = true, sha(next), lineDiff(data, next)
	return out, nil
}

// newProblems returns the problems of after that before did not have.
func newProblems(before, after []string) []string {
	had := map[string]int{}
	for _, p := range before {
		had[p]++
	}
	var added []string
	for _, p := range after {
		if had[p] > 0 {
			had[p]--
			continue
		}
		added = append(added, p)
	}
	return added
}

// writeFileAtomic writes data to a temporary file next to path and renames
// it over path, so a reader (or a crash) sees the old or the new text, never
// half of it. The mode of an existing file is kept.
func writeFileAtomic(path string, data []byte) error {
	mode := os.FileMode(0o644)
	if fi, err := os.Stat(path); err == nil {
		mode = fi.Mode().Perm()
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	tmp := f.Name()
	_, err = f.Write(data)
	if err == nil {
		err = f.Chmod(mode)
	}
	if err == nil {
		err = f.Sync()
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(tmp, path)
	}
	if err != nil {
		os.Remove(tmp)
	}
	return err
}

// lineDiff describes what changed between a and b as the lines between
// their common first and last lines: "@@ line N @@", then "- old" and "+ new"
// lines. It is a summary for the agent, not a patch.
func lineDiff(a, b []byte) string {
	x, y := strings.Split(string(a), "\n"), strings.Split(string(b), "\n")
	pre := 0
	for pre < len(x) && pre < len(y) && x[pre] == y[pre] {
		pre++
	}
	suf := 0
	for suf < len(x)-pre && suf < len(y)-pre && x[len(x)-1-suf] == y[len(y)-1-suf] {
		suf++
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "@@ line %d @@\n", pre+1)
	const max = 40
	emit := func(sign string, lines []string) {
		for i, l := range lines {
			if i == max {
				fmt.Fprintf(&sb, "%s ... %d more lines\n", sign, len(lines)-max)
				return
			}
			fmt.Fprintf(&sb, "%s %s\n", sign, strings.TrimRight(l, "\r"))
		}
	}
	emit("-", x[pre:len(x)-suf])
	emit("+", y[pre:len(y)-suf])
	return sb.String()
}

// fieldPath converts the steps of a "field" argument (strings for object
// members, whole numbers for array elements) to a jsonedit path.
func fieldPath(steps []any) ([]any, error) {
	out := make([]any, len(steps))
	for i, s := range steps {
		switch v := s.(type) {
		case string:
			if v == "" {
				return nil, fmt.Errorf("field step %d is empty", i)
			}
			out[i] = v
		case float64:
			if v < 0 || v != float64(int(v)) {
				return nil, fmt.Errorf("field step %d: %v is not an array index", i, v)
			}
			out[i] = int(v)
		default:
			return nil, fmt.Errorf("field step %d: want a member name (string) or an array index (integer), got %T", i, s)
		}
	}
	return out, nil
}

// getIn returns the value at path in v (decoded JSON).
func getIn(v any, path []any) (any, bool) {
	for _, step := range path {
		switch s := step.(type) {
		case string:
			m, ok := v.(map[string]any)
			if !ok {
				return nil, false
			}
			if v, ok = m[s]; !ok {
				return nil, false
			}
		case int:
			a, ok := v.([]any)
			if !ok || s >= len(a) {
				return nil, false
			}
			v = a[s]
		}
	}
	return v, true
}

// setIn returns v with the value at path replaced by val (an array
// element past the end is appended, a missing member added); v is changed in
// place where it is a container.
func setIn(v any, path []any, val any) (any, error) {
	if len(path) == 0 {
		return val, nil
	}
	switch s := path[0].(type) {
	case string:
		m, ok := v.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("cannot set member %q: not an object", s)
		}
		sub, err := setIn(m[s], path[1:], val)
		if err != nil {
			return nil, err
		}
		m[s] = sub
		return m, nil
	case int:
		a, ok := v.([]any)
		if !ok {
			return nil, fmt.Errorf("cannot set index %d: not an array", s)
		}
		switch {
		case s < len(a):
			sub, err := setIn(a[s], path[1:], val)
			if err != nil {
				return nil, err
			}
			a[s] = sub
		case s == len(a) && len(path) == 1:
			a = append(a, val)
		default:
			return nil, fmt.Errorf("index %d is out of range: the array has %d elements", s, len(a))
		}
		return a, nil
	}
	return nil, errors.New("bad path")
}

// marshalJSON encodes v as compact JSON without HTML escaping.
func marshalJSON(v any) ([]byte, error) {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimRight(b.Bytes(), "\n"), nil
}
