package viewkit

import (
	"os"
	"testing"
)

// TestMain fixes the display scale: the real lookup (ebiten.Monitor)
// initializes the windowing system, which a headless test run has no thread
// or display for.
func TestMain(m *testing.M) {
	DeviceScale = func() float64 { return 1 }
	os.Exit(m.Run())
}
