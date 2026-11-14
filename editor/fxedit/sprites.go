package fxedit

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"path"
	"slices"
	"strings"
	"sync"

	"github.com/weakpixel/engine/editor/toolshell"
	"github.com/weakpixel/engine/particles/fxfile"
	"github.com/weakpixel/engine/xmath"
)

// BuiltinPrefix starts the asset key of a built-in sprite. The sprites are
// code, not files: the tool serves them from the shell's overlay filesystem
// while editing, and Save as copies the one in use into the project. A saved
// effect never names this prefix.
const BuiltinPrefix = "fx-builtin/"

const spritePx = 64 // side of a built-in sprite

// builtinNames are the built-in sprites, in the order the picker lists them.
var builtinNames = []string{"soft-circle", "dot", "spark", "ring", "star", "square", "streak", "flake", "heart", "plus"}

// Keys of the built-in sprites.
const (
	SoftCircle = BuiltinPrefix + "soft-circle.png"
	Dot        = BuiltinPrefix + "dot.png"
	Spark      = BuiltinPrefix + "spark.png"
	Ring       = BuiltinPrefix + "ring.png"
	Star       = BuiltinPrefix + "star.png"
	Square     = BuiltinPrefix + "square.png"
	Streak     = BuiltinPrefix + "streak.png"
	Flake      = BuiltinPrefix + "flake.png"
	Heart      = BuiltinPrefix + "heart.png"
	Plus       = BuiltinPrefix + "plus.png"
)

var (
	builtinOnce sync.Once
	builtinPNG  map[string][]byte
)

// BuiltinKeys returns the asset keys of the built-in sprites.
func BuiltinKeys() []string {
	keys := make([]string, len(builtinNames))
	for i, n := range builtinNames {
		keys[i] = BuiltinPrefix + n + ".png"
	}
	return keys
}

// IsBuiltin reports whether key names a built-in sprite.
func IsBuiltin(key string) bool { return strings.HasPrefix(key, BuiltinPrefix) }

// BuiltinPNG returns the PNG bytes of the built-in sprite key.
func BuiltinPNG(key string) ([]byte, bool) {
	builtinOnce.Do(func() {
		builtinPNG = map[string][]byte{}
		for _, n := range builtinNames {
			builtinPNG[BuiltinPrefix+n+".png"] = encodeSprite(n)
		}
	})
	data, ok := builtinPNG[key]
	return data, ok
}

