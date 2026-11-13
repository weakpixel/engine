// Package jsonedit makes minimal text edits to a JSON document: it finds the
// value at a path and replaces, inserts or deletes it while every other byte
// of the document stays as it was, including indentation, key order and
// line ends. It is the Go counterpart of the VSCode extension's jsonc-parser
// "modify" (src/docEdits.ts), so a file edited by a tool and by the editor
// shows the same small diff.
//
// A path is a list of steps: a string selects the member of an object, an int
// the element of an array. The document must be valid JSON without comments.
// Values passed in are JSON text; they are laid out in the document's style:
// in one line (`{"a": 1, "b": 2}`) where the value they replace or the
// container they enter is on one line, else spread over lines with the
// document's own indentation. The package imports the standard library only.
package jsonedit

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// ErrNotFound is returned (wrapped) when a path leads to nothing.
var ErrNotFound = errors.New("jsonedit: no value at path")

// Span is the byte range [Start, End) of a value in a document.
type Span struct{ Start, End int }

// Find returns the span of the value at path. The empty path is the whole
// document value.
func Find(doc []byte, path ...any) (Span, error) {
	root, err := parse(doc)
	if err != nil {
		return Span{}, err
	}
	n, err := root.at(path)
	if err != nil {
		return Span{}, err
	}
	return Span{n.start, n.end}, nil
}

// Replace returns doc with the value at path replaced by value. The path must
// exist.
func Replace(doc, value []byte, path ...any) ([]byte, error) {
	root, err := parse(doc)
	if err != nil {
		return nil, err
	}
	if err := validValue(value); err != nil {
		return nil, err
	}
	n, err := root.at(path)
	if err != nil {
		return nil, err
	}
	l := newLayout(doc)
	multi := bytes.IndexByte(doc[n.start:n.end], '\n') >= 0
	text := l.format(value, multi, l.lineIndent(n.start))
	return splice(doc, n.start, n.end, text), nil
}

// Set puts value at path: it replaces the value when the path exists, else
// it adds the missing object member (and the missing objects above it, when
// the path is deeper) at the end of the nearest existing object, or appends
// to an array when the last step is an index equal to the array's length.
func Set(doc, value []byte, path ...any) ([]byte, error) {
	root, err := parse(doc)
	if err != nil {
		return nil, err
	}
	if err := validValue(value); err != nil {
		return nil, err
	}
	n := root
	for i, step := range path {
		next, err := n.child(step)
		if err == nil {
			n = next
			continue
		}
		if !errors.Is(err, ErrNotFound) {
			return nil, err
		}
		rest := path[i:]
		switch s := rest[0].(type) {
		case string:
			return insertMember(doc, root, n, path[:i], s, nest(rest[1:], value))
		case int:
			if len(rest) == 1 && n.kind == '[' && s == len(n.elems) {
				return insertElem(doc, root, n, path[:i], value)
			}
		}
		return nil, fmt.Errorf("%w: %s", ErrNotFound, describe(path))
	}
	return Replace(doc, value, path...)
}

// Insert adds value as element index of the array at path; index -1 or the
// array's length appends.
func Insert(doc, value []byte, index int, path ...any) ([]byte, error) {
	root, err := parse(doc)
	if err != nil {
		return nil, err
	}
	if err := validValue(value); err != nil {
		return nil, err
	}
	arr, err := root.at(path)
	if err != nil {
		return nil, err
	}
	if arr.kind != '[' {
		return nil, fmt.Errorf("jsonedit: %s is not an array", describe(path))
	}
	if index < 0 || index == len(arr.elems) {
		return insertElem(doc, root, arr, path, value)
	}
	if index > len(arr.elems) {
		return nil, fmt.Errorf("jsonedit: index %d is out of range: %s has %d elements", index, describe(path), len(arr.elems))
	}
	// Before element index: its text, a comma and the whitespace up to the
	// element that was there.
	l := newLayout(doc)
	next := arr.elems[index]
	multi := bytes.IndexByte(doc[arr.start:next.start], '\n') >= 0
	item := l.format(value, multi, l.lineIndent(next.start))
	sep := ", "
	if multi {
		sep = "," + l.eol + l.lineIndent(next.start)
	}
	return splice(doc, next.start, next.start, append(item, sep...)), nil
}

