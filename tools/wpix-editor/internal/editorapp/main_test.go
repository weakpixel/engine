package editorapp

import (
	"os"
	"testing"

	"github.com/weakpixel/engine/editor/viewkit"
)

// TestMain fixes the display scale: the real lookup (ebiten.Monitor)
// initializes the windowing system, which a headless test run has no thread
// or display for.
func TestMain(m *testing.M) {
	viewkit.DeviceScale = func() float64 { return 1 }
	os.Exit(m.Run())
}
