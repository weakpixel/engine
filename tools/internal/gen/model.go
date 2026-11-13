package gen

import (
	"go/token"
	"go/types"
	"strings"

	"golang.org/x/tools/go/packages"

	"github.com/weakpixel/engine/god"
	"github.com/weakpixel/engine/internal/marker"
)

// diag is a generator error anchored at a source position.
type diag = marker.Diag

// Embed is a pointer embed on a component's field path. Generated code
// allocates it when nil before touching promoted fields.
type Embed struct {
	Path []string
	Type *types.Named
}

// Component is one component field of an object, after flattening embeds.
type Component struct {
	Path      []string // field chain from the object, e.g. [Common Transform]
	Name      string   // leaf field name
	Type      *types.Named
	Optional  bool
	ByValue   bool
	PtrEmbeds []Embed  // pointer embeds on Path, outermost first
	Doc       string   // doc comment of the object field
	Via       []string // names of the embedded objects the component comes through, outermost first
	Pos       token.Pos
}

// Object is a +game:object struct with its flattened components in
// declaration order (embeds expanded in place).
type Object struct {
	Name       string
	Meta       god.SchemaMeta
	Pkg        *packages.Package
	Pos        token.Pos
	Doc        string
	Components []Component
}

// GoType is the object's Go struct, "importpath.Type".
func (o *Object) GoType() string { return o.Pkg.PkgPath + "." + o.Name }

// Package is the model of one generation target.
type Package struct {
	Pkg        *packages.Package
	Objects    []*Object
	Registered []*types.Named // +game:component types declared in this package

	// Components holds the schema of every component type used by the package
	// (registered or used by an object, including foreign ones), keyed by
	// types.TypeString(t, nil).
	Components map[string]*ComponentInfo
}

// ComponentInfo is the identity and JSON layout of a component type.
type ComponentInfo struct {
	Schema *god.TypeSchema
	Pos    token.Pos
	Type   *types.Named

	Group, Name string // identity, see componentIdentity
	// ExplicitGroup is false when Group fell back to the import path; such a
	// component may not be used by an object (checkComponentGroup).
	ExplicitGroup bool
	Doc           string // doc comment of the type
	GoType        string // "importpath.Type"
}

// Key returns the component key "group:name".
func (c *ComponentInfo) Key() string { return god.ComponentKey(c.Group, c.Name) }

func (c Component) PathString() string {
	return strings.Join(c.Path, ".")
}

func (e Embed) PathString() string {
	return strings.Join(e.Path, ".")
}
