package game

import (
	"bytes"
	_ "embed"
	"fmt"
	"image"
	"image/color"
	_ "image/jpeg" // StartupScreen.Path and the embedded default screen
	_ "image/png"  // StartupScreen.Path
	"io/fs"
	"sync"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
)

// StartupConfig configures the startup screen: one or more screens the
// engine shows from the first frame until the initial assets have loaded,
// each for at least a minimum time. Set Config.Startup to turn it on; the
// zero value shows [WeakpixelScreen] for at least 5 seconds. A [Tool] turns
// it off. See doc/guide/engine/engine.md#startup-screen.
type StartupConfig struct {
	// NoSkip ignores input: by default a key, mouse button, touch or pad
	// button ends the current screen early (never before the initial load
	// on the last screen).
	NoSkip bool
	// Screens are shown in order. Empty means one [WeakpixelScreen].
	Screens []StartupScreen
	// MinDuration is the minimum time on screen of every screen without its
	// own Duration. 0 means 5 seconds.
	MinDuration time.Duration
	// Ready is an extra condition for the end of the last screen, checked
	// once per tick after the initial load is done. nil means the initial
	// load alone decides.
	Ready func(*Engine) bool
}

// StartupScreen is one screen of the startup sequence. Exactly one of
// Image, Path and Draw must be set; [New] panics with
// "game: startup screen <i>: set one of Image, Path or Draw" otherwise.
type StartupScreen struct {
	// Image is the picture, scaled to fit the window (aspect kept, centred).
	Image image.Image
	// Path is a PNG or JPEG that New reads from Config.FS (the library's
	// files included) and shows like Image. A file that fails to read or
	// decode is reported through OnAssetError and the screen is skipped.
	Path string
	// Draw draws the screen itself into dst, which is window-sized, for
	// example a loading bar from s.Ready and s.Total. dst is cleared to
	// Background first; the engine applies the fade.
	Draw func(dst *ebiten.Image, s StartupState)
	// Duration is the minimum time on screen, fades included. 0 means
	// StartupConfig.MinDuration.
	Duration time.Duration
	// Background fills the window around the image. nil means black.
	Background color.Color
	// Fade is the length of the fade in and of the fade out, from and to
	// black, inside Duration. 0 means 250ms; negative means no fade.
	Fade time.Duration
}

// StartupState is what a [StartupScreen.Draw] function draws from.
type StartupState struct {
	// Screen is the index of the current screen in Screens, Screens their
	// number.
	Screen, Screens int
	// Elapsed is the time the current screen has been up; Duration is its
	// minimum time.
	Elapsed, Duration time.Duration
	// Ready and Total count the assets of the initial load (the global
	// scope and every level loaded so far): how many are ready and how
	// many there are.
	Ready, Total int
	// Alpha is the fade, 0 (black) to 1 (fully shown).
	Alpha float32
}

const (
	defaultStartupDuration = 5 * time.Second
	defaultStartupFade     = 250 * time.Millisecond
)

//go:embed startup/weakpixel-screen.jpg
var weakpixelScreenJPEG []byte

// weakpixelScreenImage decodes the embedded image once, on first use, so a
// game without the default screen never pays for decoding it.
var weakpixelScreenImage = sync.OnceValue(func() image.Image {
	img, _, err := image.Decode(bytes.NewReader(weakpixelScreenJPEG))
	if err != nil {
		panic("game: embedded startup screen: " + err.Error())
	}
	return img
})

// WeakpixelScreen returns the default startup screen: the Weakpixel Engine
// image on its own dark background. Use it to keep the default when adding
// screens:
//
//	Startup: &game.StartupConfig{Screens: []game.StartupScreen{{Image: studio}, game.WeakpixelScreen()}}
func WeakpixelScreen() StartupScreen {
	return StartupScreen{
		Image: weakpixelScreenImage(),
		// The image's edge colour, so the letterbox blends in.
		Background: color.RGBA{R: 1, G: 3, B: 8, A: 255},
	}
}

