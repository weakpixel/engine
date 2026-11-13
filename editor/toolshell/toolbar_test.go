package toolshell

import (
	"testing"
	"time"

	"github.com/weakpixel/engine/input"
)

func TestToolbarAt(t *testing.T) {
	r := &fakeRenderer{left: 40, right: 120}
	f := newFixture(t, Config{Renderer: r})
	f.tick(time.Millisecond)
	bar := &Toolbar{Buttons: make([]ToolbarButton, 2)}
	// Free area 40..200: the 144 px row is centered at 120, from 48 to 192.
	if x, y := bar.Origin(f.s); x != 48 || y != BarPad {
		t.Fatalf("Origin = %v, %v; want 48, %v", x, y, BarPad)
	}
	for _, tc := range []struct {
		name string
		x, y float32
		want int
		ok   bool
	}{
		{"first", 49, BarPad + 1, 0, true},
		{"second", 48 + ButtonPx, BarPad + 5, 1, true},
		{"last pixel", 48 + 2*ButtonPx - 1, BarPad + ButtonPx - 1, 1, true},
		{"left of bar", 47, BarPad + 1, 0, false},
		{"right of bar", 48 + 2*ButtonPx, BarPad + 1, 0, false},
		{"below bar", 49, BarPad + ButtonPx, 0, false},
		{"old corner", BarPad + 1, BarPad + 1, 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := bar.At(f.s, tc.x, tc.y)
			if ok != tc.ok || (ok && got != tc.want) {
				t.Fatalf("At(%v,%v) = %v, %v; want %v, %v", tc.x, tc.y, got, ok, tc.want, tc.ok)
			}
		})
	}
	// Tab hides the panels: the row is centered on the whole window.
	f.src.Frame.Keys = keys(input.KeyTab)
	f.tick(time.Millisecond)
	if x, _ := bar.Origin(f.s); x != 160-ButtonPx {
		t.Errorf("Origin with hidden panels = %v, want %v", x, 160-ButtonPx)
	}
}
