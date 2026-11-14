package animfile

import (
	"reflect"
	"strings"
	"testing"
)

func TestDecodeErrors(t *testing.T) {
	tests := []struct {
		name string
		def  string
		want string
	}{
		{"not json", `nope`, "invalid character"},
		{"bad version", `{"version":2,"sheet":"hero.json"}`, `"version" is 2, want 1`},
		{"missing version", `{"sheet":"hero.json"}`, `"version" is 0, want 1`},
		{"missing sheet", `{"version":1}`, `"sheet" is required`},
		{"sheet is a definition", `{"version":1,"sheet":"other.anim.json"}`, "must be an Aseprite export"},
		{"unknown top-level field", `{"version":1,"sheet":"a.json","pivto":{"x":1,"y":1}}`, `unknown field "pivto"`},
		{"unknown clip field", `{"version":1,"sheet":"a.json","clips":{"idle":{"lop":true}}}`, `clip "idle": json: unknown field "lop"`},
		{"unknown shape field", `{"version":1,"sheet":"a.json","clips":{"idle":{"shapes":[{"kind":"hit","rect":{"x":0,"y":0,"w":1,"h":1},"z":1}]}}}`, `unknown field "z"`},
		{"float geometry", `{"version":1,"sheet":"a.json","pivot":{"x":1.5,"y":2}}`, "cannot unmarshal number 1.5"},
		{"float fps", `{"version":1,"sheet":"a.json","clips":{"idle":{"fps":7.5}}}`, "cannot unmarshal number 7.5"},
		{"clips not an object", `{"version":1,"sheet":"a.json","clips":[]}`, `"clips" must be an object`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Decode([]byte(tt.def))
			mustContain(t, err, tt.want)
		})
	}
}

func TestDecodeReportsVersionAndSheetTogether(t *testing.T) {
	_, err := Decode([]byte(`{"version":3}`))
	mustContain(t, err, `"version" is 3`)
	mustContain(t, err, `"sheet" is required`)
}

func TestEncodeRoundTrip(t *testing.T) {
	yes, fps, speed, to := true, 8, 1.5, 3
	f := &File{
		Version:     Version,
		Sheet:       "hero.json",
		Pivot:       &Point{X: 19, Y: 45},
		DefaultClip: "walk",
		Body:        []Rect{{X: -4, Y: -13, W: 8, H: 13}},
		Clips: Clips{
			// Not in alphabetical order: Encode must keep slice order.
			{Name: "walk", Clip: Clip{Loop: &yes, FPS: &fps}},
			{Name: "attack", Clip: Clip{Speed: &speed, Durations: []int{10, 20},
				Shapes: []Shape{{Kind: "hit", ID: "a", Frames: &FrameRange{From: 1, To: &to}, Rect: Rect{X: 1, Y: 2, W: 3, H: 4}}}}},
			{Name: "idle", Clip: Clip{}},
		},
	}
	data, err := Encode(f)
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	if !strings.HasSuffix(s, "}\n") || !strings.Contains(s, "\n  \"version\": 1,") {
		t.Fatalf("not indented with a trailing newline:\n%s", s)
	}
	if strings.Index(s, `"walk"`) > strings.Index(s, `"attack"`) || strings.Index(s, `"attack"`) > strings.Index(s, `"idle"`) {
		t.Fatalf("clips not in slice order:\n%s", s)
	}
	back, err := Decode(data)
	if err != nil {
		t.Fatalf("Decode(Encode(f)): %v\n%s", err, s)
	}
	if !reflect.DeepEqual(back, f) {
		t.Fatalf("round trip changed the file:\n got %+v\nwant %+v", back, f)
	}
}

func TestEncodeOmitsAbsentFields(t *testing.T) {
	data, err := Encode(&File{Version: 1, Sheet: "s.json"})
	if err != nil {
		t.Fatal(err)
	}
	if want := "{\n  \"version\": 1,\n  \"sheet\": \"s.json\"\n}\n"; string(data) != want {
		t.Fatalf("Encode = %q, want %q", data, want)
	}
}
