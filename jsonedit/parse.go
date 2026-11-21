package jsonedit

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
)

// node is a JSON value with its place in the text.
type node struct {
	kind       byte // '{', '[' or 'v' (string, number, true, false, null)
	start, end int
	members    []member // objects
	elems      []*node  // arrays
}

// member is one "key": value of an object.
type member struct {
	key      string // decoded
	keyStart int
	keyEnd   int
	val      *node
}

// parse reads doc, which must be one valid JSON value (and nothing else but
// whitespace), into a tree of spans.
func parse(doc []byte) (*node, error) {
	if !json.Valid(doc) {
		var v any
		err := json.Unmarshal(doc, &v)
		return nil, fmt.Errorf("jsonedit: invalid JSON: %v", err)
	}
	p := &parser{b: doc}
	p.skip()
	n := p.value()
	return n, nil
}

type parser struct {
	b []byte
	i int
}

func (p *parser) skip() {
	for p.i < len(p.b) && (p.b[p.i] == ' ' || p.b[p.i] == '\t' || p.b[p.i] == '\n' || p.b[p.i] == '\r') {
		p.i++
	}
}

// value parses the value at p.i; the input is known to be valid.
func (p *parser) value() *node {
	n := &node{start: p.i}
	switch p.b[p.i] {
	case '{':
		n.kind = '{'
		p.i++
		for {
			p.skip()
			if p.b[p.i] == '}' {
				p.i++
				break
			}
			if p.b[p.i] == ',' {
				p.i++
				continue
			}
			ks := p.i
			p.str()
			var key string
			_ = json.Unmarshal(p.b[ks:p.i], &key)
			ke := p.i
			p.skip()
			p.i++ // ':'
			p.skip()
			n.members = append(n.members, member{key, ks, ke, p.value()})
		}
	case '[':
		n.kind = '['
		p.i++
		for {
			p.skip()
			if p.b[p.i] == ']' {
				p.i++
				break
			}
			if p.b[p.i] == ',' {
				p.i++
				continue
			}
			n.elems = append(n.elems, p.value())
		}
	case '"':
		n.kind = 'v'
		p.str()
	default:
		n.kind = 'v'
		for p.i < len(p.b) && !bytes.ContainsRune([]byte(" \t\r\n,]}"), rune(p.b[p.i])) {
			p.i++
		}
	}
	n.end = p.i
	return n
}

// str skips the string literal at p.i.
func (p *parser) str() {
	p.i++
	for p.b[p.i] != '"' {
		if p.b[p.i] == '\\' {
			p.i++
		}
		p.i++
	}
	p.i++
}

// at returns the node at path below n.
func (n *node) at(path []any) (*node, error) {
	cur := n
	for i, step := range path {
		next, err := cur.child(step)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				return nil, fmt.Errorf("%w: %s", ErrNotFound, describe(path[:i+1]))
			}
			return nil, err
		}
		cur = next
	}
	return cur, nil
}

// child returns the member or element selected by step. With duplicate keys
// the last one wins, as in encoding/json.
func (n *node) child(step any) (*node, error) {
	switch s := step.(type) {
	case string:
		if n.kind != '{' {
			return nil, fmt.Errorf("jsonedit: cannot select %q: not an object", s)
		}
		var found *node
		for _, m := range n.members {
			if m.key == s {
				found = m.val
			}
		}
		if found == nil {
			return nil, ErrNotFound
		}
		return found, nil
	case int:
		if n.kind != '[' {
			return nil, fmt.Errorf("jsonedit: cannot select index %d: not an array", s)
		}
		if s < 0 || s >= len(n.elems) {
			return nil, ErrNotFound
		}
		return n.elems[s], nil
	}
	return nil, fmt.Errorf("jsonedit: path steps are strings or ints, not %T", step)
}

// firstStart is where the first item of an object or array starts.
func (n *node) firstStart() (int, bool) {
	switch {
	case len(n.members) > 0:
		return n.members[0].keyStart, true
	case len(n.elems) > 0:
		return n.elems[0].start, true
	}
	return 0, false
}

// lastEnd is where the last item of a non-empty object or array ends.
func (n *node) lastEnd() int {
	if len(n.members) > 0 {
		return n.members[len(n.members)-1].val.end
	}
	return n.elems[len(n.elems)-1].end
}
