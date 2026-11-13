package game

import (
	"fmt"
	"log"
	"runtime"
	"slices"
	"strconv"
	"strings"

	"github.com/weakpixel/engine/input"
)

// Environment variables that take screenshots without code, next to
// WEAKPIXEL_FRAMELOG. See doc/guide/engine/engine.md#screenshots.
const (
	// shotKeyEnv names the hotkey ("F12"); Config.Screenshots.Key wins.
	shotKeyEnv = "WEAKPIXEL_SCREENSHOT_KEY"
	// shotAtEnv is a comma-separated list of ticks, counted from the first
	// tick after the startup screen, that each take a ShotGame capture.
	shotAtEnv = "WEAKPIXEL_SCREENSHOT_AT"
	// shotDirEnv overrides Config.Screenshots.Dir.
	shotDirEnv = "WEAKPIXEL_SCREENSHOT_DIR"
	// shotQuitEnv ("1") quits once the last WEAKPIXEL_SCREENSHOT_AT capture
	// was written.
	shotQuitEnv = "WEAKPIXEL_SCREENSHOT_QUIT"
)

// shotEnv is the WEAKPIXEL_SCREENSHOT_AT schedule: the ticks still to
// capture (ascending), the ticks counted so far and the captures not
// delivered yet. The zero value is inactive.
type shotEnv struct {
	at          []int
	n           int
	quit        bool
	outstanding int
}

func (s *shotEnv) active() bool { return len(s.at) > 0 }

// tick counts one tick after the startup screen and requests the capture
// that is due.
func (s *shotEnv) tick(e *Engine) {
	s.n++
	for len(s.at) > 0 && s.at[0] <= s.n {
		s.at = s.at[1:]
		s.outstanding++
		e.Screenshot(ShotRequest{Done: func(res ShotResult) {
			s.outstanding--
			switch {
			case res.Err != nil:
				log.Print(res.Err)
			case res.Path != "":
				log.Printf("game: screenshot saved to %s", res.Path)
			}
		}})
	}
}

// quitIfDone quits the engine once WEAKPIXEL_SCREENSHOT_QUIT is set and
// every scheduled capture was delivered.
func (s *shotEnv) quitIfDone(e *Engine) {
	if s.quit && s.n > 0 && len(s.at) == 0 && s.outstanding == 0 {
		e.Quit()
	}
}

// resolveShotEnv applies the WEAKPIXEL_SCREENSHOT_* variables (read with
// getenv) to cfg and returns the schedule. A bad value is logged and
// ignored: a development switch must not stop the game.
func resolveShotEnv(cfg ScreenshotConfig, getenv func(string) string) (ScreenshotConfig, shotEnv) {
	cfg, env, errs := parseShotEnv(cfg, getenv)
	for _, err := range errs {
		log.Print(err)
	}
	return cfg, env
}

// parseShotEnv is resolveShotEnv returning the problems instead of logging.
func parseShotEnv(cfg ScreenshotConfig, getenv func(string) string) (ScreenshotConfig, shotEnv, []error) {
	var env shotEnv
	var errs []error
	if v := getenv(shotKeyEnv); v != "" && cfg.Key == 0 {
		if k, ok := input.ParseKey(v); ok {
			cfg.Key = k
		} else {
			errs = append(errs, fmt.Errorf("game: %s: unknown key %q", shotKeyEnv, v))
		}
	}
	if v := getenv(shotDirEnv); v != "" {
		cfg.Dir = v
	}
	if v := getenv(shotAtEnv); v != "" {
		for f := range strings.SplitSeq(v, ",") {
			n, err := strconv.Atoi(strings.TrimSpace(f))
			if err != nil || n < 1 {
				errs = append(errs, fmt.Errorf("game: %s: %q is not a tick number (1 or more)", shotAtEnv, f))
				continue
			}
			env.at = append(env.at, n)
		}
		slices.Sort(env.at)
		env.at = slices.Compact(env.at)
	}
	env.quit = getenv(shotQuitEnv) == "1"
	return cfg, env, errs
}

// DevScreenshotKey is the hotkey development builds turn on with
// ScreenshotConfig.Key: F12, or F9 in js/wasm, because a browser keeps F12
// for its developer tools and the game never sees it.
func DevScreenshotKey() input.Key {
	if runtime.GOOS == "js" {
		return input.KeyF9
	}
	return input.KeyF12
}
