package viewkit

import (
	"testing"

	"github.com/weakpixel/engine/editor/toolshell"
)

func TestFramesSectionAndSet(t *testing.T) {
	f := NewFrames("sheet")
	f.SetList([]Frame{{Index: 0}, {Index: 1, Name: "run"}})
	sec := f.Section()
	if sec.ID != "frames" || sec.Side != toolshell.SideLeft || len(sec.Fields) != 2 {
		t.Fatalf("section = %+v", sec)
	}
	list := sec.Fields[1]
	if list.ID != "sheet.frame" || list.List != FrameRows || len(list.Options) != 3 || list.Options[2] != "1 run" {
		t.Errorf("list = %+v", list)
	}
	for _, c := range []struct {
		id, val string
		sel     int
		mode    LabelMode
		ok, err bool
	}{
		{"sheet.frame", "2", 1, LabelBoth, true, false},
		{"sheet.frame", "2", 1, LabelBoth, true, false}, // again: still selected
		{"sheet.frame", "3", 1, LabelBoth, true, true},
		{"sheet.labels", "3", 1, LabelNone, true, false},
		{"sheet.labels", "4", 1, LabelNone, true, true},
		{"sheet.frame", "0", -1, LabelNone, true, false},
		{"atlas.frame", "1", -1, LabelNone, false, false},
	} {
		ok, err := f.Set(c.id, []byte(c.val))
		if ok != c.ok || (err != nil) != c.err || f.Sel != c.sel || f.Mode != c.mode {
			t.Errorf("Set(%s, %s) = %v, %v; sel %d mode %d", c.id, c.val, ok, err, f.Sel, f.Mode)
		}
	}
	f.Sel = 1
	f.SetList([]Frame{{Index: 0}})
	if f.Sel != -1 {
		t.Errorf("a selection past the new list was kept: %d", f.Sel)
	}
}
