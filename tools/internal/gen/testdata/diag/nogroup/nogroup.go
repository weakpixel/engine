// Package nogroup has no +game:group:name, so its types have no explicit
// group and cannot be used by an object.
package nogroup

type Plain struct{ X int }
