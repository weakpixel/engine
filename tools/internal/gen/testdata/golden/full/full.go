// +game:group:name=fullgrp
//
// Package full is the golden-test kitchen sink: optional and value
// components, nested value/pointer embeds, foreign components with an
// import-alias clash, a bare +game:component type, group/class overrides,
// JSON tag variants, a custom-MarshalJSON type, a recursive type, []byte, a
// fixed array and maps with supported and unsupported key types.
package full

import (
	ext "github.com/weakpixel/engine/tools/internal/gen/testdata/golden/full/ext"
	extB "github.com/weakpixel/engine/tools/internal/gen/testdata/golden/full/ext2dir"
)

// +game:component
type Unused struct {
	V int
}

type A struct{ V int }
type B struct{ V int }

// C has an explicit identity: it is "named:Gamma" in data files.
//
// +game:component:group=named,name=Gamma
type C struct{ V int }

// +game:object
type Base struct {
	// +game:component
	A *A
}

// +game:object
type Mid struct {
	// +game:embed
	Base

	// +game:component
	B *B
}

// +game:object
type Top struct {
	// +game:embed
	*Mid

	// +game:component
	C *C
}

// Tags exercises encoding/json tag variants.
type Tags struct {
	Hidden   string `json:"-"`
	Maybe    string `json:"maybe,omitempty"`
	AsString int64  `json:"as_string,string"`
	Renamed  string `json:"renamed_name"`
	Plain    string

	// Icon is an asset handle: a string key in JSON, runtime-only in Go.
	//
	// +game:asset:kind=sprite
	Icon Handle `json:"-"`

	// Elapsed is playback state, not authored data.
	//
	// +game:runtime
	Elapsed float64 `json:"-"`
}

// Handle stands in for an engine asset handle.
type Handle struct{ Index, Gen uint32 }

// Money has a custom JSON encoding. As a nested field (see Purse) its
// schema is x-go-type; a custom-JSON type is only detected for nested
// fields, not for a component's own top-level type.
type Money struct{ Cents int64 }

func (m *Money) MarshalJSON() ([]byte, error) { return []byte("0"), nil }

// Purse nests a custom-JSON field.
type Purse struct {
	Amount Money
}

// Node is self-referencing, so its schema is x-preserve-unknown-fields.
type Node struct {
	Value    int
	Children []*Node
}

type structKey struct{ A, B int }

// Misc exercises []byte, a fixed array, and maps with a supported
// (integer) and an unsupported (struct) key type.
type Misc struct {
	Blob    []byte
	Fixed   [3]int
	ByInt   map[int]string
	ByOther map[structKey]string
}

// +game:object
type Kit struct {
	// +game:component
	Tags *Tags

	// +game:component
	// +game:optional
	Purse *Purse

	// +game:component
	Node *Node

	// +game:component
	Misc Misc

	// +game:component
	Alpha *ext.Alpha

	// +game:component
	Beta *extB.Beta
}

// +game:object:group=customgrp,class=Widget
type CustomClassObj struct {
	// +game:component
	A *A
}
