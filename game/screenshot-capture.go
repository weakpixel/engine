package game

import (
	"image"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/weakpixel/engine/input"
)

// ScreenshotConfig configures [Engine.Screenshot]. The zero value writes
// native captures into ./screenshots and downloads them in a browser.
type ScreenshotConfig struct {
	// Dir is the folder native captures without a ShotRequest.Path go to,
	// relative to the working directory and created on first use. Empty
	// means "screenshots".
	Dir string
	// Sink, if set, replaces the default sink (the file natively, the
	// download in a browser): it receives every successful capture, with
	// PNG set and Path empty, on the game goroutine at the start of a later
	// Update, before the request's Done. A returned error becomes the
	// result's Err. Editors use it to send captures to their host.
	Sink func(ShotResult) error
	// Key is a development hotkey: each press (an edge from up to down of
	// the raw key state) takes one capture of KeyTarget. The zero Key is
	// none, unless the WEAKPIXEL_SCREENSHOT_KEY environment variable names
	// a key; a Key set here wins over it. It reads the router's raw keys,
	// so it needs no input action, works in any game and also while a Tool
	// pauses the game (keys the tool's UI used never reach it).
	Key input.Key
	// KeyTarget is what the hotkey captures; the default is ShotGame.
	KeyTarget ShotTarget
}

// shots is the screenshot state of an Engine: the requests waiting for a
// frame, the reused capture images, and the results of the encoder
// goroutines waiting to be delivered on the game goroutine.
type shots struct {
	// queue waits for the next Draw after the startup screen; window holds
	// the ShotWindow requests of that Draw until DrawFinalScreen.
	queue  []ShotRequest
	window []ShotRequest
	// frame is the copy of the last captured logical frame, taken before
	// the stats overlay; frameTick and frameAt are its tick and time. win
	// is the window-sized target of ShotWindow. Both are reused while the
	// sizes stay the same.
	frame     *ebiten.Image
	frameTick uint64
	frameAt   time.Time
	win       *ebiten.Image

	// running counts the results not delivered yet (encoding or waiting),
	// so Update skips the lock on frames without captures. wg waits for the encoders
	// in Close.
	// env is the schedule of the WEAKPIXEL_SCREENSHOT_* variables; keyDown
	// is the hotkey's state in the last Update.
	env     shotEnv
	keyDown bool
	raw     input.Frame

	running atomic.Int32
	wg      sync.WaitGroup
	mu      sync.Mutex
	done    []shotDone
}

// shotDone is a finished capture waiting for delivery.
type shotDone struct {
	req ShotRequest
	res ShotResult
}

// Screenshot queues a capture of the next drawn frame; see [ShotRequest]
// for the options and doc/guide/engine/engine.md#screenshots for what a
// capture contains. The next Draw serves every queued request (while the
// startup screen is up, they wait); the PNG is encoded off the game
// goroutine and the result is delivered at the start of a later Update.
// It never panics: invalid requests are answered with ShotResult.Err.
// Call it on the game goroutine.
func (e *Engine) Screenshot(req ShotRequest) {
	e.shots.queue = append(e.shots.queue, req)
}

// SetScreenshotSink replaces Config.Screenshots.Sink; see there. Editor
// hosts call it to send captures to their host instead of writing files.
// nil restores the default sink. Call it on the game goroutine; captures
// already encoding use the sink set when they are delivered.
func (e *Engine) SetScreenshotSink(sink func(ShotResult) error) {
	e.cfg.Screenshots.Sink = sink
}

// hotkeyDown reports whether k is down. Normally that is the router's raw
// frame of this tick. While a Tool pauses the game the input system does
// not run, so the frame is stale: the source is polled directly instead
// (the router does not poll it in that tick, so nothing is polled twice).
// The device changes that poll reports are dropped, so a gamepad plugged
// in during a pause is noticed only after it is replugged.
func (e *Engine) hotkeyDown(k input.Key) bool {
	if !e.paused {
		return e.input.Frame().Keys.Has(k)
	}
	f := &e.shots.raw
	var ch input.DeviceChanges
	e.input.Source().Poll(f, &ch)
	return !f.Unfocused && f.Keys.Has(k)
}

// updateShots runs the hotkey and the WEAKPIXEL_SCREENSHOT_AT schedule at
// the end of an Update, after the input stage has refreshed the router
// (see hotkeyDown for a paused game). It costs a key test and a nil check on ordinary frames.
func (e *Engine) updateShots() {
	s := &e.shots
	if k := e.cfg.Screenshots.Key; k != 0 {
		down := e.hotkeyDown(k)
		if down && !s.keyDown {
			e.Screenshot(ShotRequest{Target: e.cfg.Screenshots.KeyTarget})
		}
		s.keyDown = down
	}
	if e.startup == nil && s.env.active() {
		s.env.tick(e)
	}
}

// captureFrame serves the queued requests from the frame Draw just drew
// into screen: ShotGame requests are read back now, ShotWindow ones wait
// for DrawFinalScreen. Draw calls it only when requests are queued.
func (e *Engine) captureFrame(screen *ebiten.Image) {
	s := &e.shots
	size := screen.Bounds().Size()
	s.frame = reuseImage(s.frame, size)
	s.frame.Clear()
	op := &ebiten.DrawImageOptions{Blend: ebiten.BlendCopy}
	s.frame.DrawImage(screen, op)
	s.frameTick = e.ticks
	s.frameAt = time.Now()

	limit := shotLimit()
	// One readback per scale: requests of one frame share it.
	var pix map[int][]byte
	for _, req := range s.queue {
		scale, err := shotScale(req)
		if err == nil && req.Target == ShotGame {
			err = checkShotSize(size.X*scale, size.Y*scale, limit)
		}
		switch {
		case err != nil:
			e.finishShot(req, ShotResult{Tick: s.frameTick, Err: err})
		case req.Target == ShotWindow:
			s.window = append(s.window, req)
		default:
			if pix == nil {
				pix = map[int][]byte{}
			}
			p, ok := pix[scale]
			if !ok {
				p = readScaled(s.frame, scale)
				pix[scale] = p
			}
			e.encodeShot(req, p, size.X*scale, size.Y*scale, s.frameTick, s.frameAt)
		}
	}
	clear(s.queue) // drop the Done closures
	s.queue = s.queue[:0]
}

