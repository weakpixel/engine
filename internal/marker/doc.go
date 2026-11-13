// Package marker parses controller-tools style markers, `+name` comments on Go
// declarations, into user-defined structs. It depends on the standard library
// only and knows nothing about the program that uses it, so it works on any
// Go-syntax source that go/parser reads (Go, Kage shaders, ...).
//
// # Grammar
//
//	// +game:object                       a flag
//	// +game:object:group=ui,class=Label  arguments: name=value, comma separated
//	// +game:editor:min=0,max=1.5,unit=s  numbers; absent pointer fields stay nil
//	// +game:asset:kind="sprite, big"     quoted (Go or raw) strings keep , and ;
//	// +t:args:l=a;b;c   or   l={a, b, c} lists of scalars
//	// +game:default=3                    path marker, bare form
//	// +game:default:Offset.X=2,Y=4       path marker, path form (repeatable)
//
// A marker is a whole comment line starting with `+`. Names are colon-separated
// segments of letters, digits, '_' and '-'; the longest registered name that
// ends at a boundary (end, space, ':' or '=') wins. Without arguments the rest
// of the line is ignored, so a flag may carry a note. Marker arguments are
// parsed with the field types of the definition struct: integers (decimal, 0x,
// 0b, 0o), floats, bools and strings, slices of those, and pointers to scalars,
// which are optional and nil when absent.
//
// # Use
//
// Define markers on a [Registry] with a struct per marker whose exported
// fields carry `marker:"name[,optional]"` tags ([Registry.Define]), or as
// free-form path markers ([Registry.DefinePaths]) whose values stay raw text
// ([PathArgs], [PathSettings]) for the caller to evaluate. [Registry.Parse]
// reads one marker text; [Registry.Collect] reads a whole comment group of a
// declaration and returns a [Set] plus [Diag]s anchored at the comments. Use
// [WithPrefix] to claim a namespace and ignore everybody else's `+` comments.
//
// # Repeating and checking markers
//
// A struct marker is single by default: a second line is a duplicate.
// [Registry.DefineRepeatable] allows several lines, read with [Set.Lines] or
// [Values]. Options on Define and DefineRepeatable add checks that the
// argument types cannot express:
//
//	r.DefineRepeatable("shader:param", marker.DescribesType, param{},
//		marker.Validate(validateParam),           // one marker: func(param) error
//		marker.WithLinesCheck(checkSlotsUnique),  // all lines: func([]Marker) []Diag
//	)
//
// [Validate] runs after the arguments parsed; its error becomes a Diag at the
// comment and the marker is not collected. [WithLinesCheck] sees the valid
// lines in source order and may point at a second position with Diag.Related.
//
// # Markers that belong to nothing
//
// Collect reads the groups the caller hands it. [Registry.Stray] lists the
// markers of a file that no collected group holds, so a marker in a function
// body or above the wrong declaration is reported instead of ignored.
package marker
