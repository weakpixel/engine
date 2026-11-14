package tilemap

import (
	"image"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/weakpixel/engine/gfx"
)

// BenchmarkTilemapExtract measures one TilemapExtract.Run (queue reset
// included) on a 640x360 screen:
//
//   - level: a 40x40 map of 16 px tiles with 6 full layers at zoom 3, the
//     size of a typical level.
//   - large: a 256x256 map of 16 px tiles with 4 full layers at zoom 1, the
//     camera in the middle (about 41x23 cells per layer on screen).
//
// The -chunked variants spawn the layers with SpawnOptions.Chunked and
// measure the steady state, every visible chunk baked.
func BenchmarkTilemapExtract(b *testing.B) {
	for _, sc := range benchScenes {
		for _, chunked := range []bool{false, true} {
			name := sc.name
			if chunked {
				name += "-chunked"
			}
			b.Run(name, func(b *testing.B) {
				e := sc.env(b, chunked)
				benchRun(b, e)
			})
		}
	}
}

// benchScene is a map, a camera and a 640x360 screen for benchmarks.
type benchScene struct {
	name string
	env  func(b *testing.B, chunked bool) *xenv
}

var benchScenes = []benchScene{
	{"level", func(b *testing.B, chunked bool) *xenv {
		e := newXEnvFS(b, mapFiles(b, largeMap(40, 6)), image.Pt(640, 360), 3)
		e.spawn("large.tmj", SpawnOptions{Chunked: chunked})
		e.look(gfx.Vec2{X: 20 * 16, Y: 20 * 16})
		return e
	}},
	{"large", func(b *testing.B, chunked bool) *xenv {
		e := newXEnvFS(b, mapFiles(b, largeMap(256, 4)), image.Pt(640, 360), 1)
		e.spawn("large.tmj", SpawnOptions{Chunked: chunked})
		e.look(gfx.Vec2{X: 128 * 16, Y: 128 * 16})
		return e
	}},
}

// warmUp runs enough ticks for the queue to reach its steady size and for
// every visible chunk to be baked.
func (e *xenv) warmUp() {
	for range 16 {
		e.run(time.Millisecond)
	}
}

func benchRun(b *testing.B, e *xenv) {
	e.warmUp()
	b.ReportAllocs()
	for b.Loop() {
		e.run(16 * time.Millisecond)
	}
	b.ReportMetric(float64(e.queue.Len()), "items/op")
}

// largeMap is an n x n map of t16.tsj with the given number of tile layers,
// every cell set.
// mapFiles is the 16-tile tileset t16.tsj plus mapJSON as large.tmj.
func mapFiles(b *testing.B, mapJSON string) fstest.MapFS {
	files := tilesetFiles(b)
	files["t16.tsj"] = textFile(`{"name":"t16","image":"t16.png","tilewidth":16,"tileheight":16,"columns":4,"tilecount":16}`)
	files["t16.png"] = pngFile(b, 64, 64)
	files["large.tmj"] = textFile(mapJSON)
	return files
}

func largeMap(n, layers int) string {
	var data strings.Builder
	for i := range n * n {
		if i > 0 {
			data.WriteByte(',')
		}
		data.WriteString(strconv.Itoa(1 + i%16))
	}
	var sb strings.Builder
	sb.WriteString(`{"orientation":"orthogonal","width":` + strconv.Itoa(n) + `,"height":` + strconv.Itoa(n) +
		`,"tilewidth":16,"tileheight":16,"tilesets":[{"firstgid":1,"source":"t16.tsj"}],"layers":[`)
	for l := range layers {
		if l > 0 {
			sb.WriteByte(',')
		}
		sb.WriteString(`{"type":"tilelayer","name":"l` + strconv.Itoa(l) + `","width":` + strconv.Itoa(n) +
			`,"height":` + strconv.Itoa(n) + `,"data":[` + data.String() + `]}`)
	}
	sb.WriteString(`]}`)
	return sb.String()
}
