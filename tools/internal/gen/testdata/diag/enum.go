package diag

// rule: +game:enum requires an integer type.
//
// +game:enum
type EnumString string

// rule: +game:enum needs exported constants.
//
// +game:enum
type EnumEmpty int

const enumEmptyHidden EnumEmpty = 0

// rule: +game:enum needs a constant with value 0.
//
// +game:enum
type EnumNoZero int

const EnumNoZeroOne EnumNoZero = 1

// rule: two constants of a +game:enum must not share a value.
//
// +game:enum
type EnumAlias int

const (
	EnumAliasA EnumAlias = 0
	EnumAliasB EnumAlias = 0
)

// rule: +game:enum on a type with custom JSON.
//
// +game:enum
type EnumJSON int

const EnumJSONZero EnumJSON = 0

func (EnumJSON) MarshalJSON() ([]byte, error) { return nil, nil }

// rule: +game:enum is a type marker.
//
// +game:component
type EnumOnField struct {
	// +game:enum
	V int
}
