//go:build !ebitentest

package editor

import (
	"os"
	"testing"
)

// TestMain fixes the display scale: the real lookup (ebiten.Monitor)
// initializes the windowing system, which a headless test run has no thread
// or display for. The ebitentest build does the same in main_pixels_test.go.
func TestMain(m *testing.M) {
	deviceScale = func() float64 { return 1 }
	os.Exit(m.Run())
}
