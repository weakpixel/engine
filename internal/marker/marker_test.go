package marker

import (
	"reflect"
	"strings"
	"testing"
)

type flag struct{}

type args struct {
	N    int       `marker:"n"`
	U    uint8     `marker:"u,optional"`
	B    bool      `marker:"b,optional"`
	S    string    `marker:"s,optional"`
	L    []string  `marker:"l,optional"`
	Ints []int     `marker:"ints,optional"`
	F    float64   `marker:"f,optional"`
	Fs   []float32 `marker:"fs,optional"`
	PF   *float64  `marker:"pf"`
	PI   *int      `marker:"pi"`
	skip chan int  // unexported fields are ignored
	_    [0]func() // blank fields are ignored
}

func ptr[T any](v T) *T { return &v }

func newTestRegistry(t *testing.T, opts ...Option) *Registry {
	t.Helper()
	r := NewRegistry(opts...)
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(r.Define("t:flag", DescribesType, flag{}))
	must(r.Define("t:args", DescribesType|DescribesField, &args{}))
	must(r.Define("t:args:sub", DescribesPackage, flag{}))
	return r
}

func TestParse(t *testing.T) {
	r := newTestRegistry(t)
	tests := []struct {
		name   string
		target TargetType
		text   string
		marker string
		want   any
	}{
		{"flag", DescribesType, "+t:flag", "t:flag", flag{}},
		{"flag trailing text", DescribesType, "+t:flag is fine", "t:flag", flag{}},
		{"longest prefix", DescribesPackage, "+t:args:sub", "t:args:sub", flag{}},
		{"decimal", DescribesType, "+t:args:n=42", "t:args", args{N: 42}},
		{"hex", DescribesType, "+t:args:n=0x10", "t:args", args{N: 16}},
		{"negative", DescribesField, "+t:args:n=-3", "t:args", args{N: -3}},
		{"bool", DescribesType, "+t:args:n=1,b=true", "t:args", args{N: 1, B: true}},
		{"bare string", DescribesType, "+t:args:n=1,s=hello", "t:args", args{N: 1, S: "hello"}},
		{"string with spaces", DescribesType, "+t:args:n=1,s=hello big world", "t:args", args{N: 1, S: "hello big world"}},
		{"quoted string", DescribesType, `+t:args:n=1,s="a,b;c"`, "t:args", args{N: 1, S: "a,b;c"}},
		{"raw string", DescribesType, "+t:args:n=1,s=`a\\b`", "t:args", args{N: 1, S: `a\b`}},
		{"semicolon slice", DescribesType, "+t:args:n=1,l=a;b;c", "t:args", args{N: 1, L: []string{"a", "b", "c"}}},
		{"brace slice", DescribesType, "+t:args:n=1,l={a, b, c},u=7", "t:args", args{N: 1, U: 7, L: []string{"a", "b", "c"}}},
		{"empty brace slice", DescribesType, "+t:args:n=1,l={}", "t:args", args{N: 1, L: []string{}}},
		{"float", DescribesType, "+t:args:n=1,f=-1.5", "t:args", args{N: 1, F: -1.5}},
		{"float slice", DescribesType, "+t:args:n=1,fs={1, 2.5}", "t:args", args{N: 1, Fs: []float32{1, 2.5}}},
		{"pointer set", DescribesType, "+t:args:n=1,pf=0,pi=0", "t:args", args{N: 1, PF: ptr(0.0), PI: ptr(0)}},
		{"pointer absent", DescribesType, "+t:args:n=1", "t:args", args{N: 1}},
		{"int slice", DescribesType, "+t:args:n=1,ints={1,0x2,3}", "t:args", args{N: 1, Ints: []int{1, 2, 3}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			name, got, err := r.Parse(tt.target, tt.text)
			if err != nil {
				t.Fatal(err)
			}
			if name != tt.marker || !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("got %q %#v, want %q %#v", name, got, tt.marker, tt.want)
			}
		})
	}
}

