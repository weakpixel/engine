package host

import (
	"errors"
	"time"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/weakpixel/engine/game"
)

// headlessSize is the window size a headless engine lays out for. Only the
// logical size matters (the engine's Width and Height); nothing is drawn.
const headlessSize = 1280

// RunHeadless runs e without a window: it lays the screen out once and calls
// Update at the engine's tick rate until the engine quits (the bridge quits
// it when the host closes the connection) or an Update fails. A tick that
// takes longer than the interval is not made up for. e has no Ebiten run
// loop, so nothing is drawn and [game.Engine.Screenshot] never answers.
// ebiten.Termination is the normal end and returns nil.
func RunHeadless(e *game.Engine) error {
	e.Layout(headlessSize, headlessSize*9/16)
	tps := ebiten.TPS()
	if tps <= 0 {
		tps = 60
	}
	tick := time.NewTicker(time.Second / time.Duration(tps))
	defer tick.Stop()
	for range tick.C {
		if err := e.Update(); err != nil {
			if errors.Is(err, ebiten.Termination) {
				return nil
			}
			return err
		}
	}
	return nil
}
