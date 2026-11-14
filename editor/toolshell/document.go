package toolshell

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"slices"
)

// Document is the text document a [DocumentTool] edits. Tools hand it whole
// texts from the format's encoder (the shell never diffs); how the text is
// stored and undone depends on the backend: [FileDocument] natively,
// [HostDocument] in VSCode.
type Document interface {
	// Text returns the current text.
	Text() []byte
	// Edit makes text the document's new version.
	Edit(text []byte) error
}

// maxUndo bounds the undo stack of a [FileDocument].
const maxUndo = 200

// undoStack is the text and undo history shared by [FileDocument] and
// [MemoryDocument].
type undoStack struct {
	text []byte
	undo [][]byte
	redo [][]byte
}

// Text returns the current text.
func (u *undoStack) Text() []byte { return u.text }

// Edit makes text the current version: the old text goes on the undo stack
// and the redo stack is dropped. An edit that changes nothing is ignored.
func (u *undoStack) Edit(text []byte) error {
	if bytes.Equal(text, u.text) {
		return nil
	}
	u.undo = append(u.undo, u.text)
	if len(u.undo) > maxUndo {
		u.undo = slices.Delete(u.undo, 0, len(u.undo)-maxUndo)
	}
	u.redo = nil
	u.text = slices.Clone(text)
	return nil
}

// Undo returns to the previous text. It reports false when there is none.
func (u *undoStack) Undo() bool {
	if len(u.undo) == 0 {
		return false
	}
	u.redo = append(u.redo, u.text)
	u.text, u.undo = u.undo[len(u.undo)-1], u.undo[:len(u.undo)-1]
	return true
}

// Redo undoes an Undo. It reports false when there is nothing to redo.
func (u *undoStack) Redo() bool {
	if len(u.redo) == 0 {
		return false
	}
	u.undo = append(u.undo, u.text)
	u.text, u.redo = u.redo[len(u.redo)-1], u.redo[:len(u.redo)-1]
	return true
}

// Undoer is a document with an undo history of its own: the native
// backends. The shell binds Ctrl/Cmd+Z, Shift+Z and Y to it.
type Undoer interface {
	Document
	Undo() bool
	Redo() bool
}

// FileDocument is a document backed by a file on disk with its own undo
// stack: the native backend. Edit changes the text in memory (and can be
// undone); Save writes it.
type FileDocument struct {
	undoStack
	path string
	// disk is the text last read from or written to the file.
	disk []byte
}

// MemoryDocument is a document that exists only in memory: an untitled
// effect before Save as. It has the undo stack of a [FileDocument], no file
// and no host, and always counts as having unsaved edits.
type MemoryDocument struct{ undoStack }

// NewMemoryDocument returns a document starting with text.
func NewMemoryDocument(text []byte) *MemoryDocument {
	return &MemoryDocument{undoStack{text: slices.Clone(text)}}
}

// Dirty reports true: an untitled document is never saved.
func (d *MemoryDocument) Dirty() bool { return true }

// SaveAs returns the file document that continues this one at path, with the
// text, the undo history and the saved state of a document that was just
// written to disk.
func (d *MemoryDocument) SaveAs(path string) *FileDocument {
	return &FileDocument{undoStack: d.undoStack, path: path, disk: d.text}
}

// OpenFileDocument reads the file at path.
func OpenFileDocument(path string) (*FileDocument, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("toolshell: %w", err)
	}
	return &FileDocument{undoStack: undoStack{text: data}, path: path, disk: data}, nil
}

// NewFileDocument returns a document for a file that does not exist yet,
// starting with text. It is dirty until Save creates the file.
func NewFileDocument(path string, text []byte) *FileDocument {
	return &FileDocument{undoStack: undoStack{text: slices.Clone(text)}, path: path}
}

// Dirty reports whether the text differs from the file's.
func (d *FileDocument) Dirty() bool { return !bytes.Equal(d.text, d.disk) }

// Save writes the text to the file.
func (d *FileDocument) Save() error {
	if err := os.WriteFile(d.path, d.text, 0o644); err != nil {
		return fmt.Errorf("toolshell: %w", err)
	}
	d.disk = d.text
	return nil
}

// Path returns the file's path.
func (d *FileDocument) Path() string { return d.path }

// External says what a check of the file found.
type External int

const (
	// ExternalNone means the file is as the document last saw it.
	ExternalNone External = iota
	// ExternalReloaded means the file changed and the document had no
	// unsaved edits: it now holds the file's text (the undo stacks are
	// dropped) and the tool should load it.
	ExternalReloaded
	// ExternalConflict means the file changed while the document has
	// unsaved edits: nothing is loaded. The shell reports it in the panel.
	ExternalConflict
)

// CheckDisk compares the file with what the document last read or wrote.
func (d *FileDocument) CheckDisk() (External, error) {
	data, err := os.ReadFile(d.path)
	if errors.Is(err, fs.ErrNotExist) && d.disk == nil {
		return ExternalNone, nil // a new document, not saved yet
	}
	if err != nil {
		return ExternalNone, fmt.Errorf("toolshell: %w", err)
	}
	if bytes.Equal(data, d.disk) {
		return ExternalNone, nil
	}
	if d.Dirty() {
		return ExternalConflict, nil
	}
	d.undoStack, d.disk = undoStack{text: data}, data
	return ExternalReloaded, nil
}

// HostDocument is the document of the host page (VSCode): the extension owns
// the text and its undo. Edit sends an edit message; the host answers with a
// document message, which [HostDocument.Receive] filters for the echo of the
// last edit.
type HostDocument struct {
	t       Transport
	text    []byte
	last    []byte
	hasLast bool
}

// NewHostDocument returns a document that sends its edits over t.
func NewHostDocument(t Transport) *HostDocument { return &HostDocument{t: t} }

// Text implements [Document]: the last text the host sent or the tool made.
func (d *HostDocument) Text() []byte { return d.text }

// Edit implements [Document]: it sends the text to the host.
func (d *HostDocument) Edit(text []byte) error {
	d.text = slices.Clone(text)
	d.last, d.hasLast = d.text, true
	send(d.t, editMsg{Type: "edit", Text: string(text)})
	return nil
}

// Receive takes a text the host sent. It reports whether the tool should
// load it: false for the echo of the last Edit, which the tool made itself.
func (d *HostDocument) Receive(text []byte) (load bool) {
	d.text = slices.Clone(text)
	if d.hasLast && bytes.Equal(text, d.last) {
		d.hasLast = false
		return false
	}
	d.hasLast = false
	return true
}
