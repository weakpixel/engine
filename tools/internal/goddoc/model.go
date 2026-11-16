package goddoc

import (
	"bytes"
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"reflect"
	"slices"
	"strings"

	"github.com/weakpixel/engine/god"
)

// supportedVersion is the only "version" of a GOD file wpix doc reads.
const supportedVersion = 0

// generatorName is the "generator" of the merged GOD that --format god
// writes. Directory walks skip such files, so a merged GOD in the specs
// folder is not read back as an input.
const generatorName = "wpix doc"

// document is the merged documentation of all loaded definition files. Every
// renderer works from it, so every format shows the same content.
type document struct {
	Title      string
	Groups     []*group     // sorted by name
	Components []*component // sorted by key
}

// group is the set of objects sharing a group name.
type group struct {
	Name    string
	Objects []*object // sorted by class
}

// object is one Game Object.
type object struct {
	Group, Class string
	Doc          string
	Source       string // definition file it came from
	Components   []objectComponent

	def god.Schema // as read, for --format god
}

// objectComponent is a component as one object uses it.
type objectComponent struct {
	Field    string // Go field name in the object struct
	Optional bool
	Readonly bool
	Via      []string // embedded objects it is promoted through, outermost first
	Doc      string   // doc of the object field
	Comp     *component
}

// Embedded reports whether the component comes from an embedded object.
func (c objectComponent) Embedded() bool { return len(c.Via) > 0 }

// component is a component type, shared by every object that uses it.
type component struct {
	Group, Name string
	Key         string // "group:name", unique: used as title and anchor
	GoType      string
	Doc         string // type doc
	Type        string // one-line type, e.g. "object"
	Rows        []fieldRow
	Users       []usage // sorted by object; empty for an unused component
	Source      string  // definition file it came from first

	def god.ComponentSchema // as read, for --format god
}

// usage is one object using a component.
type usage struct {
	Object *object
	Via    []string
}

// fieldRow is one line of a component's field table.
type fieldRow struct {
	Name        string // dotted path for nested fields
	Type        string
	Required    bool
	Nullable    bool
	Runtime     bool // x-runtime: playback or cache state, not authored data
	Description string
	Values      []god.EnumValue // x-enum members of the field (or its items or map values)
	Default     string          // the editor's initial value, as JSON (an enum by constant name); "" when none
}

// godFile is a GOD file: one package's (wpix gen) or a merged one (--format god).
type godFile struct {
	Version    int                   `json:"version"`
	Generator  string                `json:"generator"`
	Package    string                `json:"package,omitempty"`
	Components []god.ComponentSchema `json:"components"`
	Objects    []god.Schema          `json:"objects"`
}

// key identifies the object, "group:class".
func (o *object) key() string { return o.Group + ":" + o.Class }

// loadDocument reads and merges the definition files. Components merge by
// group:name and objects by group:class: identical definitions (docs
// ignored) collapse into one, a different definition under the same key is
// an error naming both files. All problems (unreadable file, bad JSON,
// unsupported version, conflicts, unknown components) are reported together.
func loadDocument(title string, files []string) (*document, error) {
	var errs []error
	objects := map[string]*object{}
	comps := map[string]*component{}

	var loaded []godFile
	var sources []string
	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		var f godFile
		if err := json.Unmarshal(data, &f); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", file, err))
			continue
		}
		if f.Version != supportedVersion {
			errs = append(errs, fmt.Errorf("%s: unsupported version %d (want %d); regenerate with wpix gen", file, f.Version, supportedVersion))
			continue
		}
		loaded = append(loaded, f)
		sources = append(sources, file)
	}

	// Components first: objects of one file may use components of another.
	for i, f := range loaded {
		for _, cs := range f.Components {
			key := cs.Key()
			if prev := comps[key]; prev != nil {
				if !sameComponent(prev.def, cs) {
					errs = append(errs, fmt.Errorf("%s: component %s differs from the one in %s", sources[i], key, prev.Source))
				}
				continue
			}
			comps[key] = newComponent(cs, sources[i])
		}
	}
	for i, f := range loaded {
		for _, s := range f.Objects {
			o := &object{Group: s.Group, Class: s.Class, Doc: s.Doc, Source: sources[i], def: s}
			if prev := objects[o.key()]; prev != nil {
				if !sameObject(prev.def, s) {
					errs = append(errs, fmt.Errorf("%s: object %s differs from the one in %s", sources[i], o.key(), prev.Source))
				}
				continue
			}
			for _, c := range s.Components {
				comp := comps[c.Key()]
				if comp == nil {
					errs = append(errs, fmt.Errorf("%s: object %s uses component %s, which no input defines", sources[i], o.key(), c.Key()))
					continue
				}
				o.Components = append(o.Components, objectComponent{
					Field: c.Field, Optional: c.Optional, Readonly: c.Readonly, Via: c.EmbeddedVia, Doc: c.Doc, Comp: comp,
				})
			}
			objects[o.key()] = o
		}
	}
	if err := errors.Join(errs...); err != nil {
		return nil, err
	}
	return buildDocument(title, objects, comps), nil
}

// sameComponent compares two definitions of a component type, ignoring
// documentation.
func sameComponent(a, b god.ComponentSchema) bool {
	return a.GoType == b.GoType && reflect.DeepEqual(a.Schema.WithoutDescriptions(), b.Schema.WithoutDescriptions())
}

