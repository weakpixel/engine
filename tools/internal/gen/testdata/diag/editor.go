package diag

// BadEditor has +game:editor markers that cannot take effect.
//
// +game:component
type BadEditor struct {
	Mode  Level
	Flag  bool
	Name  string
	Count int32

	// +game:editor:min=x
	BadMin float32
	// +game:editor:min=5,max=1
	Inverted float32
	// +game:editor:step=0
	ZeroStep float32
	// +game:editor:order=first
	BadOrder float32
	// +game:editor:min=0
	OnString string
	// +game:editor:unit=deg
	OnBool bool
	// +game:editor:color=true
	NotColor float32
	// +game:editor:color=true
	WrongArray [2]float32
	// +game:editor:showIf=Missing
	NoSibling float32
	// +game:editor:showIf=Mode=LevelNope
	BadEnum float32
	// +game:editor:showIf=Flag=maybe
	BadBool float32
	// +game:editor:showIf=Count=many
	BadInt float32
	// +game:editor:showIf=OnSelf
	OnSelf float32
	// +game:editor:bogus=1
	Unknown float32
	// +game:editor:name=Hidden
	Skipped int32 `json:"-"`
	// +game:editor:name=Unexported
	unexported float32
}