// captureWindow serves the ShotWindow requests of the last captured frame:
// it runs the final pass a second time, from the frame copy (so without the
// stats overlay) into an image of the window's size, and reads that back.
// Tool.DrawFinal draws only on the real screen, so it is never captured.
func (e *Engine) captureWindow(bounds image.Rectangle, geoM ebiten.GeoM) {
	s := &e.shots
	size := bounds.Size()
	if err := checkShotSize(size.X, size.Y, shotLimit()); err != nil {
		for _, req := range s.window {
			e.finishShot(req, ShotResult{Tick: s.frameTick, Err: err})
		}
	} else {
		s.win = reuseImage(s.win, size)
		// Clear, so the letterbox bars are what the window shows: black
		// once the alpha is forced.
		s.win.Clear()
		e.Renderer().DrawFinal(s.win, s.frame, geoM)
		p := make([]byte, 4*size.X*size.Y)
		s.win.ReadPixels(p)
		for _, req := range s.window {
			e.encodeShot(req, p, size.X, size.Y, s.frameTick, s.frameAt)
		}
	}
	clear(s.window)
	s.window = s.window[:0]
}

// shotLimit is the largest capture side: maxShotSize, or Ebiten's device
// limit when that is lower.
func shotLimit() int {
	if m := ebiten.MaxImageSize(); m > 0 && m < maxShotSize {
		return m
	}
	return maxShotSize
}

// reuseImage returns img if it has the given size, otherwise a new image
// of that size (img is disposed).
func reuseImage(img *ebiten.Image, size image.Point) *ebiten.Image {
	if img != nil && img.Bounds().Size() == size {
		return img
	}
	if img != nil {
		img.Deallocate()
	}
	return ebiten.NewImage(size.X, size.Y)
}

// readScaled reads src back, enlarged by scale with nearest-neighbour
// filtering on the GPU first when scale > 1. ReadPixels stalls until the
// GPU has drawn the frame: the one cost a capture frame pays on the game
// goroutine.
func readScaled(src *ebiten.Image, scale int) []byte {
	size := src.Bounds().Size()
	if scale > 1 {
		dst := ebiten.NewImage(size.X*scale, size.Y*scale)
		defer dst.Deallocate()
		op := &ebiten.DrawImageOptions{Blend: ebiten.BlendCopy, Filter: ebiten.FilterNearest}
		op.GeoM.Scale(float64(scale), float64(scale))
		dst.DrawImage(src, op)
		src, size = dst, size.Mul(scale)
	}
	p := make([]byte, 4*size.X*size.Y)
	src.ReadPixels(p)
	return p
}

// encodeShot encodes pix on a goroutine of its own and, without a custom
// sink, writes it there with the default sink; the result waits for
// deliverShots. pix may be shared by several requests and is not modified.
func (e *Engine) encodeShot(req ShotRequest, pix []byte, w, h int, tick uint64, at time.Time) {
	s := &e.shots
	dir := e.cfg.Screenshots.Dir
	custom := e.cfg.Screenshots.Sink != nil
	s.running.Add(1)
	s.wg.Go(func() {
		res := ShotResult{Width: w, Height: h, Tick: tick}
		res.PNG, res.Err = encodeShot(pix, w, h, req.KeepAlpha)
		if res.Err == nil && !custom {
			res.Path, res.Err = defaultShotSink(dir, req, at, tick, res.PNG)
		}
		s.mu.Lock()
		s.done = append(s.done, shotDone{req, res})
		s.mu.Unlock()
	})
}

// finishShot queues a result for delivery without encoding (an invalid
// request).
func (e *Engine) finishShot(req ShotRequest, res ShotResult) {
	s := &e.shots
	s.running.Add(1)
	s.mu.Lock()
	s.done = append(s.done, shotDone{req, res})
	s.mu.Unlock()
}

// deliverShots hands the finished captures to the custom sink, if any, and
// to their Done callbacks (or the log), on the game goroutine. Update calls
// it first; without captures in flight it costs one atomic load.
func (e *Engine) deliverShots() {
	s := &e.shots
	if s.running.Load() == 0 {
		return
	}
	s.mu.Lock()
	done := s.done
	s.done = nil
	s.mu.Unlock()
	sink := e.cfg.Screenshots.Sink
	for _, d := range done {
		s.running.Add(-1)
		res := d.res
		res.ID = d.req.ID
		if res.Err == nil && sink != nil {
			res.Err = sink(res)
		}
		switch {
		case d.req.Done != nil:
			d.req.Done(res)
		case res.Err != nil:
			log.Print(res.Err)
		case res.Path != "":
			log.Printf("game: screenshot saved to %s", res.Path)
		}
	}
	e.shots.env.quitIfDone(e)
}

// closeShots waits for the encoders and delivers their results; Close
// calls it. Captures still queued are dropped: no frame is drawn anymore.
func (e *Engine) closeShots() {
	e.shots.wg.Wait()
	e.deliverShots()
}