// Delete removes the object member or array element at path, with its comma,
// and returns doc. The path must exist and not be empty.
func Delete(doc []byte, path ...any) ([]byte, error) {
	if len(path) == 0 {
		return nil, errors.New("jsonedit: cannot delete the document")
	}
	root, err := parse(doc)
	if err != nil {
		return nil, err
	}
	parent, err := root.at(path[:len(path)-1])
	if err != nil {
		return nil, err
	}
	last := path[len(path)-1]
	var items []span2 // (start of the item, end of its value)
	idx := -1
	switch parent.kind {
	case '{':
		key, ok := last.(string)
		if !ok {
			return nil, fmt.Errorf("jsonedit: %s is an object, not an array", describe(path[:len(path)-1]))
		}
		for i, m := range parent.members {
			items = append(items, span2{m.keyStart, m.val.end})
			if m.key == key {
				idx = i
			}
		}
	case '[':
		i, ok := last.(int)
		if !ok {
			return nil, fmt.Errorf("jsonedit: %s is an array, not an object", describe(path[:len(path)-1]))
		}
		for j, e := range parent.elems {
			items = append(items, span2{e.start, e.end})
			if j == i {
				idx = j
			}
		}
	default:
		return nil, fmt.Errorf("%w: %s", ErrNotFound, describe(path))
	}
	if idx < 0 {
		return nil, fmt.Errorf("%w: %s", ErrNotFound, describe(path))
	}
	switch {
	case len(items) == 1:
		// Only item: leave the empty container. Its inside is cleared.
		return splice(doc, parent.start+1, parent.end-1, nil), nil
	case idx < len(items)-1:
		// Up to the next item: drops this item, its comma and the line break.
		return splice(doc, items[idx].start, items[idx+1].start, nil), nil
	}
	// The last of several: drops the comma in front of it instead.
	return splice(doc, items[idx-1].end, items[idx].end, nil), nil
}

type span2 struct{ start, end int }

// insertMember adds "key": value as the last member of object n.
func insertMember(doc []byte, root, n *node, at []any, key string, value []byte) ([]byte, error) {
	if n.kind != '{' {
		return nil, fmt.Errorf("jsonedit: %s is not an object", describe(at))
	}
	l := newLayout(doc)
	kb, _ := marshal(key)
	return l.insertItem(doc, root, n, at, append(append(kb, ':', ' '), l.formatInto(value, n, at, root)...)), nil
}

// insertElem adds value as the last element of array n.
func insertElem(doc []byte, root, n *node, at []any, value []byte) ([]byte, error) {
	l := newLayout(doc)
	return l.insertItem(doc, root, n, at, l.formatInto(value, n, at, root)), nil
}

// formatInto lays value out for an item of container n: spread over lines if
// the container's items are.
func (l layout) formatInto(value []byte, n *node, at []any, root *node) []byte {
	return l.format(value, l.itemsOnLines(n, at, root), l.itemIndent(n))
}

// insertItem puts item (already laid out) after the last item of container n.
func (l layout) insertItem(doc []byte, root, n *node, at []any, item []byte) []byte {
	lines := l.itemsOnLines(n, at, root)
	count := len(n.elems) + len(n.members)
	switch {
	case count == 0 && !lines:
		return splice(doc, n.start+1, n.end-1, item)
	case count == 0:
		in := l.lineIndent(n.start)
		text := append([]byte(l.eol+in+l.unit), item...)
		text = append(text, l.eol+in...)
		return splice(doc, n.start+1, n.end-1, text)
	}
	prevEnd := n.lastEnd()
	if !lines {
		return splice(doc, prevEnd, prevEnd, append([]byte(", "), item...))
	}
	return splice(doc, prevEnd, prevEnd, append([]byte(","+l.eol+l.itemIndent(n)), item...))
}

// itemsOnLines reports whether the items of container n are on lines of
// their own: a non-empty container whose first item starts after a line
// break, else an empty one inside a container that spans several lines (or
// the document itself, when it has several lines).
func (l layout) itemsOnLines(n *node, at []any, root *node) bool {
	if first, ok := n.firstStart(); ok {
		return bytes.IndexByte(l.doc[n.start:first], '\n') >= 0
	}
	if len(at) == 0 {
		return bytes.IndexByte(l.doc, '\n') >= 0
	}
	parent, err := root.at(at[:len(at)-1])
	if err != nil {
		return false
	}
	return bytes.IndexByte(l.doc[parent.start:parent.end], '\n') >= 0
}

// itemIndent is the indentation of the items of container n.
func (l layout) itemIndent(n *node) string {
	if first, ok := n.firstStart(); ok {
		if bytes.IndexByte(l.doc[n.start:first], '\n') >= 0 {
			return l.lineIndent(first)
		}
	}
	return l.lineIndent(n.start) + l.unit
}

// nest wraps value in objects for the keys of path.
func nest(path []any, value []byte) []byte {
	for i := len(path) - 1; i >= 0; i-- {
		key, _ := path[i].(string)
		kb, _ := marshal(key)
		value = append(append(append([]byte{'{'}, kb...), ':'), append(value, '}')...)
	}
	return value
}

func marshal(s string) ([]byte, error) {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	err := enc.Encode(s)
	return bytes.TrimRight(b.Bytes(), "\n"), err
}

func validValue(value []byte) error {
	if !json.Valid(value) {
		return errors.New("jsonedit: the value is not valid JSON")
	}
	return nil
}

func splice(doc []byte, start, end int, text []byte) []byte {
	out := make([]byte, 0, len(doc)-(end-start)+len(text))
	out = append(out, doc[:start]...)
	out = append(out, text...)
	return append(out, doc[end:]...)
}

// describe renders a path for messages: a.b[2].c.
func describe(path []any) string {
	var b strings.Builder
	for _, step := range path {
		switch s := step.(type) {
		case string:
			if b.Len() > 0 {
				b.WriteByte('.')
			}
			b.WriteString(s)
		case int:
			fmt.Fprintf(&b, "[%d]", s)
		}
	}
	if b.Len() == 0 {
		return "(document)"
	}
	return b.String()
}
