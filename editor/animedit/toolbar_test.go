package animedit

import (
	"testing"

	"github.com/weakpixel/engine/editor/toolshell"
)

func TestToolButtonAt(t *testing.T) {
	r := newRig(t, heroDef)
	tool := r.tool
	const px = toolshell.ButtonPx
	// Five buttons centered on the 640 px window without panels.
	x0, pad := tool.bar.Origin(r.sh)
	if x0 != 320-px*5/2 {
		t.Fatalf("toolbar starts at x %v, want centered at %v", x0, 320-px*5/2)
	}
	for _, tc := range []struct {
		name string
		x, y float32
		want ToolKind
		ok   bool
	}{
		{"first", x0 + 1, pad + 1, ToolSelect, true},
		{"second", x0 + px, pad + 5, ToolPivot, true},
		{"last", x0 + px*5 - 1, pad + px - 1, ToolBody, true},
		{"left of bar", x0 - 1, pad + 1, 0, false},
		{"below bar", x0 + 1, pad + px, 0, false},
		{"viewport", 300, 300, 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := tool.toolButtonAt(r.sh, tc.x, tc.y)
			if ok != tc.ok || (ok && got != tc.want) {
				t.Fatalf("toolButtonAt(%v,%v) = %v, %v; want %v, %v", tc.x, tc.y, got, ok, tc.want, tc.ok)
			}
		})
	}
}

func TestToolIconsEmbedded(t *testing.T) {
	icons := toolshell.IconFS()
	for _, b := range toolButtons {
		if _, err := icons.ReadFile("icons/" + b.Icon + ".png"); err != nil {
			t.Errorf("%s: %v", b.Icon, err)
		}
	}
}
