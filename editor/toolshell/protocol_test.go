package toolshell

import (
	"slices"
	"testing"
	"time"
)

func TestProtocolReady(t *testing.T) {
	tr := &fakeTransport{}
	f := newFixture(t, Config{Transport: tr, Document: NewHostDocument(tr)})
	f.tick(time.Millisecond)
	if len(tr.msgs) != 1 {
		t.Fatalf("messages = %v, want only ready", tr.msgs)
	}
	m := tr.msgs[0]
	if m["type"] != "ready" || m["version"] != 0.0 || m["tool"] != "counter" || m["path"] != "doc.json" || m["document"] != true {
		t.Errorf("ready = %v", m)
	}
	if p, _ := m["panel"].(map[string]any); p["title"] != "Counter" {
		t.Errorf("ready panel = %v", m["panel"])
	}
}

func TestProtocolSetAnswersAtOnce(t *testing.T) {
	tr := &fakeTransport{}
	f := newFixture(t, Config{Transport: tr, Document: NewHostDocument(tr)})
	f.tick(time.Millisecond)
	tr.types()
	f.send(`{"type":"set","id":"n","value":5}`)
	f.tick(time.Millisecond) // well inside the throttle window
	got := tr.types()
	if !slices.Equal(got, []string{"edit", "panel"}) {
		t.Fatalf("messages = %v, want edit then panel", got)
	}
	// A set that changes nothing is still answered, so an input shows the
	// value the tool accepted.
	f.send(`{"type":"set","id":"n","value":5}`)
	f.tick(time.Millisecond)
	if got := tr.types(); !slices.Contains(got, "panel") {
		t.Errorf("messages after an unchanged set = %v, want a panel", got)
	}
	// A set the tool rejects reports an error.
	f.send(`{"type":"set","id":"bad","value":1}`)
	f.tick(time.Millisecond)
	if got := tr.types(); !slices.Equal(got, []string{"error", "panel"}) {
		t.Errorf("messages after a failed set = %v, want error then panel", got)
	}
}

func TestProtocolReadoutIsThrottled(t *testing.T) {
	tr := &fakeTransport{}
	f := newFixture(t, Config{Transport: tr})
	f.tick(time.Millisecond)
	tr.types()
	f.tool.readout = "1, 1"
	f.tick(10 * time.Millisecond)
	if got := tr.types(); len(got) != 0 {
		t.Errorf("a readout change 10 ms after ready sent %v", got)
	}
	f.tool.readout = "2, 2"
	f.tick(100 * time.Millisecond)
	if got := tr.types(); !slices.Equal(got, []string{"panel"}) {
		t.Errorf("a readout change after the throttle sent %v, want one panel", got)
	}
	// A structural change (a value) is not throttled.
	f.tool.n = 9
	f.tick(time.Millisecond)
	if got := tr.types(); !slices.Equal(got, []string{"panel"}) {
		t.Errorf("a value change sent %v, want one panel at once", got)
	}
	f.tick(time.Millisecond)
	if got := tr.types(); len(got) != 0 {
		t.Errorf("an unchanged panel sent %v", got)
	}
}

func TestProtocolDocument(t *testing.T) {
	tr := &fakeTransport{}
	f := newFixture(t, Config{Transport: tr, Document: NewHostDocument(tr)})
	f.tick(time.Millisecond)
	f.send(`{"type":"document","text":"{\"n\":3}"}`)
	f.tick(time.Millisecond)
	if !slices.Equal(f.tool.loads, []string{`{"n":3}`}) || f.tool.n != 3 {
		t.Fatalf("loads = %q, n = %v; want the host's text loaded", f.tool.loads, f.tool.n)
	}
	// The tool edits: edit goes out, and the host's echo is not loaded again.
	f.tool.shell.Edit([]byte(`{"n":4}`))
	f.send(`{"type":"document","text":"{\"n\":4}"}`)
	f.tick(time.Millisecond)
	if len(f.tool.loads) != 1 {
		t.Errorf("the echo of the tool's own edit was loaded: %q", f.tool.loads)
	}
	var edit map[string]any
	for _, m := range tr.msgs {
		if m["type"] == "edit" {
			edit = m
		}
	}
	if edit == nil || edit["text"] != `{"n":4}` {
		t.Errorf("edit message = %v", edit)
	}
	// An undo in the host sends an older text: that one loads.
	f.send(`{"type":"document","text":"{\"n\":3}"}`)
	f.tick(time.Millisecond)
	if len(f.tool.loads) != 2 || f.tool.n != 3 {
		t.Errorf("loads = %q, n = %v; want the undone text loaded", f.tool.loads, f.tool.n)
	}
}

func TestProtocolErrors(t *testing.T) {
	tr := &fakeTransport{}
	f := newFixture(t, Config{Transport: tr})
	f.tick(time.Millisecond)
	tr.types()
	f.send(`{"type":"nope"}`)
	f.send(`not json`)
	f.tick(time.Millisecond)
	if got := tr.types(); !slices.Equal(got, []string{"error", "error", "panel"}) {
		t.Errorf("messages = %v, want two errors and the panel with the status", got)
	}
}

// A document tool cannot switch files; the open message is an error and
// nothing is sent again.
func TestProtocolOpenRejectedForDocumentTool(t *testing.T) {
	tr := &fakeTransport{}
	f := newFixture(t, Config{Transport: tr, Document: NewHostDocument(tr)})
	f.tick(time.Millisecond)
	tr.types()
	f.send(`{"type":"open","path":"other.json"}`)
	f.tick(time.Millisecond)
	if len(tr.msgs) == 0 || tr.msgs[0]["type"] != "error" || tr.msgs[0]["message"] != "toolshell: open: counter cannot switch files" {
		t.Errorf("messages = %v, want the error", tr.msgs)
	}
	if f.s.Key() != "doc.json" {
		t.Errorf("key = %q, want doc.json kept", f.s.Key())
	}
}
