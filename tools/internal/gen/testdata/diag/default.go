package diag

import "github.com/weakpixel/ecs"

// +game:enum
type Level uint8

const (
	LevelLow Level = iota
	LevelHigh
)

// Other is a second enum: its constants are not Level values.
//
// +game:enum
type Other uint8

const OtherA Other = 0

func f() int { return 1 }

type DefVec struct{ X, Y float64 }

type DefHandle struct{ id int }

// +game:object
type DefObject struct {
	// +game:component
	// +game:default=1
	C *BadDefaults
	// +game:default=1
	Plain int
}

// +game:component
type BadDefaults struct {
	// +game:default="x"
	WrongType float64
	// +game:default=300
	Overflow uint8
	// +game:default=1.5
	Fraction int
	// +game:default=Missing
	Unknown int
	// +game:default=f()
	NotConstant int
	// +game:default=LevelMissing
	NotAMember Level
	// +game:default=7
	BadMember Level
	// +game:default=OtherA
	WrongConstType Level

	// +game:default=1
	Entity ecs.Entity
	// +game:default=1
	// +game:runtime
	Runtime DefHandle `json:"-"`
	// +game:default="a"
	// +game:asset:kind=sprite
	Asset DefHandle `json:"-"`
	// +game:default=1
	Hidden int `json:"-"`
	// +game:default=1
	unexported int
	// +game:default=1
	Counter int `json:",string"`

	// +game:default=1
	Struct DefVec
	// +game:default:Z=1
	NoField DefVec
	// +game:default:X.Y=1
	OnScalar DefVec
	// +game:default:X=1
	// +game:default:X=2
	Twice DefVec
	// +game:default:X=1,X=2
	TwiceOneLine DefVec
	// +game:default=1
	// +game:default:X=1
	Mixed DefVec
	// +game:default:X=1
	ScalarPath float64
	// +game:default:0=1
	SlicePath []int
	// +game:default={1, 2}
	ArrayLen [3]int
	// +game:default=1
	ListNeeded []int
	// +game:default={DefVec{}}
	StructList []DefVec
	// +game:default:a=1
	MapOfSlices map[string][]int
	// +game:default:a.b=1
	MapDeep map[string]DefVec
	// +game:default:x=1
	IntKeys map[int]int
	// +game:default
	Empty int
	// +game:default=
	EmptyValue int
}
