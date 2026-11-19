package toolshell

import (
	"errors"
	"fmt"
	"testing"
)

type counter struct{ N int }

func newCounterModel() (*Model[*counter], *[]string) {
	var sent []string
	m := &Model[*counter]{
		Encode: func(c *counter) ([]byte, error) { return []byte(fmt.Sprint(c.N)), nil },
		Emit:   func(text []byte) { sent = append(sent, string(text)) },
	}
	return m, &sent
}

func TestModel(t *testing.T) {
	inc := func(c *counter) error { c.N++; return nil }
	refuse := func(*counter) error { return errors.New("no") }

	t.Run("no document yet", func(t *testing.T) {
		m, sent := newCounterModel()
		if err := m.Do(inc); !errors.Is(err, ErrNoDocument) || len(*sent) != 0 {
			t.Errorf("Do = %v, sent %q", err, *sent)
		}
	})
	t.Run("a good edit emits once", func(t *testing.T) {
		m, sent := newCounterModel()
		m.SetFile(&counter{N: 1})
		if err := m.Do(inc); err != nil || len(*sent) != 1 || (*sent)[0] != "2" {
			t.Errorf("Do = %v, sent %q", err, *sent)
		}
	})
	t.Run("a refused edit emits nothing", func(t *testing.T) {
		m, sent := newCounterModel()
		m.SetFile(&counter{})
		if err := m.Do(refuse); err == nil || len(*sent) != 0 {
			t.Errorf("Do = %v, sent %q", err, *sent)
		}
	})
	t.Run("stale keeps the file", func(t *testing.T) {
		m, sent := newCounterModel()
		good := &counter{N: 5}
		m.SetFile(good)
		m.MarkStale(errors.New("bad json"))
		if err := m.Do(inc); !errors.Is(err, ErrStale) || len(*sent) != 0 || m.File != good || good.N != 5 {
			t.Errorf("Do = %v, sent %q, file %+v", err, *sent, m.File)
		}
		if m.Stale != "bad json" {
			t.Errorf("Stale = %q", m.Stale)
		}
		m.SetFile(&counter{})
		if m.Stale != "" || m.Do(inc) != nil {
			t.Errorf("a new version did not clear the stale mark")
		}
	})
	t.Run("read-only refuses first", func(t *testing.T) {
		m, sent := newCounterModel()
		m.SetFile(&counter{})
		m.MarkStale(errors.New("bad"))
		m.ReadOnly = true
		if err := m.Do(inc); !errors.Is(err, ErrReadOnly) || len(*sent) != 0 {
			t.Errorf("Do = %v, sent %q", err, *sent)
		}
	})
}
