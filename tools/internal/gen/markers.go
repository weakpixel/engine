package gen

import (
	"github.com/weakpixel/engine/internal/marker"
)

const (
	markObject    = "game:object"
	markComponent = "game:component"
	markOptional  = "game:optional"
	markEmbed     = "game:embed"
	markGroup     = "game:group"
	markAsset     = "game:asset"
	markRuntime   = "game:runtime"
	markEnum      = "game:enum"
	markDefault   = "game:default"
	markEditor    = "game:editor"

	markerNamespace = "game:"
	markerPrefix    = "+" + markerNamespace
)

// objectArgs are the optional arguments of +game:object.
type objectArgs struct {
	Group string `marker:"group,optional"`
	Class string `marker:"class,optional"`
}

// componentArgs are the optional arguments of +game:component on a type:
// the component's identity. On a field the marker takes no arguments.
type componentArgs struct {
	Group string `marker:"group,optional"`
	Name  string `marker:"name,optional"`
}

// groupArgs are the arguments of the package marker +game:group.
type groupArgs struct {
	Name string `marker:"name"`
}

// assetArgs are the arguments of +game:asset: the asset kind ("sprite",
// "clip", ...), a free string that only the runtime's AssetResolver interprets.
type assetArgs struct {
	Kind string `marker:"kind"`
}

// registry holds the generator's markers.
var registry = newRegistry()

func newRegistry() *marker.Registry {
	r := marker.NewRegistry(marker.WithPrefix(markerNamespace))
	for _, d := range []struct {
		name   string
		target marker.TargetType
		args   any
	}{
		{markObject, marker.DescribesType, objectArgs{}},
		{markComponent, marker.DescribesType | marker.DescribesField, componentArgs{}},
		{markOptional, marker.DescribesField, struct{}{}},
		{markEmbed, marker.DescribesField, struct{}{}},
		{markGroup, marker.DescribesPackage, groupArgs{}},
		{markAsset, marker.DescribesField, assetArgs{}},
		{markRuntime, marker.DescribesField, struct{}{}},
		{markEnum, marker.DescribesType, struct{}{}},
		{markEditor, marker.DescribesField, editorArgs{}},
	} {
		if err := r.Define(d.name, d.target, d.args); err != nil {
			panic(err)
		}
	}
	if err := r.DefinePaths(markDefault, marker.DescribesField); err != nil {
		panic(err)
	}
	return r
}
