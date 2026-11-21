package jsonedit

import (
	"bytes"
	"strings"
)

// layout is how a document is laid out: its line ends and indentation unit.
type layout struct {
	doc  []byte
	eol  string
	unit string
}

// newLayout reads the style of doc: CRLF or LF, and the leading whitespace
// of its first indented line as the indentation unit (two spaces if none).
func newLayout(doc []byte) layout {
	l := layout{doc: doc, eol: "\n", unit: "  "}
	if bytes.Contains(doc, []byte("\r\n")) {
		l.eol = "\r\n"
	}
	for i := 0; i < len(doc); i++ {
		if doc[i] != '\n' {
			continue
		}
		j := i + 1
		for j < len(doc) && (doc[j] == ' ' || doc[j] == '\t') {
			j++
		}
		if j > i+1 && j < len(doc) && doc[j] != '\n' && doc[j] != '\r' {
			l.unit = string(doc[i+1 : j])
			break
		}
	}
	return l
}

// lineIndent is the leading whitespace of the line holding offset.
func (l layout) lineIndent(offset int) string {
	start := bytes.LastIndexByte(l.doc[:offset], '\n') + 1
	end := start
	for end < len(l.doc) && (l.doc[end] == ' ' || l.doc[end] == '\t') {
		end++
	}
	return string(l.doc[start:end])
}

// format returns value in the document's style: on one line with a space
// after ':' and ',' (`{"a": 1, "b": [1, 2]}`), or, with multi, one item per
// line, each level indented by the unit from indent (the indentation of the
// line the value starts on).
func (l layout) format(value []byte, multi bool, indent string) []byte {
	root, err := parse(value)
	if err != nil {
		return value
	}
	var b strings.Builder
	l.emit(&b, value, root, multi, indent)
	return []byte(b.String())
}

func (l layout) emit(b *strings.Builder, src []byte, n *node, multi bool, indent string) {
	sep := func(inner string) {
		if multi {
			b.WriteString(l.eol + inner)
		}
	}
	switch n.kind {
	case '{':
		if len(n.members) == 0 {
			b.WriteString("{}")
			return
		}
		b.WriteByte('{')
		for i, m := range n.members {
			if i > 0 {
				b.WriteByte(',')
				if !multi {
					b.WriteByte(' ')
				}
			}
			sep(indent + l.unit)
			b.Write(src[m.keyStart:m.keyEnd])
			b.WriteString(": ")
			l.emit(b, src, m.val, multi, indent+l.unit)
		}
		sep(indent)
		b.WriteByte('}')
	case '[':
		if len(n.elems) == 0 {
			b.WriteString("[]")
			return
		}
		b.WriteByte('[')
		for i, e := range n.elems {
			if i > 0 {
				b.WriteByte(',')
				if !multi {
					b.WriteByte(' ')
				}
			}
			sep(indent + l.unit)
			l.emit(b, src, e, multi, indent+l.unit)
		}
		sep(indent)
		b.WriteByte(']')
	default:
		b.Write(src[n.start:n.end])
	}
}
