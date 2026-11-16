// Package diag exercises every validation rule in wpix gen with one
// deliberately invalid construct per rule. generate() must report every
// one of them, sorted and deduplicated, in a single call.
package diag

import (
	"github.com/weakpixel/engine/tools/internal/gen/testdata/diag/foreign"
)

type V1 struct{ N int }

// rule: a type cannot be both +game:object and +game:component.
//
// +game:object
// +game:component
type BothMarkers struct {
	// +game:component
	V *V1
}

// rule: +game:object and +game:component must be applied to a struct type.
//
// +game:object
type NotAStruct int

// rule: generic types are not supported.
//
// +game:object
type Generic[T any] struct {
	V T
}

// rule: a +game:component type must not contain nested +game:component
// or +game:embed fields.
//
// +game:component
type NestedMarkerComponent struct {
	// +game:component
	Inner *V1
}

// rule: component type does not resolve.
//
// +game:object
type UnresolvedComponent struct {
	// +game:component
	Bad *DoesNotExist
}

// rule: component type must be T or *T, not a nested pointer.
//
// +game:object
type NestedPointer struct {
	// +game:component
	V **V1
}

// rule: component type must be a named type.
//
// +game:object
type UnnamedComponent struct {
	// +game:component
	V *[]int
}

// rule: component type must not be an interface.
//
// +game:object
type InterfaceComponent struct {
	// +game:component
	V *Iface
}

type Iface interface{ M() }

// rule: a foreign +game:component field that is unexported cannot be
// accessed from this package.
//
// +game:object
type UsesForeignField struct {
	// +game:embed
	foreign.Base
}

// rule: a foreign type that is unexported (even behind an exported alias)
// cannot be named from this package.
//
// +game:object
type UsesForeignType struct {
	// +game:component
	V *foreign.VisibleAlias
}

// rule: +game:component and +game:embed are mutually exclusive.
//
// +game:object
type ComponentAndEmbed struct {
	// +game:component
	// +game:embed
	V *V1
}

// rule: +game:optional requires +game:component on the same field (bare).
//
// +game:object
type BareOptional struct {
	// +game:optional
	V *V1
}

type Embeddable struct {
	V *V1
}

// rule: +game:optional requires +game:component on the same field
// (combined with +game:embed).
//
// +game:object
type OptionalEmbed struct {
	// +game:embed
	// +game:optional
	Embeddable
}

// rule: +game:component requires a named field; embedded fields should
// instead use +game:embed.
//
// +game:object
type AnonymousComponent struct {
	// +game:component
	V1
}

// rule: +game:optional is not allowed on a value component.
//
// +game:object
type OptionalValue struct {
	// +game:component
	// +game:optional
	V V1
}

// rule: +game:embed field must be a +game:object struct (or a pointer to
// one).
//
// +game:object
type EmbedNonStruct struct {
	// +game:embed
	Name string
}

// Box is a generic type, never marked +game:object.
type Box[T any] struct {
	V T
}

// rule: embedding a generic object is not supported.
//
// +game:object
type EmbedGenericInstance struct {
	// +game:embed
	Box[int]
}

// NotStructNamed is a named type over a non-struct.
type NotStructNamed int

// rule: embedded type must be a struct declared with +game:object (here:
// not a struct at all).
//
// +game:object
type EmbedNotStruct struct {
	// +game:embed
	NotStructNamed
}

// PlainStruct is a real struct, but never marked +game:object.
type PlainStruct struct {
	V *V1
}

// rule: embedded type is not marked +game:object.
//
// +game:object
type EmbedUnmarked struct {
	// +game:embed
	PlainStruct
}

// rule: embed cycle. (Pointer embeds: a value-embed cycle here would be an
// illegal recursive Go type before the generator even runs.)
//
// +game:object
type CycleA struct {
	// +game:embed
	*CycleB
}

// +game:object
type CycleB struct {
	// +game:embed
	*CycleA
}

// rule: a component type may be used only once per object (after
// flattening), by type.
//
// +game:object
type DuplicateComponentType struct {
	// +game:component
	A *V1
	// +game:component
	B *V1
}

type V2 struct{ N int }

// +game:object
type HasFooA struct {
	// +game:component
	Foo *V1
}

// +game:object
type HasFooB struct {
	// +game:component
	Foo *V2
}

// rule: a component type may be used only once per object (after
// flattening), by field name.
//
// +game:object
type NameCollision struct {
	// +game:embed
	HasFooA
	// +game:embed
	HasFooB
}

// rule: object key (group:class) must be unique among a package's objects.
//
// +game:object:class=DupKey
type DupKeyA struct {
	// +game:component
	V *V1
}

// +game:object:class=DupKey
type DupKeyB struct {
	// +game:component
	V *V1
}

// Handle stands in for an asset handle.
type Handle struct{ Index, Gen uint32 }

// rule: asset and runtime marker fields must be exported and tagged
// json:"-"; an asset field also needs a kind and a struct handle type.
//
// +game:component
type BadAssetFields struct {
	// +game:asset
	NoKind Handle `json:"-"`
	// +game:asset:kind=sprite
	NoTag Handle
	// +game:asset:kind=sprite
	Pointer *Handle `json:"-"`
	// +game:asset:kind=sprite
	NotStruct int `json:"-"`
	// +game:asset:kind=sprite
	unexported Handle `json:"-"`
	// +game:asset:kind=sprite
	// +game:runtime
	Both Handle `json:"-"`
	// +game:asset:kind=sprite
	Handle `json:"-"`
	// +game:runtime
	RuntimeNoTag float64
}

// rule: the markers are only valid on direct fields of a component struct.
//
// +game:component
type NestedAsset struct {
	Inner struct {
		// +game:asset:kind=sprite
		H Handle `json:"-"`
	}
}

// +game:object
type ObjectWithAsset struct {
	// +game:asset:kind=sprite
	H Handle `json:"-"`
}

// NotAComponent is not used as a component anywhere.
type NotAComponent struct {
	// +game:runtime
	T float64 `json:"-"`
}
