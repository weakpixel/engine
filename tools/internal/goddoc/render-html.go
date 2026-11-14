package goddoc

import (
	_ "embed"
	"html/template"
	"io"
	"strings"
	"unicode"
)

//go:embed page.html.tpl
var pageTemplate string

var page = template.Must(template.New("page").Parse(pageTemplate))

// htmlPage is what page.html.tpl renders. The template does no logic of its
// own beyond looping, so the content matches the Markdown output.
type htmlPage struct {
	Title      string
	Groups     []htmlGroup
	Components []htmlComponent
}

type htmlGroup struct {
	Name    string
	Objects []htmlObject
}

type htmlObject struct {
	ID, Class, Key string
	Doc            []string // paragraphs
	Components     []htmlObjectComponent
}

type htmlObjectComponent struct {
	Key, Field, CompID string
	Optional           bool
	Readonly           bool
	Embedded           string // "embedded (Common)" or ""
	Doc                string
}

type htmlComponent struct {
	ID, Key, GoType, Type string
	Doc                   []string
	Rows                  []fieldRow
	Users                 []htmlUsage
}

type htmlUsage struct {
	ID, Key  string
	Embedded string
}

// paragraphs splits doc text at blank lines.
func paragraphs(text string) []string {
	var out []string
	for p := range strings.SplitSeq(text, "\n\n") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func objectID(o *object) string { return "obj-" + o.Group + "-" + o.Class }

// componentID is the anchor of a component: its key with every character
// but letters, digits, '-' and '_' replaced by '-'.
func componentID(c *component) string {
	return "comp-" + strings.Map(func(r rune) rune {
		if r == '-' || r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r) {
			return r
		}
		return '-'
	}, c.Key)
}

// renderHTML writes doc as one self-contained HTML page.
func renderHTML(w io.Writer, doc *document) error {
	data := htmlPage{Title: doc.Title}
	for _, g := range doc.Groups {
		hg := htmlGroup{Name: g.Name}
		for _, o := range g.Objects {
			ho := htmlObject{ID: objectID(o), Class: o.Class, Key: o.key(), Doc: paragraphs(o.Doc)}
			for _, c := range o.Components {
				ho.Components = append(ho.Components, htmlObjectComponent{
					Key: c.Comp.Key, Field: c.Field, CompID: componentID(c.Comp), Optional: c.Optional, Readonly: c.Readonly,
					Embedded: embeddedLabel(c.Via), Doc: c.Doc,
				})
			}
			hg.Objects = append(hg.Objects, ho)
		}
		data.Groups = append(data.Groups, hg)
	}
	for _, c := range doc.Components {
		hc := htmlComponent{ID: componentID(c), Key: c.Key, GoType: c.GoType, Type: c.Type, Doc: paragraphs(c.Doc), Rows: c.Rows}
		for _, u := range c.Users {
			hc.Users = append(hc.Users, htmlUsage{ID: objectID(u.Object), Key: u.Object.key(), Embedded: embeddedLabel(u.Via)})
		}
		data.Components = append(data.Components, hc)
	}
	return page.Execute(w, data)
}