// sameObject compares two definitions of an object, ignoring documentation.
func sameObject(a, b god.Schema) bool {
	return a.GoType == b.GoType && slices.EqualFunc(a.Components, b.Components, func(x, y god.Component) bool {
		return x.Key() == y.Key() && x.Field == y.Field && x.Optional == y.Optional && x.Readonly == y.Readonly
	})
}

// newComponent builds the documentation of a component type.
func newComponent(cs god.ComponentSchema, source string) *component {
	c := &component{Group: cs.Group, Name: cs.Name, Key: cs.Key(), GoType: cs.GoType, Doc: cs.Doc, Source: source, def: cs}
	if cs.Schema != nil {
		c.Type = typeString(cs.Schema)
		c.Rows = fieldRows(cs.Schema, "")
	}
	return c
}

// buildDocument sorts and cross-links the loaded objects and components.
func buildDocument(title string, objects map[string]*object, comps map[string]*component) *document {
	doc := &document{Title: title}

	groups := map[string]*group{}
	for _, o := range objects {
		g := groups[o.Group]
		if g == nil {
			g = &group{Name: o.Group}
			groups[o.Group] = g
			doc.Groups = append(doc.Groups, g)
		}
		g.Objects = append(g.Objects, o)
	}
	slices.SortFunc(doc.Groups, func(a, b *group) int { return cmp.Compare(a.Name, b.Name) })
	for _, g := range doc.Groups {
		slices.SortFunc(g.Objects, func(a, b *object) int { return cmp.Compare(a.Class, b.Class) })
		for _, o := range g.Objects {
			for _, c := range o.Components {
				c.Comp.Users = append(c.Comp.Users, usage{Object: o, Via: c.Via})
			}
		}
	}

	for _, c := range comps {
		doc.Components = append(doc.Components, c)
	}
	slices.SortFunc(doc.Components, func(a, b *component) int { return cmp.Compare(a.Key, b.Key) })
	return doc
}

// merged returns the merged GOD of doc (--format god): every component and
// object once, sorted by key, with documentation.
func (doc *document) merged() godFile {
	f := godFile{Version: supportedVersion, Generator: generatorName, Components: []god.ComponentSchema{}, Objects: []god.Schema{}}
	for _, c := range doc.Components {
		f.Components = append(f.Components, c.def)
	}
	for _, g := range doc.Groups {
		for _, o := range g.Objects {
			f.Objects = append(f.Objects, o.def)
		}
	}
	return f
}

// typeString is the one-line type of a schema for the field tables.
func typeString(s *god.TypeSchema) string {
	switch {
	case s == nil || s.PreserveUnknownFields:
		return "any"
	case s.GoType != "":
		return s.GoType
	case s.Type == "string" && s.Format == "byte":
		return "bytes"
	case s.Type == "array":
		if s.MinItems != nil && s.MaxItems != nil && *s.MinItems == *s.MaxItems {
			return fmt.Sprintf("array[%d] of %s", *s.MaxItems, typeString(s.Items))
		}
		return "array of " + typeString(s.Items)
	case s.Type == "object" && s.AdditionalProperties != nil:
		return "map of " + typeString(s.AdditionalProperties)
	case s.Format == god.FormatEntity:
		return "reference to another object" // an object id in JSON, an ecs.Entity at runtime
	case len(s.XEnum) > 0:
		names := make([]string, len(s.XEnum))
		for i, e := range s.XEnum {
			names[i] = e.Name
		}
		return "enum (" + strings.Join(names, ", ") + ")" // an integer in JSON
	case s.Format == "asset" && s.XAsset != "":
		return "asset (" + s.XAsset + ")" // a string key in JSON, a handle at runtime
	case s.Format != "":
		return s.Type + " (" + s.Format + ")"
	}
	return s.Type
}

// enumOf returns the x-enum members of a property, its array items or its
// map values, or nil.
func enumOf(s *god.TypeSchema) []god.EnumValue {
	switch {
	case s == nil:
		return nil
	case len(s.XEnum) > 0:
		return s.XEnum
	case s.Items != nil:
		return enumOf(s.Items)
	}
	return enumOf(s.AdditionalProperties)
}

// fieldRows flattens the properties of an object schema into table rows;
// nested objects follow their parent with dotted names. Properties are sorted
// by name (the JSON does not keep struct order).
func fieldRows(s *god.TypeSchema, prefix string) []fieldRow {
	if s == nil || len(s.Properties) == 0 {
		return nil
	}
	names := make([]string, 0, len(s.Properties))
	for name := range s.Properties {
		names = append(names, name)
	}
	slices.Sort(names)

	var rows []fieldRow
	for _, name := range names {
		p := s.Properties[name]
		rows = append(rows, fieldRow{
			Name:        prefix + name,
			Type:        typeString(p),
			Required:    slices.Contains(s.Required, name),
			Nullable:    p.Nullable,
			Runtime:     p.XRuntime,
			Description: p.Description,
			Values:      enumOf(p),
			Default:     defaultString(p),
		})
		rows = append(rows, fieldRows(p, prefix+name+".")...)
	}
	return rows
}

// defaultString is the default of s as shown in the field tables: its JSON,
// or for an enum the name of the constant.
func defaultString(s *god.TypeSchema) string {
	if len(s.Default) == 0 {
		return ""
	}
	var n int64
	if json.Unmarshal(s.Default, &n) == nil {
		for _, e := range s.XEnum {
			if e.Value == n {
				return e.Name
			}
		}
	}
	var compact bytes.Buffer
	if json.Compact(&compact, s.Default) != nil {
		return string(s.Default)
	}
	return compact.String()
}
