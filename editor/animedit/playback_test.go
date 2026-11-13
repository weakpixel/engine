package animedit

import (
	"testing"
	"time"

	"github.com/weakpixel/engine/anim/animfile"
)

func timeline(loop animfile.LoopMode, ms ...int) *animfile.ResolvedClip {
	rc := &animfile.ResolvedClip{Loop: loop}
	for _, d := range ms {
		rc.Durations = append(rc.Durations, time.Duration(d)*time.Millisecond)
	}
	return rc
}

func TestPlayback(t *testing.T) {
	ms := time.Millisecond
	t.Run("loop wraps", func(t *testing.T) {
		c := Timeline(timeline(animfile.Loop, 100, 100, 100))
		p := Playback{Playing: true}
		p.Advance(c, 250*ms)
		if p.Frame != 2 || !p.Playing {
			t.Errorf("at 250ms: frame %d playing %v", p.Frame, p.Playing)
		}
		p.Advance(c, 100*ms)
		if p.Frame != 0 {
			t.Errorf("at 350ms: frame %d, want 0", p.Frame)
		}
	})
	t.Run("once stops on the last frame", func(t *testing.T) {
		c := Timeline(timeline(animfile.Once, 100, 100))
		p := Playback{Playing: true}
		p.Advance(c, 500*ms)
		if p.Frame != 1 || p.Playing {
			t.Errorf("frame %d playing %v, want 1 and stopped", p.Frame, p.Playing)
		}
	})
	t.Run("ping-pong goes back", func(t *testing.T) {
		c := Timeline(timeline(animfile.PingPong, 100, 100, 100))
		p := Playback{Playing: true}
		var seen []int
		for range 5 {
			p.Advance(c, 100*ms)
			seen = append(seen, p.Frame)
		}
		want := []int{1, 2, 1, 0, 1}
		for i := range want {
			if seen[i] != want[i] {
				t.Fatalf("frames = %v, want %v", seen, want)
			}
		}
	})
	t.Run("durations are the resolved ones", func(t *testing.T) {
		c := Timeline(timeline(animfile.Loop, 50, 300))
		p := Playback{Playing: true}
		p.Advance(c, 60*ms)
		if p.Frame != 1 {
			t.Errorf("frame = %d, want 1", p.Frame)
		}
	})
	t.Run("paused does not advance", func(t *testing.T) {
		c := Timeline(timeline(animfile.Loop, 100, 100))
		p := Playback{}
		p.Advance(c, time.Second)
		if p.Time != 0 || p.Frame != 0 {
			t.Errorf("paused moved: %+v", p)
		}
	})
	t.Run("step wraps and pauses", func(t *testing.T) {
		c := Timeline(timeline(animfile.Loop, 100, 100, 100))
		p := Playback{Playing: true}
		p.Step(c, -1)
		if p.Frame != 2 || p.Playing || p.Time != 200*ms {
			t.Errorf("step -1: %+v", p)
		}
		p.Step(c, 1)
		if p.Frame != 0 || p.Time != 0 {
			t.Errorf("step +1: %+v", p)
		}
	})
	t.Run("seek clamps", func(t *testing.T) {
		c := Timeline(timeline(animfile.Loop, 100, 100))
		p := Playback{}
		p.Seek(c, 9)
		if p.Frame != 1 {
			t.Errorf("seek 9: frame %d", p.Frame)
		}
		p.Seek(c, -3)
		if p.Frame != 0 || p.Time != 0 {
			t.Errorf("seek -3: %+v", p)
		}
	})
	t.Run("empty clip", func(t *testing.T) {
		p := Playback{Playing: true}
		c := Timeline(timeline(animfile.Loop))
		p.Advance(c, 100*ms)
		p.Step(c, 1)
		p.Seek(c, 3)
		if p.Frame != 0 {
			t.Errorf("frame = %d", p.Frame)
		}
	})
}
