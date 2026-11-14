package tilemap

import (
	"math"

	"github.com/weakpixel/engine/tiled"
)

// orient turns Tiled's flip flags into the rotation and mirroring of a
// render.Item drawn about the tile's center.
//
// Tiled transforms a tile by a transpose (FlipD: swap x and y), then FlipH
// (negate x), then FlipV (negate y). The renderer mirrors first and rotates
// second, so in item terms that is a rotation R and mirror flags F with
// R·F equal to Tiled's matrix. Without FlipD the answer is the mirror flags
// alone; with it, a quarter turn plus mirroring. A double mirror becomes a
// half turn, so the result never sets both flags. The hexagonal 120° flag is
// ignored. orient_test.go checks all eight cases by mapping the corners.
func orient(f tiled.Flip) (rot float32, flipX, flipY bool) {
	h, v := f&tiled.FlipH != 0, f&tiled.FlipV != 0
	if f&tiled.FlipD == 0 {
		rot, flipX, flipY = 0, h, v
	} else {
		// Tiled's matrix is [[0, sh], [sv, 0]] (sh = -1 with FlipH, sv = -1
		// with FlipV). A quarter turn [[0, -1], [1, 0]] times diag(fx, fy) is
		// [[0, -fy], [fx, 0]], so fx = sv and fy = -sh.
		rot, flipX, flipY = math.Pi/2, v, !h
	}
	if flipX && flipY {
		rot, flipX, flipY = rot+math.Pi, false, false
	}
	return rot, flipX, flipY
}
