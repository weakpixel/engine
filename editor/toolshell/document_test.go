package toolshell

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/weakpixel/engine/input"
)

func writeDoc(t *testing.T, text string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "doc.json")
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestDocumentUndoRedoSave(t *testing.T) {
	path := writeDoc(t, "a")
	d, err := OpenFileDocument(path)
	if err != nil {
		t.Fatal(err)
	}
	if d.Dirty() || d.Undo() {
		t.Fatal("a fresh document is clean with nothing to undo")
	}
	d.Edit([]byte("b"))
	d.Edit([]byte("c"))
	if string(d.Text()) != "c" || !d.Dirty() {
		t.Fatalf("text = %q dirty = %v", d.Text(), d.Dirty())
	}
	d.Undo()
	d.Undo()
	if string(d.Text()) != "a" || d.Dirty() {
		t.Errorf("after two undos: text = %q dirty = %v, want a and clean", d.Text(), d.Dirty())
	}
	d.Redo()
	if string(d.Text()) != "b" {
		t.Errorf("redo = %q, want b", d.Text())
	}
	d.Edit([]byte("d")) // drops the redo stack
	if d.Redo() {
		t.Error("an edit kept the redo stack")
	}
	if err := d.Save(); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(path); string(got) != "d" || d.Dirty() {
		t.Errorf("file = %q dirty = %v after save", got, d.Dirty())
	}
}

func TestDocumentExternalChange(t *testing.T) {
	path := writeDoc(t, "a")
	d, _ := OpenFileDocument(path)
	if ext, err := d.CheckDisk(); err != nil || ext != ExternalNone {
		t.Fatalf("CheckDisk = %v, %v; want none", ext, err)
	}
	os.WriteFile(path, []byte("b"), 0o644)
	if ext, _ := d.CheckDisk(); ext != ExternalReloaded || string(d.Text()) != "b" {
		t.Errorf("clean document: ext = %v text = %q, want reloaded b", ext, d.Text())
	}
	d.Edit([]byte("mine"))
	os.WriteFile(path, []byte("c"), 0o644)
	if ext, _ := d.CheckDisk(); ext != ExternalConflict || string(d.Text()) != "mine" {
		t.Errorf("dirty document: ext = %v text = %q, want a conflict and the edits kept", ext, d.Text())
	}
}

func TestDocumentShellKeysAndReload(t *testing.T) {
	path := writeDoc(t, `{"n":1}`)
	d, _ := OpenFileDocument(path)
	f := newFixture(t, Config{Document: d})
	f.tick(time.Millisecond)
	if f.tool.n != 1 {
		t.Fatalf("n = %v, want the file loaded on the first tick", f.tool.n)
	}
	f.send(`{"type":"set","id":"n","value":2}`)
	f.tick(time.Millisecond)
	if !f.s.Dirty() {
		t.Fatal("an edit left the document clean")
	}
	// Ctrl+Z undoes and loads the old text into the tool.
	f.src.Frame.Keys = keys(input.KeyControlLeft, input.KeyZ)
	f.tick(time.Millisecond)
	if f.tool.n != 1 || f.s.Dirty() {
		t.Errorf("after undo: n = %v dirty = %v", f.tool.n, f.s.Dirty())
	}
	// Ctrl+Shift+Z redoes.
	f.src.Frame.Keys = keys(input.KeyControlLeft)
	f.tick(time.Millisecond)
	f.src.Frame.Keys = keys(input.KeyControlLeft, input.KeyShiftLeft, input.KeyZ)
	f.tick(time.Millisecond)
	if f.tool.n != 2 {
		t.Errorf("after redo: n = %v, want 2", f.tool.n)
	}
	// Ctrl+S writes.
	f.src.Frame.Keys = keys(input.KeyControlLeft)
	f.tick(time.Millisecond)
	f.src.Frame.Keys = keys(input.KeyControlLeft, input.KeyS)
	f.tick(time.Millisecond)
	if got, _ := os.ReadFile(path); string(got) != `{"n":2}` {
		t.Errorf("file = %q after Ctrl+S", got)
	}
	// A change on disk with no edits reloads into the tool after the poll.
	f.src.Frame.Keys = input.KeySet{}
	os.WriteFile(path, []byte(`{"n":9}`), 0o644)
	f.tick(diskPoll)
	if f.tool.n != 9 {
		t.Errorf("n = %v after an external change, want 9", f.tool.n)
	}
	// With unsaved edits the change is reported, not loaded.
	f.send(`{"type":"set","id":"n","value":4}`)
	f.tick(time.Millisecond)
	os.WriteFile(path, []byte(`{"n":10}`), 0o644)
	f.tick(diskPoll)
	if f.tool.n != 4 || f.s.fullPanel().Status == "" {
		t.Errorf("n = %v status = %q; want the edit kept and a status", f.tool.n, f.s.fullPanel().Status)
	}
}

func TestNewFileDocument(t *testing.T) {
	path := filepath.Join(t.TempDir(), "new.json")
	d := NewFileDocument(path, []byte("start"))
	if !d.Dirty() || string(d.Text()) != "start" {
		t.Fatalf("new document: dirty %v text %q", d.Dirty(), d.Text())
	}
	if ext, err := d.CheckDisk(); err != nil || ext != ExternalNone {
		t.Errorf("CheckDisk of a file that does not exist yet = %v, %v", ext, err)
	}
	d.Edit([]byte("edited"))
	if err := d.Save(); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(path); err != nil || string(data) != "edited" {
		t.Errorf("file = %q, %v", data, err)
	}
	if d.Dirty() {
		t.Error("saved document is dirty")
	}
}
