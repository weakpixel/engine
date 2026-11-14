package toolshell

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestMemoryDocument(t *testing.T) {
	d := NewMemoryDocument([]byte("a"))
	if !d.Dirty() {
		t.Error("an untitled document is dirty from the start")
	}
	d.Edit([]byte("b"))
	d.Edit([]byte("c"))
	if !d.Undo() || string(d.Text()) != "b" {
		t.Errorf("undo: %q", d.Text())
	}
	if !d.Redo() || string(d.Text()) != "c" {
		t.Errorf("redo: %q", d.Text())
	}
	var _ Undoer = d
}

func TestCreateNativeSwitchesToFileDocumentKeepingUndo(t *testing.T) {
	root := t.TempDir()
	mem := NewMemoryDocument([]byte(`{"n":1}`))
	mem.Edit([]byte(`{"n":2}`))
	f := newFixture(t, Config{Document: mem, Root: root})
	f.tick(time.Millisecond)
	if !f.s.Dirty() {
		t.Error("untitled shell should be dirty")
	}
	err := f.s.Create("fx/a.prefab.json", []byte(`{"n":2}`), []CreateFile{{Path: "fx/a.png", Data: []byte("PNG")}})
	if err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]string{"fx/a.prefab.json": `{"n":2}`, "fx/a.png": "PNG"} {
		got, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(name)))
		if err != nil || string(got) != want {
			t.Errorf("%s = %q, %v", name, got, err)
		}
	}
	fd, ok := f.s.cfg.Document.(*FileDocument)
	if !ok || fd.Path() != filepath.Join(root, "fx", "a.prefab.json") {
		t.Fatalf("document = %T", f.s.cfg.Document)
	}
	if fd.Dirty() || f.s.Dirty() || f.s.Key() != "fx/a.prefab.json" {
		t.Errorf("dirty=%v key=%q after saving", fd.Dirty(), f.s.Key())
	}
	if !fd.Undo() || string(fd.Text()) != `{"n":1}` || !fd.Dirty() {
		t.Errorf("undo after Save as: text=%q dirty=%v", fd.Text(), fd.Dirty())
	}
}

func TestCreateKeepsDocumentOfASavedFile(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "x.prefab.json")
	os.WriteFile(path, []byte("x"), 0o644)
	doc, _ := OpenFileDocument(path)
	f := newFixture(t, Config{Document: doc, Root: root})
	f.tick(time.Millisecond)
	if err := f.s.Create("copy.prefab.json", []byte("x"), nil); err != nil {
		t.Fatal(err)
	}
	if f.s.cfg.Document != Document(doc) || f.s.Key() != "doc.json" {
		t.Error("a copy must not move the open document")
	}
}

func TestCreateRejectsBadKeys(t *testing.T) {
	root := t.TempDir()
	f := newFixture(t, Config{Document: NewMemoryDocument(nil), Root: root})
	f.tick(time.Millisecond)
	for _, key := range []string{"", "/abs.json", "../out.json", "a/../../out.json", "a//b.json", "a\\b.json", "./x.json"} {
		if err := f.s.Create(key, []byte("x"), nil); err == nil {
			t.Errorf("Create(%q) accepted", key)
		}
	}
	if err := f.s.Create("ok.json", []byte("x"), []CreateFile{{Path: "../evil.png"}}); err == nil || !strings.Contains(err.Error(), "evil") {
		t.Errorf("bad file path: %v", err)
	}
	if entries, _ := os.ReadDir(root); len(entries) != 0 {
		t.Errorf("a rejected Create wrote %v", entries)
	}
	if err := New(Config{Tool: &counterTool{}}).Create("a.json", nil, nil); err == nil {
		t.Error("Create without a root or transport succeeded")
	}
}

func TestCreateSendsMessageInThePage(t *testing.T) {
	tr := &fakeTransport{}
	f := newFixture(t, Config{Document: NewMemoryDocument([]byte(`{"n":1}`)), Transport: tr})
	f.tick(time.Millisecond)
	ready := tr.msgs[0]
	if ready["type"] != "ready" || ready["document"] != false || ready["path"] != "doc.json" {
		t.Errorf("ready = %v", ready)
	}
	tr.msgs = nil
	if err := f.s.Create("fx/a.prefab.json", []byte("text"), []CreateFile{{Path: "fx/a.png", Data: []byte{1, 2, 3}}}); err != nil {
		t.Fatal(err)
	}
	if len(tr.msgs) != 1 {
		t.Fatalf("messages = %v", tr.msgs)
	}
	raw, _ := json.Marshal(tr.msgs[0])
	const want = `{"files":[{"data":"AQID","path":"fx/a.png"}],"path":"fx/a.prefab.json","text":"text","type":"create"}`
	if string(raw) != want {
		t.Errorf("create =\n%s\nwant\n%s", raw, want)
	}
	if _, ok := f.s.cfg.Document.(*MemoryDocument); !ok {
		t.Error("the page build keeps the untitled document until the host reopens")
	}
}
