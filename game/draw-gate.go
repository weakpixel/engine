package game

// DrawGate is an optional interface of a [Tool] that decides which frames
// are drawn. Engine.Draw calls DrawFrame once per frame; when it returns
// false, Draw leaves the screen as it is, so Ebitengine keeps showing the
// last drawn frame and, after a few such frames, stops presenting and
// skips DrawFinalScreen too. A tool uses it to draw only when something
// changed, for example once per tick of a slow-ticking editor.
//
// The engine draws regardless of DrawFrame while the startup screen is up
// and while a capture ([Engine.Screenshot]) waits for a frame, and does
// not call it then.
//
// Skipping a frame only keeps the last image when the screen is not
// cleared between frames: whoever runs the game must call
// ebiten.SetScreenClearedEveryFrame(false). A drawn frame always fills the
// whole screen first, so that setting changes nothing else.
type DrawGate interface {
	// DrawFrame reports whether this frame is drawn. It is called from
	// Draw, on the game goroutine, and must not change the World.
	DrawFrame(e *Engine) bool
}

// skipDraw reports whether this Draw leaves the screen as it is: the tool
// is a [DrawGate] that says so, and nothing forces the frame.
func (e *Engine) skipDraw() bool {
	if e.gate == nil || e.startup != nil || len(e.shots.queue) > 0 || len(e.shots.window) > 0 {
		return false
	}
	return !e.gate.DrawFrame(e)
}
