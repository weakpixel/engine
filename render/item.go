// Package render draws a queue of plain draw items with Ebiten. It does not
// import ecs: extract systems (package extract) fill the Queue during Update
// and the Renderer reads it in Draw, so Draw never touches the World.
//
// The package also holds the Ebiten asset types ([Texture], [Sprite], [Atlas],
// [FontSource], [Font], [Shader]) and their loaders for package asset:
// [RegisterLoaders] registers them on an asset.Server. Loaders create every
// Ebiten object in Finish on the game goroutine; Decode is CPU work only.
// Items hold handles to these assets and the Renderer resolves them through
// [Stores] in Draw, so a missing, loading or failed asset draws its
// placeholder instead of failing.
//
// Panics raised by this package are programmer errors and start with
// "render: ".
package render

import "github.com/weakpixel/engine/gfx"

// Space says what coordinates an Item is in.
type Space uint8

// Spaces.
const (
	// WorldSpace items are transformed by the camera.
	WorldSpace Space = iota
	// ScreenSpace items (HUD, UI) are in logical screen pixels and ignore
	// the camera.
	ScreenSpace
)

// Kind selects how an Item is drawn.
type Kind uint8

// Kinds.
const (
	KindSprite Kind = iota
	KindRect
	KindLine
	KindCircle
	KindText
	// KindCanvas draws Item.Canvas, an image pre-rendered with
	// [Canvases.Paint].
	KindCanvas
	// KindNineSlice draws Item.Sprite as a nine-slice panel of Item.Size,
	// cut by Item.Insets (see gfx.NineSlice).
	KindNineSlice
)

// Blend selects how an Item is composited onto what is below it.
type Blend uint8

// Blend modes.
const (
	BlendNormal Blend = iota
	BlendAdditive
	// BlendMultiply multiplies what is below by the source color: white
	// keeps it, black darkens it to black. Composite a light map group with
	// it.
	BlendMultiply
	// BlendScreen brightens like BlendAdditive but never exceeds white
	// (src + dst*(1-src)): soft glows, light shafts, flashes.
	BlendScreen
	// BlendSubtract subtracts the source color from what is below
	// (dst - src, alpha unchanged): darkening tints, negative light.
	BlendSubtract
	// BlendLighten keeps the brighter of source and destination per channel
	// (max): overlapping glows that do not add up.
	BlendLighten
	// BlendErase removes what is below by the source's alpha
	// (dst*(1-srcAlpha)). Use it for items inside a layer group, where it
	// cuts holes (fog of war, flashlight masks); on the screen it erases to
	// the window background.
	BlendErase
)

// Item is one draw command: a plain value written by extract systems and
// read by the Renderer. Fields that a Kind does not use are ignored.
type Item struct {
	Kind  Kind
	Space Space
	// Layer is the draw order: lower layers draw first.
	Layer int16
	// SortY orders items inside a layer configured with YSort.
	SortY float32
	// Insets are the borders of a KindNineSlice item in source pixels. They
	// sit here, next to the small fields, to keep Item from growing by
	// padding. For this kind Origin is the pivot as a fraction of Size, the
	// zero Origin being the top-left corner.
	Insets gfx.Insets

	// Sprite is the image for KindSprite. The zero handle draws the
	// placeholder.
	Sprite gfx.SpriteHandle
	// Pos is the position at the current tick, PrevPos at the previous one.
	// The Renderer blends them by the draw alpha. For ScreenSpace items they
	// are logical screen pixels.
	Pos, PrevPos gfx.Vec2
	// Rot is the rotation in radians.
	Rot float32
	// Scale multiplies the size. Extract resolves the "zero means 1" rule,
	// so the Renderer treats a zero Scale as zero.
	Scale gfx.Vec2
	// Origin is the pivot as a fraction of the sprite size. The zero Origin
	// means the sprite's own default pivot.
	Origin       gfx.Vec2
	FlipX, FlipY bool
	// Tile repeats the edges and center of a KindNineSlice item instead of
	// stretching them.
	Tile bool

	// Size is the rect size, the line end offset, in X the circle radius, or
	// the size of a KindNineSlice panel (zero is the image's own size).
	Size gfx.Vec2

	// Color tints sprites and colors shapes and text. The zero Color is
	// white (see gfx.Color).
	Color gfx.Color
	Blend Blend

	// Shader draws the item through a Kage shader instead of the default
	// pipeline. The zero handle, and a shader that is loading or failed,
	// draw the item normally. The shader gets the item's image as
	// imageSrc0, Color (premultiplied) as the color argument of Fragment,
	// and Params as the "Params" vec4 uniform (see [ShaderUniformParams]).
	//
	// For KindSprite and KindCanvas the image is the sprite's frame or the
	// canvas. A KindText, KindNineSlice, KindRect, KindLine or KindCircle
	// item is first drawn white, at scale 1 and unrotated, into an
	// offscreen image with a transparent margin of 4 pixels; the shader
	// gets that image, so it sees the whole text, panel or shape. Each such
	// item is a draw call of its own.
	Shader gfx.ShaderHandle
	// Params are per-item shader parameters (a flash amount, an outline
	// color, a dissolve threshold): what they mean is up to the shader.
	Params [4]float32
	// Aux is a second image for Shader (a noise texture, a mask, a
	// palette), which the shader reads as imageSrc1; without Shader it is
	// ignored. It may have any size and may be an atlas frame. The zero
	// handle, and a sprite that is loading or failed, give no image:
	// imageSrc1At returns transparent pixels, never the placeholder. Items
	// with different Aux images are different batches.
	Aux gfx.SpriteHandle

	// Canvas is the image of a KindCanvas item. Origin is the pivot as a
	// fraction of the canvas size, the zero Origin being the top-left
	// corner; Shader draws it through a shader like a sprite. A zero or stale ID, or a Renderer without
	// canvases (see [Renderer.SetCanvases]), draws nothing.
	Canvas CanvasID

	// Text and Font are used by KindText. The queue lives for one tick, so
	// holding a string here is fine.
	Text string
	Font gfx.FontHandle
}
