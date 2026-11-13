//go:build ebitentest

package toolshell

import (
	"os"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// testGame runs the tests from inside Update, the only place where Ebiten
// lets a program read pixels back.
type testGame struct{ code int }

func (g *testGame) Update() error {
	g.code = testsMain.Run()
	return ebiten.Termination
}
func (g *testGame) Draw(*ebiten.Image)         {}
func (g *testGame) Layout(w, h int) (int, int) { return 64, 64 }

var testsMain *testing.M

func TestMain(m *testing.M) {
	testsMain = m
	g := &testGame{}
	ebiten.SetWindowSize(64, 64)
	if err := ebiten.RunGame(g); err != nil {
		panic(err)
	}
	os.Exit(g.code)
}
