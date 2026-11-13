package animfile

import (
	"strings"
	"testing"
)

func heroSheet(t *testing.T) *Sheet {
	t.Helper()
	s, err := DecodeSheet([]byte(sheetHash(6, heroDurs, heroTags)))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestDiagnose(t *testing.T) {
	tests := []struct {
		name    string
		members string
		sizeX   int32
		want    []string // "<level>: <substring>"
		resolve bool
	}{
		{name: "unlisted tags are info", members: `"clips":{"idle":{}}`, want: []string{"info: tag \"run\""}, resolve: true},
		{name: "unknown clip is an error", members: `"clips":{"nope":{}}`, want: []string{`error: clip "nope": no such tag`}},
		{name: "two errors, two findings", members: `"defaultClip":"x","clips":{"nope":{}}`, want: []string{`error: "defaultClip"`, `error: clip "nope"`}},
		{name: "wide sheet warns", members: ``, sizeX: 9000, want: []string{"warn: sheet \"hero.json\" is 9000x"}, resolve: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f, err := Decode([]byte(`{"version":1,"sheet":"hero.json"` + prefixComma(tc.members) + `}`))
			if err != nil {
				t.Fatal(err)
			}
			s := heroSheet(t)
			s.Size.X = tc.sizeX
			res, got := Diagnose(f, s)
			if (res != nil) != tc.resolve {
				t.Fatalf("resolved = %v, want %v", res != nil, tc.resolve)
			}
			for _, w := range tc.want {
				if !hasFinding(got, w) {
					t.Errorf("findings %v lack %q", got, w)
				}
			}
		})
	}
}

func TestDiagnoseUnevenTrim(t *testing.T) {
	s := heroSheet(t)
	f := &File{Version: 1, Sheet: "hero.json", Pivot: &Point{X: 1, Y: 1}}
	tag := s.Tags[0]
	s.Frames[tag.From].Offset = Point{X: 1}
	s.Frames[tag.To].Offset = Point{X: 2}
	if tag.From == tag.To {
		t.Skip("one-frame tag")
	}
	_, got := Diagnose(f, s)
	if !hasFinding(got, "warn: clip \""+tag.Name+"\": frames are trimmed differently") {
		t.Errorf("findings %v lack the trim warning", got)
	}
}

func TestStarter(t *testing.T) {
	s := heroSheet(t)
	f := Starter(s, "hero.json")
	if f.DefaultClip != s.Tags[0].Name || len(f.Clips) != len(s.Tags) {
		t.Fatalf("starter = %+v", f)
	}
	if _, got := Diagnose(f, s); hasFinding(got, "error") {
		t.Errorf("starter has errors: %v", got)
	}
}

func prefixComma(s string) string {
	if s == "" {
		return ""
	}
	return "," + s
}

func hasFinding(fs []Finding, want string) bool {
	for _, f := range fs {
		if strings.Contains(string(f.Level)+": "+f.Msg, want) {
			return true
		}
	}
	return false
}
