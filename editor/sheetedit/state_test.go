package sheetedit

import (
	"encoding/json"
	"image"
	"strings"
	"testing"

	"github.com/weakpixel/engine/editor/toolshell"
	"github.com/weakpixel/engine/sheetfile"
)

func newState(t *testing.T, size image.Point) (*State, *[]string) {
	t.Helper()
	var sent []string
	s := NewState(func(text []byte) { sent = append(sent, string(text)) })
	s.SetFile(&sheetfile.File{Image: "a.png", FrameW: 16, FrameH: 16})
	if size != (image.Point{}) {
		s.SetImage(size, nil)
	}
	return s, &sent
}

func num(v float64) json.RawMessage { b, _ := json.Marshal(v); return b }

func TestSetWritesOneCanonicalEditPerChange(t *testing.T) {
	s, sent := newState(t, image.Pt(64, 32))
	if err := s.Set(IDFrameW, num(8)); err != nil {
		t.Fatal(err)
	}
	if len(*sent) != 1 {
		t.Fatalf("%d edits, want 1", len(*sent))
	}
	f, err := sheetfile.Decode([]byte((*sent)[0]))
	if err != nil || f.FrameW != 8 || f.FrameH != 16 {
		t.Fatalf("written %q: %v", (*sent)[0], err)
	}
	if g := s.Grid(); g.Cols != 8 || g.Rows != 2 || g.Count() != 16 {
		t.Errorf("grid %dx%d", g.Cols, g.Rows)
	}
	if err := s.Set(IDFrameW, num(8)); err != nil || len(*sent) != 1 {
		t.Errorf("an unchanged value sent an edit: %v", err)
	}
}

func TestSetRefusesBadValues(t *testing.T) {
	s, sent := newState(t, image.Pt(64, 32))
	for _, tt := range []struct {
		id string
		v  json.RawMessage
	}{
		{IDFrameW, num(0)}, {IDFrameH, num(-3)}, {IDMargin, num(-1)}, {IDSpacing, num(1.5)},
		{IDFrameW, json.RawMessage(`"x"`)}, {"sheet.nope", num(1)},
	} {
		if err := s.Set(tt.id, tt.v); err == nil {
			t.Errorf("Set(%s, %s) accepted", tt.id, tt.v)
		}
	}
	if len(*sent) != 0 || s.File.FrameW != 16 || s.File.Margin != 0 {
		t.Errorf("a refused value changed the file: %v %+v", *sent, s.File)
	}
}

func TestStaleDocumentIsNotOverwritten(t *testing.T) {
	s, sent := newState(t, image.Pt(64, 32))
	s.MarkStale(json.Unmarshal([]byte("{"), new(any)))
	if err := s.Set(IDFrameW, num(8)); err != toolshell.ErrStale || len(*sent) != 0 {
		t.Errorf("err %v, %d edits", err, len(*sent))
	}
	if p := s.Panel("a", toolshell.Section{ID: "view", Label: "View"}); !strings.Contains(p.Status, "document has errors") {
		t.Errorf("status %q", p.Status)
	}
	s.SetFile(s.File)
	if s.Stale != "" {
		t.Error("a new document did not clear the stale mark")
	}
}

func TestHoverAndInfo(t *testing.T) {
	s, _ := newState(t, image.Pt(70, 40))
	s.File.Margin, s.File.Spacing = 1, 2
	s.SetHover(image.Pt(20, 5), true) // second cell of the first row
	if s.Hover != 1 {
		t.Fatalf("hover %d", s.Hover)
	}
	s.SetHover(image.Pt(17, 5), true) // spacing
	if s.Hover != -1 {
		t.Errorf("hover %d over spacing", s.Hover)
	}
	s.SetHover(image.Pt(20, 5), false)
	if s.Hover != -1 {
		t.Errorf("hover %d off the image", s.Hover)
	}
	p := s.Panel("a.sheet.json", toolshell.Section{ID: "view", Label: "View"})
	if err := p.Check(); err != nil {
		t.Fatal(err)
	}
	for id, want := range map[string]string{"info.grid": "3 × 2", "info.count": "6", "info.size": "70 × 40 px"} {
		f, ok := p.Find(id)
		if !ok || !strings.Contains(string(f.Value), want) {
			t.Errorf("%s = %s, want %q", id, f.Value, want)
		}
	}
}

func TestFindings(t *testing.T) {
	s, _ := newState(t, image.Pt(10, 10))
	if got := s.Findings(); len(got) != 1 || !strings.Contains(got[0], "does not fit") {
		t.Errorf("findings %v", got)
	}
	s.SetImage(image.Point{}, errString("missing"))
	if got := s.Findings(); len(got) != 1 || !strings.Contains(got[0], "missing") {
		t.Errorf("findings %v", got)
	}
	p := s.Panel("a", toolshell.Section{ID: "view", Label: "View"})
	if _, ok := p.Find("problem.0"); !ok {
		t.Error("no problem shown")
	}
	if err := p.Check(); err != nil {
		t.Fatal(err)
	}
	s.SetImage(image.Pt(64, 64), nil)
	if len(s.Findings()) != 0 {
		t.Error("findings for a fitting grid")
	}
}

type errString string

func (e errString) Error() string { return string(e) }
