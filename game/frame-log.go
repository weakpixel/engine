package game

import (
	"bufio"
	"fmt"
	"image"
	"os"
	"runtime/debug"
	"time"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/weakpixel/engine/gfx"
	"github.com/weakpixel/engine/render"
)

// frameLogEnv names the environment variable that enables the frame log
// without changing the game's code; Config.FrameLog wins over it.
const frameLogEnv = "WEAKPIXEL_FRAMELOG"

// frameLogFlushEvery is how many rows are written before the file is
// flushed, so that a crash loses at most this many frames.
const frameLogFlushEvery = 120

// frameLogHeader is the first line of the CSV. Times are milliseconds since
// the first Draw unless the column says otherwise.
const frameLogHeader = "frame,t_ms,draw_gap_ms,ticks,since_update_ms,alpha,update_ms,draw_ms," +
	"cam_x,cam_y,cam_prev_x,cam_prev_y,view_x,view_y,gc,fps,tps,origin_x,origin_y,scroll_x,scroll_y\n"

// frameLog records one CSV row per Draw to find stutter: uneven draw gaps,
// frames with zero or several ticks, late updates, a jumping alpha, GC
// pauses. It is a debug tool: it allocates and does file I/O, and is nil
// (every method a no-op) unless enabled.
type frameLog struct {
	f *os.File
	w *bufio.Writer

	start    time.Time
	lastDraw time.Time
	frame    int
	// ticks and updateDur are filled by Update and consumed by the next Draw.
	ticks     int
	updateDur time.Duration
	// origin is where the last row drew the world origin on screen, for
	// the scroll columns; hasOrigin is false before the first row.
	origin    gfx.Vec2
	hasOrigin bool
	err       error
}

// newFrameLog opens the log file at path, or returns nil for an empty path.
func newFrameLog(path string) (*frameLog, error) {
	if path == "" {
		return nil, nil
	}
	f, err := os.Create(path)
	if err != nil {
		return nil, err
	}
	l := &frameLog{f: f, w: bufio.NewWriter(f)}
	_, l.err = l.w.WriteString(frameLogHeader)
	return l, nil
}

// tick records one finished Update that took d.
func (l *frameLog) tick(d time.Duration) {
	if l == nil {
		return
	}
	l.ticks++
	l.updateDur = d
}

// draw writes the row of one Draw. now is the time Draw started, lastUpdate
// the end of the last Update, drawDur the CPU time Draw took, screen the
// logical screen size.
func (l *frameLog) draw(now, lastUpdate time.Time, alpha float64, cam *render.Camera, screen image.Point, drawDur time.Duration) {
	if l == nil || l.err != nil {
		return
	}
	if l.start.IsZero() {
		l.start, l.lastDraw = now, now
	}
	view := cam.At(float32(min(max(alpha, 0), 1)))
	// The world origin on screen, through the view as drawn (WorldToScreen
	// applies PixelSnap): its change between rows is how far the frame
	// scrolled, in logical pixels.
	origin := view.WorldToScreen(gfx.Vec2{}, screen)
	var scroll gfx.Vec2
	if l.hasOrigin {
		scroll = origin.Sub(l.origin)
	}
	l.origin, l.hasOrigin = origin, true
	var gc debug.GCStats
	debug.ReadGCStats(&gc)
	_, l.err = fmt.Fprintf(l.w, "%d,%.3f,%.3f,%d,%.3f,%.4f,%.3f,%.3f,%.3f,%.3f,%.3f,%.3f,%.3f,%.3f,%d,%.1f,%.1f,%.3f,%.3f,%.3f,%.3f\n",
		l.frame, ms(now.Sub(l.start)), ms(now.Sub(l.lastDraw)), l.ticks,
		ms(now.Sub(lastUpdate)), alpha, ms(l.updateDur), ms(drawDur),
		cam.Pos.X, cam.Pos.Y, cam.PrevPos.X, cam.PrevPos.Y, view.Pos.X, view.Pos.Y,
		gc.NumGC, ebiten.ActualFPS(), ebiten.ActualTPS(),
		origin.X, origin.Y, scroll.X, scroll.Y)
	l.frame++
	l.lastDraw, l.ticks = now, 0
	if l.frame%frameLogFlushEvery == 0 && l.err == nil {
		l.err = l.w.Flush()
	}
}

// close flushes and closes the file.
func (l *frameLog) close() error {
	if l == nil {
		return nil
	}
	err := l.err
	if ferr := l.w.Flush(); err == nil {
		err = ferr
	}
	if cerr := l.f.Close(); err == nil {
		err = cerr
	}
	return err
}

func ms(d time.Duration) float64 { return float64(d) / float64(time.Millisecond) }

// openFrameLog opens the log named by cfg.FrameLog or the environment.
func openFrameLog(cfg Config) (*frameLog, error) {
	path := cfg.FrameLog
	if path == "" {
		path = os.Getenv(frameLogEnv)
	}
	return newFrameLog(path)
}