func TestParseErrors(t *testing.T) {
	r := newTestRegistry(t)
	tests := []struct {
		name   string
		target TargetType
		text   string
		want   string
	}{
		{"not a marker", DescribesType, "t:flag", "must start with +"},
		{"unknown", DescribesType, "+t:flags trailing", "unknown marker +t:flags"},
		{"wrong target", DescribesField, "+t:flag", "marker +t:flag is not valid on a field"},
		{"bad target", DescribesField | DescribesType, "+t:flag", "invalid target"},
		{"bare value", DescribesType, "+t:flag=1", "unexpected =value"},
		{"flag with arg", DescribesType, "+t:flag:x=1", `unknown argument "x"`},
		{"empty arg list", DescribesType, "+t:flag:", ""},
		{"unknown arg", DescribesType, "+t:args:n=1,z=2", `unknown argument "z"`},
		{"missing required", DescribesType, "+t:args:b=true", `missing required argument "n"`},
		{"missing required no args", DescribesType, "+t:args", `missing required argument "n"`},
		{"duplicate arg", DescribesType, "+t:args:n=1,n=2", `duplicate argument "n"`},
		{"no value", DescribesType, "+t:args:n", `argument "n" needs a value`},
		{"missing value", DescribesType, "+t:args:n=", "missing value"},
		{"bad int", DescribesType, "+t:args:n=x", `invalid int "x"`},
		{"int overflow", DescribesType, "+t:args:n=1,u=300", `invalid uint8 "300"`},
		{"bad float", DescribesType, "+t:args:n=1,f=x", `invalid float64 "x"`},
		{"bad pointer value", DescribesType, "+t:args:n=1,pi=1.5", `invalid int "1.5"`},
		{"bad bool", DescribesType, "+t:args:n=1,b=maybe", `invalid bool "maybe"`},
		{"unterminated quote", DescribesType, `+t:args:n=1,s="abc`, "unterminated"},
		{"unterminated brace", DescribesType, "+t:args:n=1,l={a,b", "unterminated '{'"},
		{"bad list", DescribesType, "+t:args:n=1,l={a b c", "unterminated '{'"},
		{"semicolon in scalar", DescribesType, "+t:args:n=1;2", `unexpected ";2"`},
		{"trailing comma", DescribesType, "+t:args:n=1,", "missing argument after"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := r.Parse(tt.target, tt.text)
			if err == nil {
				if tt.want == "" {
					t.Skip("accepted")
				}
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error %q does not contain %q", err, tt.want)
			}
		})
	}
}

func TestDefineErrors(t *testing.T) {
	type noTag struct{ A int }
	type emptyTag struct {
		A int `marker:""`
	}
	type badOpt struct {
		A int `marker:"a,wat"`
	}
	type badType struct {
		A complex128 `marker:"a"`
	}
	type ptrOpt struct {
		A *int `marker:"a,optional"`
	}
	type ptrSlice struct {
		A []*int `marker:"a"`
	}
	type badElem struct {
		A [][]int `marker:"a"`
	}
	type dupArg struct {
		A int `marker:"x"`
		B int `marker:"x"`
	}
	tests := []struct {
		name   string
		mname  string
		target TargetType
		obj    any
	}{
		{"empty name", "", DescribesType, flag{}},
		{"bad name", "a::b", DescribesType, flag{}},
		{"no target", "a", 0, flag{}},
		{"bad target", "a", 1 << 10, flag{}},
		{"nil", "a", DescribesType, nil},
		{"non-struct", "a", DescribesType, 3},
		{"nil pointer to non-struct", "a", DescribesType, new(int)},
		{"no tag", "a", DescribesType, noTag{}},
		{"empty tag", "a", DescribesType, emptyTag{}},
		{"bad option", "a", DescribesType, badOpt{}},
		{"unsupported type", "a", DescribesType, badType{}},
		{"pointer with optional", "a", DescribesType, ptrOpt{}},
		{"slice of pointers", "a", DescribesType, ptrSlice{}},
		{"unsupported elem", "a", DescribesType, badElem{}},
		{"duplicate arg", "a", DescribesType, dupArg{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := NewRegistry().Define(tt.mname, tt.target, tt.obj); err == nil {
				t.Fatal("expected error")
			}
		})
	}

	t.Run("duplicate name", func(t *testing.T) {
		r := NewRegistry()
		if err := r.Define("a", DescribesType, flag{}); err != nil {
			t.Fatal(err)
		}
		if err := r.Define("a", DescribesField, flag{}); err == nil {
			t.Fatal("expected error")
		}
	})
}

