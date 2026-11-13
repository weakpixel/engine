package animfile

import (
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
)

const ms = time.Millisecond

// sheetHash is a hash-form export of n frames with the given durations (ms,
// cycled) and the tags JSON ("" for none).
func sheetHash(n int, durs []int, tags string) string {
	var sb strings.Builder
	sb.WriteString(`{"frames":{`)
	for i := range n {
		if i > 0 {
			sb.WriteString(",")
		}
		sb.WriteString(`"hero ` + strconv.Itoa(i) + `.aseprite":{"frame":{"x":0,"y":0,"w":4,"h":4},"duration":` + strconv.Itoa(durs[i%len(durs)]) + `}`)
	}
	sb.WriteString(`},"meta":{"image":"hero.png"`)
	if tags != "" {
		sb.WriteString(`,"frameTags":` + tags)
	}
	sb.WriteString(`}}`)
	return sb.String()
}

const heroTags = `[
	{"name":"idle","from":0,"to":1,"direction":"forward"},
	{"name":"run","from":2,"to":5,"direction":"reverse"},
	{"name":"bounce","from":1,"to":3,"direction":"pingpong"},
	{"name":"bounce_rev","from":1,"to":3,"direction":"pingpong_reverse"}
]`

var heroDurs = []int{100, 200, 50, 60, 70, 80}

func mustContain(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("error %v does not contain %q", err, want)
	}
}

func durs(ds ...int) []time.Duration {
	out := make([]time.Duration, len(ds))
	for i, d := range ds {
		out[i] = time.Duration(d) * ms
	}
	return out
}

func TestDecodeSheetTags(t *testing.T) {
	s, err := DecodeSheet([]byte(sheetHash(6, heroDurs, heroTags)))
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Frames) != 6 || s.Frames[2].Name != "hero 2.aseprite" || s.Frames[1].Duration != 200*ms {
		t.Fatalf("frames = %+v", s.Frames)
	}
	want := []Tag{
		{Name: "idle", Loop: Loop, Durations: durs(100, 200)},
		{Name: "run", Loop: Loop, Durations: durs(80, 70, 60, 50)},
		{Name: "bounce", Loop: PingPong, Durations: durs(200, 50, 60)},
		{Name: "bounce_rev", Loop: PingPong, Durations: durs(60, 50, 200)},
	}
	got := s.PlayOrder()
	if len(got) != len(want) {
		t.Fatalf("PlayOrder = %+v", got)
	}
	for i := range want {
		if got[i].Name != want[i].Name || got[i].Loop != want[i].Loop || !slices.Equal(got[i].Durations, want[i].Durations) {
			t.Errorf("tag %d = %+v, want %+v", i, got[i], want[i])
		}
	}
	if idx := s.Tags[1].FrameIndices(); !slices.Equal(idx, []int{5, 4, 3, 2}) {
		t.Errorf("run FrameIndices = %v", idx)
	}
}

func TestDecodeSheetArrayFormTrimAndNoTags(t *testing.T) {
	const sheet = `{"frames":[
		{"filename":"a","frame":{"x":0,"y":0,"w":4,"h":4},"trimmed":true,"spriteSourceSize":{"x":2,"y":1,"w":4,"h":4},"duration":40},
		{"filename":"b","frame":{"x":4,"y":0,"w":4,"h":4},"duration":60}
	],"meta":{"image":"hero.png","frameTags":[]}}`
	s, err := DecodeSheet([]byte(sheet))
	if err != nil {
		t.Fatal(err)
	}
	if f := s.Frames[0]; f.Name != "a" || !f.Trimmed || f.Offset != (Point{X: 2, Y: 1}) {
		t.Errorf("frame 0 = %+v", f)
	}
	if f := s.Frames[1]; f.Trimmed || f.Offset != (Point{}) {
		t.Errorf("frame 1 = %+v", f)
	}
	if len(s.Tags) != 1 || s.Tags[0] != (SheetTag{Name: AllClip, From: 0, To: 1, Loop: Loop}) {
		t.Fatalf("tags = %+v, want one %q tag", s.Tags, AllClip)
	}
}

func TestDecodeSheetErrors(t *testing.T) {
	tests := []struct {
		name  string
		sheet string
		want  []string
	}{
		{"tag outside frames", sheetHash(2, []int{50}, `[{"name":"x","from":0,"to":5}]`), []string{"outside the file's 2 frames"}},
		{"unknown direction", sheetHash(2, []int{50}, `[{"name":"x","from":0,"to":1,"direction":"sideways"}]`), []string{`unknown direction "sideways"`}},
		{"duplicate tag", sheetHash(2, []int{50}, `[{"name":"x","from":0,"to":0},{"name":"x","from":1,"to":1}]`), []string{`duplicate tag name "x"`}},
		{"unnamed tag", sheetHash(2, []int{50}, `[{"from":0,"to":0}]`), []string{"a tag has no name"}},
		{"no duration", `{"frames":{"a":{"frame":{"x":0,"y":0,"w":4,"h":4}}},"meta":{}}`, []string{"no positive duration"}},
		{"no frames", `{"meta":{}}`, []string{`"frames" is required`}},
		{"empty frames", `{"frames":[],"meta":{}}`, []string{`no "frames"`}},
		{"frames not a collection", `{"frames":3}`, []string{"must be an object or an array"}},
		{"not json", `nope`, []string{"invalid character"}},
		{"every tag problem", sheetHash(2, []int{50}, `[{"name":"a","from":0,"to":9},{"name":"b","from":0,"to":1,"direction":"zig"}]`), []string{`tag "a"`, `tag "b"`}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := DecodeSheet([]byte(tt.sheet))
			for _, w := range tt.want {
				mustContain(t, err, w)
			}
		})
	}
}
