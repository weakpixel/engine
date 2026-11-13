package gfx

import (
	"github.com/weakpixel/asset"
	"github.com/weakpixel/engine/xmath"
)

// Handles that components store instead of images, so they stay pointer-free.
// Each asset kind that game code touches has exactly one concrete handle type,
// defined here because components live here. The packages that own the asset
// values (render, anim, sound, prefab, speech) take these same types in their stores and Load
// helpers, so no call site converts between handle types.
//
// A handle is 8 bytes, comparable and pointer-free. The zero handle means
// "none" and draws the placeholder. IsZero and Index come from the embedded
// [asset.Ref].
type (
	// SpriteHandle refers to a render.Sprite. Load one with render.LoadSprite.
	SpriteHandle struct{ asset.Ref }
	// FontHandle refers to a render.Font. Load one with render.LoadFont.
	FontHandle struct{ asset.Ref }
	// ClipHandle refers to an anim.Clip. Load one with anim.LoadClip.
	ClipHandle struct{ asset.Ref }
	// ShaderHandle refers to a render.Shader. Load one with
	// render.LoadShader. The zero handle means "no shader".
	ShaderHandle struct{ asset.Ref }
	// SoundHandle refers to a sound.Sound, a short effect decoded to memory.
	// Load one with sound.LoadSound.
	SoundHandle struct{ asset.Ref }
	// MusicHandle refers to a sound.Music, a track streamed while it plays.
	// Load one with sound.LoadMusic.
	MusicHandle struct{ asset.Ref }
	// PrefabHandle refers to a prefab.Prefab. Load one with prefab.Load; it
	// is the same type as prefab.Handle.
	PrefabHandle struct{ asset.Ref }
	// EffectHandle refers to a particles.Effect, a particle effect loaded
	// from an *.fx.json file. Load one with particles.LoadEffect.
	EffectHandle struct{ asset.Ref }
	// VoiceHandle refers to a speech.Voice, a *.voice.json file or a
	// starter voice by name ("chirpy"). Load one with speech.LoadVoice.
	VoiceHandle struct{ asset.Ref }
	// DialogueHandle refers to a speech.Dialogue, a *.dialogue.json file.
	// Load one with speech.LoadDialogue.
	DialogueHandle struct{ asset.Ref }
)

// Vec2 is a 2D vector in world units or pixels, depending on context. It is
// an alias of [xmath.Vec2], which holds its methods (Add, Len, Lerp, ...),
// so components keep reading gfx.Vec2.
type Vec2 = xmath.Vec2

// Rect is an axis-aligned rectangle from Min (inclusive) to Max (exclusive).
// It is an alias of [xmath.Rect], which holds its methods (Contains, Grow, ...).
type Rect = xmath.Rect

// Color is a packed 0xRRGGBBAA color, non-premultiplied. It is a plain
// uint32 so components containing it serialize as a number.
//
// The zero value means "no tint" (opaque white), not transparent black, so
// that a zero-valued component draws normally. A fully transparent color is
// any value with a zero alpha byte but other bits set, such as RGBA(255, 255, 255, 0).
type Color uint32

// Common colors.
const (
	White Color = 0xffffffff
	Black Color = 0x000000ff
)

// RGBA packs 8-bit channels into a Color.
func RGBA(r, g, b, a uint8) Color {
	return Color(r)<<24 | Color(g)<<16 | Color(b)<<8 | Color(a)
}

// Floats returns the channels as non-premultiplied values in [0,1]. The zero
// Color is returned as opaque white.
func (c Color) Floats() (r, g, b, a float32) {
	if c == 0 {
		return 1, 1, 1, 1
	}
	return float32(c>>24&0xff) / 255, float32(c>>16&0xff) / 255,
		float32(c>>8&0xff) / 255, float32(c&0xff) / 255
}
