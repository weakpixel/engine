package ebiteninput

import (
	"testing"

	"github.com/weakpixel/engine/input"
)

// TestEveryKeyMapped checks that every input.Key has an Ebiten key of the
// same name, so no key is silently unreachable.
func TestEveryKeyMapped(t *testing.T) {
	for k := input.Key(1); int(k) <= input.KeyCount; k++ {
		if !inputKnown[k] {
			t.Errorf("input key %v has no Ebiten key", k)
		}
	}
}

// TestPadButtonsDistinct checks that the pad table maps every button to a
// different Ebiten standard button.
func TestPadButtonsDistinct(t *testing.T) {
	seen := map[int]input.PadButton{}
	for b, eb := range padButtons {
		if prev, ok := seen[int(eb)]; ok {
			t.Errorf("%v and %v both map to Ebiten button %d", prev, input.PadButton(b), eb)
		}
		seen[int(eb)] = input.PadButton(b)
	}
}

func TestDeviceOf(t *testing.T) {
	if DeviceOf(0) != input.FirstPadDevice || DeviceOf(3) != input.FirstPadDevice+3 {
		t.Fatal("DeviceOf")
	}
}
