package bridge

import (
	"strings"
	"testing"

	"github.com/weakpixel/engine/editor"
)

// TestSettingsRate: the rate part changes only the rates it has; a bad one
// answers an error and changes nothing.
func TestSettingsRate(t *testing.T) {
	f := newFixture(t)
	if got := f.b.Rates(); got != editor.DefaultRates {
		t.Fatalf("rates %+v before settings, want the defaults", got)
	}
	f.recv(`{"type":"settings","rate":{"idle":5}}`)
	if got := f.b.Rates(); got != (editor.Rates{Idle: 5, Active: 60, Hidden: 0}) {
		t.Errorf("rates %+v after idle 5", got)
	}
	f.recv(`{"type":"settings","rate":{"active":30,"hidden":2}}`)
	if got := f.b.Rates(); got != (editor.Rates{Idle: 5, Active: 30, Hidden: 2}) {
		t.Errorf("rates %+v after active 30, hidden 2", got)
	}
	f.recv(`{"type":"settings","rate":{"active":0}}`)
	errs := f.host.take("error")
	if len(errs) != 1 || !strings.Contains(errs[0]["message"].(string), "bridge: settings: editor: rates") {
		t.Errorf("active 0 answered %v", errs)
	}
	if got := f.b.Rates(); got.Active != 30 {
		t.Errorf("a bad rate changed the rates to %+v", got)
	}
}

// TestVisibleMessage: visible hides and shows the view; hidden, no frame is
// drawn; status reports the rate and the drawn frames.
func TestVisibleMessage(t *testing.T) {
	f := newFixture(t)
	f.recv(`{"type":"visible","visible":false}`)
	if f.b.Visible() {
		t.Fatal("visible after visible:false")
	}
	if f.b.DrawFrame(f.e) {
		t.Error("a hidden view drew a frame")
	}
	f.recv(`{"type":"status"}`)
	st := f.host.take("status")[0]
	if st["tps"] != float64(0) {
		t.Errorf("hidden status tps %v, want 0", st["tps"])
	}
	f.recv(`{"type":"visible","visible":true}`)
	if !f.b.Visible() || !f.b.DrawFrame(f.e) {
		t.Error("visible again: no frame drawn after the tick")
	}
	f.recv(`{"type":"status"}`)
	st = f.host.take("status")[0]
	if st["tps"] != float64(60) || st["draws"] != float64(1) {
		t.Errorf("status %v, want tps 60 (shown is activity), draws 1", st)
	}
	f.recv(`{"type":"visible"}`)
	if errs := f.host.take("error"); len(errs) != 1 {
		t.Errorf("visible without the field answered %v", errs)
	}
}