// startupSeq is the timing of the startup screens, free of Ebiten and of
// the engine so it can be tested on its own. Each screen fades in, holds
// and fades out; it starts leaving once Duration minus the fade has passed
// (or on a skip), and the last screen only once the initial load is done.
type startupSeq struct {
	durs, fades []time.Duration
	noSkip      bool

	i       int           // current screen
	elapsed time.Duration // on the current screen
	leaving bool          // fading out since leftAt
	leftAt  time.Duration
	skip    bool // a press asked to leave the current screen
	done    bool
}

// newStartupSeq returns the sequence for the given durations and fades,
// which must already have their defaults applied (fade 0 means none).
func newStartupSeq(durs, fades []time.Duration, noSkip bool) *startupSeq {
	return &startupSeq{durs: durs, fades: fades, noSkip: noSkip, done: len(durs) == 0}
}

// tick advances the sequence by dt. loaded reports whether the initial load
// is done; pressed whether the player pressed something this tick.
func (s *startupSeq) tick(dt time.Duration, loaded, pressed bool) {
	if s.done {
		return
	}
	s.elapsed += dt
	if pressed && !s.noSkip {
		s.skip = true
	}
	fade := s.fades[s.i]
	if !s.leaving && (s.i < len(s.durs)-1 || loaded) && (s.skip || s.elapsed >= s.durs[s.i]-fade) {
		s.leaving, s.leftAt = true, s.elapsed
	}
	if s.leaving && s.elapsed-s.leftAt >= fade {
		s.i++
		s.elapsed, s.leaving, s.skip = 0, false, false
		s.done = s.i == len(s.durs)
	}
}

// alpha is the fade of the current screen: rising during the fade in,
// falling from the moment it started leaving. A skip during the fade in
// fades out from where it is, without a jump.
func (s *startupSeq) alpha() float32 {
	if s.done {
		return 0
	}
	fade := s.fades[s.i]
	if fade <= 0 {
		return 1
	}
	a := min(float32(s.elapsed)/float32(fade), 1)
	if s.leaving {
		a = min(a, 1-float32(s.elapsed-s.leftAt)/float32(fade))
	}
	return max(a, 0)
}

// startupScreens validates the screens of c, loads the Path ones from fsys
// and applies the defaults. It returns the screens to show; screens whose
// Path failed are reported through report and left out.
func startupScreens(c StartupConfig, fsys fs.FS, report func(error)) []StartupScreen {
	in := c.Screens
	if len(in) == 0 {
		in = []StartupScreen{WeakpixelScreen()}
	}
	minDur := c.MinDuration
	if minDur <= 0 {
		minDur = defaultStartupDuration
	}
	out := make([]StartupScreen, 0, len(in))
	for i, sc := range in {
		n := 0
		for _, set := range []bool{sc.Image != nil, sc.Path != "", sc.Draw != nil} {
			if set {
				n++
			}
		}
		if n != 1 {
			panic(fmt.Sprintf("game: startup screen %d: set one of Image, Path or Draw", i))
		}
		if sc.Path != "" {
			img, err := decodeImage(fsys, sc.Path)
			if err != nil {
				report(fmt.Errorf("game: startup screen %d: %w", i, err))
				continue
			}
			sc.Image = img
		}
		if sc.Duration <= 0 {
			sc.Duration = minDur
		}
		switch {
		case sc.Fade == 0:
			sc.Fade = defaultStartupFade
		case sc.Fade < 0:
			sc.Fade = 0
		}
		if sc.Background == nil {
			sc.Background = color.Black
		}
		out = append(out, sc)
	}
	return out
}

// decodeImage reads and decodes a PNG or JPEG from fsys.
func decodeImage(fsys fs.FS, name string) (image.Image, error) {
	if fsys == nil {
		return nil, fmt.Errorf("%s: Config.FS is nil", name)
	}
	data, err := fs.ReadFile(fsys, name)
	if err != nil {
		return nil, err
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	return img, nil
}