// encodeSprite draws the named sprite: white with an alpha shape, 64×64.
func encodeSprite(name string) []byte {
	img := image.NewNRGBA(image.Rect(0, 0, spritePx, spritePx))
	const c = (spritePx - 1) / 2.0
	for y := range spritePx {
		for x := range spritePx {
			dx, dy := (float64(x)-c)/(spritePx/2), (float64(y)-c)/(spritePx/2) // -1..1
			a := spriteAlpha(name, dx, dy)
			img.SetNRGBA(x, y, color.NRGBA{R: 255, G: 255, B: 255, A: uint8(xmath.Clamp(a, 0, 1)*255 + 0.5)})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		panic("fxedit: " + err.Error())
	}
	return buf.Bytes()
}

// edge turns a signed distance d (positive inside, in sprite units of -1..1)
// into alpha with about one pixel of antialiasing.
func edge(d float64) float64 { return xmath.Clamp(d*spritePx/2, 0, 1) }

// spriteAlpha is the alpha of the named sprite at (dx, dy), both -1..1 with
// Y down. Values outside 0..1 are clamped by the caller.
func spriteAlpha(name string, dx, dy float64) float64 {
	r := math.Hypot(dx, dy)
	ax, ay := math.Abs(dx), math.Abs(dy)
	switch name {
	case "dot":
		return edge(0.92 - r) // hard edge
	case "soft-circle":
		a := xmath.Clamp(1-r, 0, 1)
		return a * a
	case "spark":
		// A four-pointed star: the astroid |x|^½ + |y|^½ = 1, with a bright
		// core.
		v := math.Sqrt(ax) + math.Sqrt(ay)
		return xmath.Clamp((1-v)*3, 0, 1) + xmath.Clamp(1-r*2.5, 0, 1)
	case "ring":
		// A band around r = 0.8 with soft sides: shockwaves and bubbles. The
		// faint fill keeps a bubble readable on a busy background.
		band := xmath.Clamp(1-math.Abs(r-0.8)/0.12, 0, 1)
		return band*band + 0.08*edge(0.8-r)
	case "star":
		// A five-pointed star, one point up: the boundary radius runs from
		// the outer to the inner radius and back between the points.
		t := (math.Atan2(dy, dx) + math.Pi/2) * 5 / (2 * math.Pi)
		d := math.Abs(t-math.Floor(t)-0.5) * 2 // 1 at a point, 0 between two
		bound := 0.4 + 0.55*d*d
		return edge(bound - r)
	case "square":
		return edge(0.75 - max(ax, ay))
	case "streak":
		// A vertical line that tapers to both ends: rain and speed lines.
		w := 0.1 * (1 - dy*dy)
		return edge(w-ax) * xmath.Clamp(1-ay, 0, 1)
	case "flake":
		// Six arms with a pair of twigs on each and a small hexagonal core.
		a := edge(0.18 - r)
		for k := range 6 {
			s, c := math.Sincos(float64(k) * math.Pi / 3)
			along, across := dx*c+dy*s, math.Abs(-dx*s+dy*c)
			if along > 0 {
				a = max(a, edge(0.05-across)*edge(0.9-along))
				// Twigs leave the arm at 0.5 and point outward at ±60°.
				if t := along - 0.5; t > 0 && t < 0.3 {
					a = max(a, edge(0.05-math.Abs(across-t*math.Sqrt(3))))
				}
			}
		}
		return a
	case "heart":
		// The implicit heart (x²+y²-1)³ - x²y³ <= 0, flipped for Y down. Its
		// value is no distance, so the edge is 4×4 supersampled instead.
		const sub = 1.0 / spritePx // a quarter pixel in -1..1 units
		n := 0
		for j := range 4 {
			for i := range 4 {
				x := (dx + (float64(i)-1.5)*sub) * 1.2
				y := -(dy+(float64(j)-1.5)*sub)*1.2 + 0.2
				if math.Pow(x*x+y*y-1, 3)-x*x*y*y*y <= 0 {
					n++
				}
			}
		}
		return float64(n) / 16
	case "plus":
		return max(edge(0.22-ax)*edge(0.8-ay), edge(0.22-ay)*edge(0.8-ax))
	}
	return 0
}

// SaveFile is a file Save as writes next to the effect (a toolshell.CreateFile).
type SaveFile = toolshell.CreateFile

// PNGKey returns the asset key of the sprite file Save as writes for the
// effect at effectKey: the same folder and name, ".png".
func PNGKey(effectKey string) string {
	base := strings.TrimSuffix(path.Base(effectKey), fxfile.Suffix)
	return path.Join(path.Dir(effectKey), base+".png")
}

// ExportBuiltin prepares e for saving as the effect at effectKey. An emitter
// that uses a built-in sprite gets a copy of it next to the effect: the
// returned emitter names that file and the returned slice holds it. An
// emitter with a project sprite is returned as it is, with no files.
func ExportBuiltin(e Emitter, effectKey string) (Emitter, []SaveFile) {
	data, ok := BuiltinPNG(e.Sprite)
	if !ok {
		return e, nil
	}
	e.Sprite = PNGKey(effectKey)
	return e, []SaveFile{{Path: e.Sprite, Data: slices.Clone(data)}}
}

// ExportAll is [ExportBuiltin] for every emitter of an effect, for built-in
// sprites and shaders. The first built-in sprite becomes [PNGKey]; another
// one gets "-2", "-3" and so on before the extension. Built-in shaders are
// named the same way with ".kage". Emitters that share a built-in file share
// the copy.
func ExportAll(ems []Emitter, effectKey string) ([]Emitter, []SaveFile) {
	out := slices.Clone(ems)
	var files []SaveFile
	sprites := exporter{ext: ".png", data: BuiltinPNG}
	shaders := exporter{ext: ".kage", data: BuiltinShader}
	for i := range out {
		files = sprites.export(&out[i].Sprite, effectKey, files)
		files = shaders.export(&out[i].Shader, effectKey, files)
	}
	return out, files
}

// exporter copies the built-in files of one kind for [ExportAll].
type exporter struct {
	ext  string
	data func(key string) ([]byte, bool)
	done map[string]string // built-in key -> exported key
}

// export points *key at the project copy of a built-in file and appends the
// copy to files the first time it is needed. Other keys stay as they are.
func (x *exporter) export(key *string, effectKey string, files []SaveFile) []SaveFile {
	data, ok := x.data(*key)
	if !ok {
		return files
	}
	to, seen := x.done[*key]
	if !seen {
		base := strings.TrimSuffix(PNGKey(effectKey), ".png")
		to = base + x.ext
		if len(x.done) > 0 {
			to = fmt.Sprintf("%s-%d%s", base, len(x.done)+1, x.ext)
		}
		if x.done == nil {
			x.done = map[string]string{}
		}
		x.done[*key] = to
		files = append(files, SaveFile{Path: to, Data: slices.Clone(data)})
	}
	*key = to
	return files
}
