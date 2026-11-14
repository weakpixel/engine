package marker

import "fmt"

// TargetType is a bit set of the declarations a marker may describe.
type TargetType int

const (
	DescribesPackage TargetType = 1 << iota
	DescribesType
	DescribesField

	allTargets = DescribesPackage | DescribesType | DescribesField
)

// single reports whether t names exactly one target.
func (t TargetType) single() bool {
	return t&allTargets == t && t != 0 && t&(t-1) == 0
}

func (t TargetType) String() string {
	switch t {
	case DescribesPackage:
		return "package"
	case DescribesType:
		return "type"
	case DescribesField:
		return "field"
	}
	return fmt.Sprintf("target(%d)", int(t))
}
