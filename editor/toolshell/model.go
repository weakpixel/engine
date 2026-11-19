package toolshell

import "errors"

// ErrStale is returned by every edit while the document cannot be parsed:
// the tool keeps showing the last good version and never writes over the
// user's broken text.
var ErrStale = errors.New("the document has errors: fix it in the text editor first")

// ErrReadOnly is returned by every edit of a file the tool shows but does not
// write. A tool may return its own error that wraps it (errors.Is matches).
var ErrReadOnly = errors.New("the file is shown read-only")

// ErrNoDocument is returned by an edit before the first valid document
// arrived.
var ErrNoDocument = errors.New("nothing to edit: no valid document yet")

// DocStatus is the state every document tool keeps besides its file: whether
// a valid version was loaded, the parse error of the last text and whether
// edits are refused. A tool embeds it (directly or through [Model]) so the
// three checks and their errors are the same everywhere.
type DocStatus struct {
	// Loaded says that a valid document was loaded at least once.
	Loaded bool
	// Stale is the parse error of the last text, "" while the shown file is
	// the document. The last good version stays shown.
	Stale string
	// ReadOnly refuses every edit with [ErrReadOnly].
	ReadOnly bool
}

// MarkStale keeps the shown version and notes that the document does not
// parse.
func (d *DocStatus) MarkStale(err error) { d.Stale = err.Error() }

// Editable reports why an edit is refused: [ErrReadOnly], [ErrStale] or
// [ErrNoDocument], in that order; nil when it may go ahead.
func (d *DocStatus) Editable() error {
	switch {
	case d.ReadOnly:
		return ErrReadOnly
	case d.Stale != "":
		return ErrStale
	case !d.Loaded:
		return ErrNoDocument
	}
	return nil
}

// Model is the model of a document tool whose state is one decoded file F,
// usually a pointer to the format's struct: a change is a function on File,
// then Encode, then one Emit (the tool passes [Shell.Edit]), so undo, dirty
// state and the host's diff see the file as any other text. Decoding stays
// with the tool, which also loads what the file names; it calls [Model.SetFile]
// or [DocStatus.MarkStale] with the result.
type Model[F any] struct {
	DocStatus
	// File is the decoded file, the zero F until the first valid document.
	File F
	// Encode returns the canonical text of a file.
	Encode func(F) ([]byte, error)
	// Emit takes the text of every edit; nil drops it (tests, no document).
	Emit func(text []byte)
}

// SetFile replaces the file with a new document version and clears a stale
// mark.
func (m *Model[F]) SetFile(f F) {
	m.File, m.Loaded, m.Stale = f, true, ""
}

// Do runs edit on File when [DocStatus.Editable] allows it, then encodes the
// file and emits the text: one document edit. An edit that returns an error
// must leave File as it was; nothing is emitted then.
func (m *Model[F]) Do(edit func(F) error) error {
	if err := m.Editable(); err != nil {
		return err
	}
	if err := edit(m.File); err != nil {
		return err
	}
	data, err := m.Encode(m.File)
	if err != nil {
		return err
	}
	if m.Emit != nil {
		m.Emit(data)
	}
	return nil
}
