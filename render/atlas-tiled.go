package render

import (
	"errors"
	"image"
	"strconv"

	"github.com/weakpixel/asset"
	"github.com/weakpixel/engine/gfx"
	"github.com/weakpixel/engine/tiled"
)

// decodeTiledTileset reads a single-image Tiled tileset (*.tsj) as an atlas:
// the image is cut by tilewidth/tileheight, margin, spacing and columns into
// tilecount frames named by local tile ID ("0", "1", ...), row by row.
//
// The tilemap package loads the same key as a tilemap.Tileset and takes its
// sprites from this atlas: a child type has one parent type (see
// asset.RegisterChild), so the frames must be published here.
func decodeTiledTileset(ctx *asset.DecodeContext, data []byte) (atlasSpec, error) {
	ts, err := tiled.DecodeTileset(data)
	if err != nil {
		return atlasSpec{}, err
	}
	if ts.IsCollection() {
		return atlasSpec{}, errors.New("collection tileset has no single image; load it as tilemap.Tileset")
	}
	frames := make([]frameSpec, ts.TileCount)
	for id := range frames {
		x := ts.Margin + id%ts.Columns*(ts.TileWidth+ts.Spacing)
		y := ts.Margin + id/ts.Columns*(ts.TileHeight+ts.Spacing)
		frames[id] = frameSpec{
			name:   strconv.Itoa(id),
			rect:   image.Rect(x, y, x+ts.TileWidth, y+ts.TileHeight),
			origin: defaultOrigin,
			size:   gfx.Vec2{X: float32(ts.TileWidth), Y: float32(ts.TileHeight)},
		}
	}
	return atlasSpec{texture: asset.Dep[Texture](ctx, ts.Image), frames: frames}, nil
}
