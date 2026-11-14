package input

import _ "embed"

//go:embed schema/bindings.schema.json
var bindingsSchema []byte

// BindingsSchema returns schema/bindings.schema.json: the JSON shape of a
// bindings file in the GOD TypeSchema dialect. Tools such as
// `wpix schema` convert it for editors. The slice is a copy.
func BindingsSchema() []byte { return append([]byte(nil), bindingsSchema...) }
