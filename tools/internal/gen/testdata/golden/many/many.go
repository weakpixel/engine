// +game:group:name=many
//
// Package many covers an object with more than 16 required components:
// the untyped world.Query() path with IDs.
package many

// +game:object
type Big struct {
	// +game:component
	C1 *C1
	// +game:component
	C2 *C2
	// +game:component
	C3 *C3
	// +game:component
	C4 *C4
	// +game:component
	C5 *C5
	// +game:component
	C6 *C6
	// +game:component
	C7 *C7
	// +game:component
	C8 *C8
	// +game:component
	C9 *C9
	// +game:component
	C10 *C10
	// +game:component
	C11 *C11
	// +game:component
	C12 *C12
	// +game:component
	C13 *C13
	// +game:component
	C14 *C14
	// +game:component
	C15 *C15
	// +game:component
	C16 *C16
	// +game:component
	C17 *C17
}

type C1 struct{ V int }
type C2 struct{ V int }
type C3 struct{ V int }
type C4 struct{ V int }
type C5 struct{ V int }
type C6 struct{ V int }
type C7 struct{ V int }
type C8 struct{ V int }
type C9 struct{ V int }
type C10 struct{ V int }
type C11 struct{ V int }
type C12 struct{ V int }
type C13 struct{ V int }
type C14 struct{ V int }
type C15 struct{ V int }
type C16 struct{ V int }
type C17 struct{ V int }
