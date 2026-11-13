package voiceview

import (
	"slices"
	"testing"
	"time"

	"github.com/weakpixel/engine/speech"
)

func TestWrap(t *testing.T) {
	measure := func(s string) float64 { return float64(len(s)) } // 1 per byte
	lines := func(w wrapped) []string {
		var out []string
		for i := range w.starts {
			out = append(out, w.text[w.starts[i]:w.ends[i]])
		}
		return out
	}
	for _, c := range []struct {
		text  string
		width float64
		want  []string
	}{
		{"", 10, nil},
		{"hello", 10, []string{"hello"}},
		{"hello there traveler", 11, []string{"hello there", "traveler"}},
		{"hello there traveler", 5, []string{"hello", "there", "traveler"}},
		{"a b\nc", 10, []string{"a b", "c"}},
		{"extraordinarily long", 5, []string{"extraordinarily", "long"}},
	} {
		if got := lines(wrap(c.text, c.width, measure)); !slices.Equal(got, c.want) {
			t.Errorf("wrap(%q, %v) = %q, want %q", c.text, c.width, got, c.want)
		}
	}
}

func TestRollLayout(t *testing.T) {
	notes := []speech.Note{
		{On: 0, Off: 100 * time.Millisecond, Pitch: 60, Kind: speech.NoteVowel},
		{On: 500 * time.Millisecond, Off: time.Second, Pitch: 72, Kind: speech.NoteMotif},
	}
	r := rollLayout(notes, float64(time.Second), 10, 20, 100, 130, nil)
	if len(r) != 2 {
		t.Fatalf("%d rects", len(r))
	}
	// 13 rows of 10 px: pitch 72 is the top row, 60 the bottom one.
	if r[0].x != 10 || r[0].w != 10 || r[0].y != 140 || r[0].kind != speech.NoteVowel {
		t.Errorf("first note %+v", r[0])
	}
	if r[1].x != 60 || r[1].w != 50 || r[1].y != 20 {
		t.Errorf("second note %+v", r[1])
	}
	if got := rollLayout(nil, 1, 0, 0, 1, 1, nil); len(got) != 0 {
		t.Errorf("no notes gave %v", got)
	}
}
