package game

import (
	"bytes"
	"fmt"
	"image"
	"image/png"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// ShotTarget selects what a screenshot captures. See [Engine.Screenshot].
type ShotTarget int

const (
	// ShotGame captures the logical frame at the game's resolution
	// (Config.Width x Config.Height): layer groups, post passes and
	// PostChain.Fade included, the final pass and the window scaling not.
	ShotGame ShotTarget = iota
	// ShotWindow captures the frame as the window shows it: window
	// resolution, with the post chain's final pass (or Ebiten's default
	// scaling) and the letterbox bars of IntegerScale.
	ShotWindow
)

// String returns "game" or "window", the names the editor protocols use.
func (t ShotTarget) String() string {
	switch t {
	case ShotGame:
		return "game"
	case ShotWindow:
		return "window"
	}
	return "ShotTarget(" + strconv.Itoa(int(t)) + ")"
}

// ShotRequest asks for one screenshot. The zero value captures the logical
// frame, opaque, into a new file in Config.Screenshots.Dir.
type ShotRequest struct {
	// Target is what to capture; the default is ShotGame.
	Target ShotTarget
	// Scale enlarges a ShotGame capture by a whole multiple with
	// nearest-neighbour filtering, 1..MaxShotScale; 0 means 1. Ignored for
	// ShotWindow.
	Scale int
	// KeepAlpha keeps the frame's alpha channel. By default the PNG is
	// opaque: the frame as the window shows it over black.
	KeepAlpha bool
	// Path is the file the default native sink writes (relative to the
	// working directory, its folder created, an existing file
	// overwritten). Empty means a new name in Config.Screenshots.Dir.
	// Ignored in a browser and when Config.Screenshots.Sink is set.
	Path string
	// ID is an opaque tag the engine echoes in ShotResult.ID, for hosts
	// that match answers to requests (the editor protocols use it).
	ID string
	// Done, if set, receives the result on the game goroutine, at the
	// start of a later Update (or in Close). Without Done the engine logs
	// the file written, or the error.
	Done func(ShotResult)
}

// ShotResult is the outcome of a [ShotRequest].
type ShotResult struct {
	// Path is the file the default native sink wrote; "" when no file was
	// written (an error, a browser download, a custom sink).
	Path string
	// PNG is the encoded image; nil on error.
	PNG []byte
	// ID is the request's ID; "" for captures the engine took itself
	// (hotkey, environment variables).
	ID string
	// Width and Height are the image's size in pixels.
	Width, Height int
	// Tick is the Engine tick (the number of Updates) the frame belongs to.
	Tick uint64
	// Err is set when the capture, the encode or the sink failed.
	Err error
}

// MaxShotScale is the largest ShotRequest.Scale.
const MaxShotScale = 8

// maxShotSize is the largest width or height of a capture. Ebiten's limit
// (ebiten.MaxImageSize) is device-dependent and can be lower; the capture
// uses the smaller of the two.
const maxShotSize = 4096

// defaultShotDir is the default of ScreenshotConfig.Dir.
const defaultShotDir = "screenshots"

// shotScale returns the effective scale of r, or an error for a scale
// outside 0..MaxShotScale or an unknown target.
func shotScale(r ShotRequest) (int, error) {
	switch r.Target {
	case ShotGame:
	case ShotWindow:
		return 1, nil
	default:
		return 0, fmt.Errorf("game: screenshot: unknown target %v", r.Target)
	}
	switch {
	case r.Scale == 0:
		return 1, nil
	case r.Scale < 0 || r.Scale > MaxShotScale:
		return 0, fmt.Errorf("game: screenshot: scale %d out of range 1..%d", r.Scale, MaxShotScale)
	}
	return r.Scale, nil
}

// checkShotSize reports an error when a w x h capture exceeds limit on a
// side.
func checkShotSize(w, h, limit int) error {
	if w <= 0 || h <= 0 {
		return fmt.Errorf("game: screenshot: empty frame %dx%d", w, h)
	}
	if w > limit || h > limit {
		return fmt.Errorf("game: screenshot: %dx%d exceeds the maximum image size %d", w, h, limit)
	}
	return nil
}

// shotName is the file name of a capture taken at time at (local time) of
// the frame of the given tick: screenshot-YYYYMMDD-HHMMSS-<tick>.png.
func shotName(at time.Time, tick uint64) string {
	return "screenshot-" + at.Format("20060102-150405") + "-" + strconv.FormatUint(tick, 10) + ".png"
}

// shotCandidate is the n-th name tried for name: name itself for n <= 1,
// otherwise name with "-<n>" before its extension.
func shotCandidate(name string, n int) string {
	if n <= 1 {
		return name
	}
	ext := filepath.Ext(name)
	return strings.TrimSuffix(name, ext) + "-" + strconv.Itoa(n) + ext
}

// encodeShot encodes w x h premultiplied RGBA pixels (the layout of
// ebiten.Image.ReadPixels and image.RGBA) as a PNG. Unless keepAlpha is
// set, alpha is forced to 255: with premultiplied colour that is the frame
// composited over black, what the window shows. pix is not modified.
func encodeShot(pix []byte, w, h int, keepAlpha bool) ([]byte, error) {
	if len(pix) != 4*w*h {
		return nil, fmt.Errorf("game: screenshot: %d bytes for %dx%d pixels", len(pix), w, h)
	}
	img := &image.RGBA{Pix: pix, Stride: 4 * w, Rect: image.Rect(0, 0, w, h)}
	if !keepAlpha {
		// A copy, because several requests of one frame share pix.
		img.Pix = make([]byte, len(pix))
		copy(img.Pix, pix)
		for i := 3; i < len(img.Pix); i += 4 {
			img.Pix[i] = 255
		}
	}
	var buf bytes.Buffer
	// BestSpeed: the encode runs on the only thread in js/wasm, and native
	// captures are taken interactively.
	enc := png.Encoder{CompressionLevel: png.BestSpeed}
	if err := enc.Encode(&buf, img); err != nil {
		return nil, fmt.Errorf("game: screenshot: %w", err)
	}
	return buf.Bytes(), nil
}
