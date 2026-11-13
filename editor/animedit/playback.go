package animedit

import (
	"time"

	"github.com/weakpixel/engine/anim"
	"github.com/weakpixel/engine/anim/animfile"
)

// TickDuration is how much playback time one tool tick adds. The engine runs
// at 60 ticks per second.
const TickDuration = time.Second / 60

// Playback is the clock of the clip being shown. The shell ticks paused with
// no animation system, and the editor needs stepping and scrubbing, so it
// keeps its own time and asks anim.Clip.FrameAt for the frame: one
// implementation of loop, once and ping-pong for the game and the editor.
type Playback struct {
	Playing bool
	Time    time.Duration
	Frame   int
}

// Timeline builds the clip to play from the definition's resolved clip:
// frames with the final durations and its loop mode. Only the durations and
// the loop mode are set; sprites come from the sheet's own clip.
func Timeline(rc *animfile.ResolvedClip) *anim.Clip {
	c := &anim.Clip{Frames: make([]anim.Frame, len(rc.Durations)), Loop: loopMode(rc.Loop)}
	for i, d := range rc.Durations {
		c.Frames[i].Duration = d
		c.Total += d
	}
	return c
}

func loopMode(m animfile.LoopMode) anim.LoopMode {
	switch m {
	case animfile.Once:
		return anim.Once
	case animfile.PingPong:
		return anim.PingPong
	}
	return anim.Loop
}

// Advance moves time by dt while playing. A clip that does not loop stops on
// its last frame.
func (p *Playback) Advance(c *anim.Clip, dt time.Duration) {
	if !p.Playing {
		return
	}
	p.Time += dt
	f, done := c.FrameAt(p.Time)
	p.Frame = f
	if done {
		p.Playing = false
	}
}

// Seek shows frame (clamped to the clip) and sets the time to its start.
func (p *Playback) Seek(c *anim.Clip, frame int) {
	if len(c.Frames) == 0 {
		p.Time, p.Frame = 0, 0
		return
	}
	frame = max(0, min(frame, len(c.Frames)-1))
	p.Time = 0
	for i := 0; i < frame; i++ {
		p.Time += c.Frames[i].Duration
	}
	p.Frame = frame
}

// Step pauses and moves by delta frames, wrapping around the clip.
func (p *Playback) Step(c *anim.Clip, delta int) {
	p.Playing = false
	n := len(c.Frames)
	if n == 0 {
		return
	}
	p.Seek(c, ((p.Frame+delta)%n+n)%n)
}
