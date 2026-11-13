package tilemap

import "github.com/weakpixel/engine/gfx"

// TileLayer draws one tile or image layer of a map: [TilemapExtract] queues
// its visible tiles each tick. The entity's gfx.Transform.Pos is the world
// position of the map's top-left corner, so moving the Transform moves the
// layer; the Transform's rotation and scale are ignored.
//
// Create layers with [Spawn] or [SpawnWorld]. TileLayer is pointer-free and
// a plain component, not a Game Object component: maps are not GOD data.
type TileLayer struct {
	// Map is the map the layer belongs to. A map that is not Ready draws
	// nothing.
	Map MapHandle
	// Index is the layer's index in Map.Layers.
	Index int32
	// Layer is the render layer of the queued items.
	Layer int16
	// Hidden skips the layer. Spawn sets it for layers that are invisible in
	// Tiled; toggling it at run time is a field write, as for gfx.Sprite.
	Hidden bool
	// Chunked pre-renders the layer's static tiles into chunk images, so
	// that it queues one item per visible chunk (16x16 cells) instead of one
	// per visible tile; animated tiles are still queued one by one. Use it
	// for large layers whose cells do not change. It is ignored for image
	// layers, in a render layer configured with YSort (a chunk has one
	// SortY) and without a *render.Canvases resource; those draw tile by
	// tile. Spawn sets it from SpawnOptions.Chunked.
	Chunked bool
	// Tint multiplies the colors of the layer's tiles. Spawn sets it to the
	// layer's effective tint with its opacity folded into the alpha. Zero
	// means no tint.
	Tint gfx.Color
	// Shader draws the layer's tiles (or image) through a Kage shader, with
	// the same inputs as gfx.Sprite.Shader: the tile image as imageSrc0, Tint
	// as the color argument and Params as the "Params" vec4. Spawn sets it
	// from the layer property "shader" (see [PropShader]); assigning it at
	// run time works like any field write. The zero handle, and a shader that
	// is loading or failed, draw normally.
	//
	// An unchunked layer makes one shader draw per visible tile. A Chunked
	// layer shades each baked chunk image instead, one draw per visible chunk
	// and animated tile: the shader runs every frame (Time animates it) but
	// sees the chunk's pixels, so a shader that samples neighbouring texels
	// meets chunk borders. Prefer Chunked for shaded layers.
	Shader gfx.ShaderHandle
	// Params are the shader's Params uniform. Spawn sets them from the layer
	// property "shaderParams".
	Params [4]float32

	// follow, mapVersion, srcShader and srcParams let a layer made by Spawn
	// take the shader of a reloaded map (see syncShader). srcShader and
	// srcParams are the map values last copied in; a field game code has
	// changed since no longer equals them and is left alone.
	follow     bool
	mapVersion uint32
	srcShader  gfx.ShaderHandle
	srcParams  [4]float32
}
