package bridge

import (
	"strings"
	"testing"
)

// TestScreenshotBadTarget: a request with an unknown target is answered
// with its id and an error, and queues no capture.
func TestScreenshotBadTarget(t *testing.T) {
	f := newFixture(t)
	f.recv(`{"type":"screenshot","id":"s1","target":"moon"}`)
	got := f.host.take("screenshot")
	if len(got) != 1 || got[0]["id"] != "s1" || !strings.Contains(got[0]["error"].(string), `unknown target "moon"`) {
		t.Fatalf("answer = %v", got)
	}
	if got[0]["png"] != nil && got[0]["png"] != "" {
		t.Errorf("error answer carries a png: %v", got[0]["png"])
	}
	if errs := f.host.take("error"); len(errs) != 0 {
		t.Errorf("unexpected error messages %v", errs)
	}
}
