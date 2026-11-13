package diag

import "github.com/weakpixel/engine/tools/internal/gen/testdata/diag/nogroup"

// rule: a component used by an object needs an explicit group.
// +game:object
type UsesUngrouped struct {
	// +game:component
	V *nogroup.Plain
}

// rule: component keys are unique within a package.
// +game:component:name=Clash
type ClashA struct{ N int }

// +game:component:name=Clash
type ClashB struct{ N int }

// rule: group and name are identifiers.
// +game:component:group=a-b,name=1x
type BadIdentity struct{ N int }

// rule: group and name belong on the type, not on a field.
// +game:object
type FieldArgs struct {
	// +game:component:name=X
	V *V1
}