func TestLookup(t *testing.T) {
	r := newTestRegistry(t)
	def, ok := r.Lookup("t:args")
	if !ok || def.Name != "t:args" || def.Target != DescribesType|DescribesField {
		t.Fatalf("lookup: %+v %v", def, ok)
	}
	if _, ok := r.Lookup("nope"); ok {
		t.Fatal("unexpected definition")
	}
}

func TestParsePaths(t *testing.T) {
	r := NewRegistry()
	if err := r.DefinePaths("t:def", DescribesField); err != nil {
		t.Fatal(err)
	}
	if err := r.Define("t:flag", DescribesField, flag{}); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name, text string
		want       PathArgs
		err        string
	}{
		{"bare", "+t:def=1.5", PathArgs{HasBare: true, Bare: "1.5"}, ""},
		{"bare list", "+t:def={1, 2, 3}", PathArgs{HasBare: true, Bare: "{1, 2, 3}"}, ""},
		{"bare empty list", "+t:def={}", PathArgs{HasBare: true, Bare: "{}"}, ""},
		{"bare expression", "+t:def=gfx.White", PathArgs{HasBare: true, Bare: "gfx.White"}, ""},
		{"paths", "+t:def:X=1,Y=2", PathArgs{Paths: []PathValue{{"X", "1"}, {"Y", "2"}}}, ""},
		{"dotted path", "+t:def:Offset.X=2", PathArgs{Paths: []PathValue{{"Offset.X", "2"}}}, ""},
		{"map keys", "+t:def:walk=2, run=4", PathArgs{Paths: []PathValue{{"walk", "2"}, {"run", "4"}}}, ""},
		{"quoted comma", `+t:def:A="a, b",B=1`, PathArgs{Paths: []PathValue{{"A", `"a, b"`}, {"B", "1"}}}, ""},
		{"call comma", "+t:def:A=f(1, 2),B=1", PathArgs{Paths: []PathValue{{"A", "f(1, 2)"}, {"B", "1"}}}, ""},
		{"no value", "+t:def", PathArgs{}, "needs a value"},
		{"empty bare", "+t:def=", PathArgs{}, "missing value"},
		{"empty path value", "+t:def:X=", PathArgs{}, "missing value"},
		{"no equals", "+t:def:X", PathArgs{}, "needs a value"},
		{"bad path", "+t:def:X..Y=1", PathArgs{}, "invalid path"},
		{"duplicate path", "+t:def:X=1,X=2", PathArgs{}, "duplicate path"},
		{"trailing comma", "+t:def:X=1,", PathArgs{}, "missing path"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			name, got, err := r.Parse(DescribesField, tt.text)
			if tt.err != "" {
				if err == nil || !strings.Contains(err.Error(), tt.err) {
					t.Fatalf("err = %v, want containing %q", err, tt.err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if name != "t:def" || !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("got %q %+v, want %+v", name, got, tt.want)
			}
		})
	}
	def, _ := r.Lookup("t:def")
	flagDef, _ := r.Lookup("t:flag")
	if !def.Repeatable() || flagDef.Repeatable() {
		t.Error("only path markers are repeatable")
	}
	if _, _, err := r.Parse(DescribesField, "+t:flag=1"); err == nil || !strings.Contains(err.Error(), "unexpected =value") {
		t.Errorf("other markers keep the =value error, got %v", err)
	}
}

func TestSplitList(t *testing.T) {
	tests := []struct {
		in   string
		want []string
		err  bool
	}{
		{"{}", nil, false},
		{"{1}", []string{"1"}, false},
		{"{1, 2, 3}", []string{"1", "2", "3"}, false},
		{"{1, 2,}", []string{"1", "2"}, false},
		{`{"a, b", f(1, 2)}`, []string{`"a, b"`, "f(1, 2)"}, false},
		{"1, 2", nil, true},
		{"{1,, 2}", nil, true},
	}
	for _, tt := range tests {
		got, err := SplitList(tt.in)
		if (err != nil) != tt.err || !reflect.DeepEqual(got, tt.want) {
			t.Errorf("SplitList(%q) = %q, %v; want %q, err=%v", tt.in, got, err, tt.want, tt.err)
		}
	}
}
