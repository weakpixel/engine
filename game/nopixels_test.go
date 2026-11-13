//go:build !ebitentest

package game

// pixelsEnabled is set when the tests run inside a real Ebiten game loop
// (-tags ebitentest), see main_pixels_test.go.
const pixelsEnabled = false
