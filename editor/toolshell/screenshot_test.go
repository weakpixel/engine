package toolshell

import (
	"strings"
	"testing"
	"time"
)

// answers returns and forgets the screenshot messages sent so far.
func (t *fakeTransport) answers() []map[string]any {
	var out, rest []map[string]any
	for _, m := range t.msgs {
		if m["type"] == "screenshot" {
			out = append(out, m)
		} else {
			rest = append(rest, m)
		}
	}
	t.msgs = rest
	return out
}

func TestScreenshotBadTarget(t *testing.T) {
	tr := &fakeTransport{}
	f := newFixture(t, Config{Transport: tr})
	f.tick(time.Millisecond)
	tr.answers()
	f.send(`{"type":"screenshot","id":"s1","target":"moon"}`)
	f.tick(time.Millisecond)
	got := tr.answers()
	if len(got) != 1 || got[0]["id"] != "s1" || !strings.Contains(got[0]["error"].(string), `unknown target "moon"`) {
		t.Fatalf("answers = %v", got)
	}
	for _, typ := range tr.types() {
		if typ == "error" {
			t.Error("a screenshot error must not also send an error message")
		}
	}
}

// TestViewSectionHasScreenshotButton: every tool's view section offers the
// button, and SetView takes it.
func TestViewSectionHasScreenshotButton(t *testing.T) {
	f := newFixture(t, Config{})
	f.tick(time.Millisecond)
	found := false
	for _, fld := range f.s.ViewSection().Fields {
		found = found || fld.ID == "view.shot"
	}
	if !found {
		t.Fatal("no view.shot button")
	}
	if ok, err := f.s.SetView("view.shot", nil); !ok || err != nil {
		t.Fatalf("SetView = %v, %v", ok, err)
	}
}
