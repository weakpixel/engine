package gfx

import "time"

// Transform places an entity in the world.
//
// The zero Scale means (1, 1), so a zero-valued Transform is valid.
type Transform struct {
	// Pos is the position in world units at the current tick.
	Pos Vec2
	// PrevPos is Pos at the start of the tick. The extract.SnapshotTransforms
	// system copies Pos here, and the renderer blends PrevPos to Pos so that
	// 60 TPS looks smooth on faster displays.
	PrevPos Vec2
	// Rot is the rotation in radians, clockwise on screen.
	//
	// +game:editor:unit=deg,step=5
	Rot float32
	// Scale multiplies the sprite size. Zero means (1, 1); the editor starts a
	// new Transform at (1, 1).
	//
	// +game:default:X=1,Y=1
	Scale Vec2
}

// Sprite draws an image at the entity's Transform.
type Sprite struct {
	// ID selects the image. Animation systems change it per frame. The zero
	// handle draws the placeholder. In the Game Object schema it is an asset
	// key ("chars/hero.aseprite.json#idle_0"); the handle itself is
	// runtime-only (json:"-"), and the resolver package converts the two.
	//
	// +game:asset:kind=sprite
	ID SpriteHandle `json:"-"`
	// Layer is the draw order: higher layers draw on top.
	Layer int16
	// Origin is the pivot as a fraction of the sprite size: (0.5,0.5) is the
	// center. The zero Origin means the sprite's own default pivot, so
	// top-left needs a tiny non-zero value such as (0.0001, 0.0001).
	Origin Vec2
	// FlipX mirrors the sprite horizontally.
	FlipX bool
	// FlipY mirrors the sprite vertically.
	FlipY bool
	// Tint multiplies the sprite's color. Zero means no tint.
	Tint Color
	// Blend is how the sprite is composited onto what is below it. The zero
	// value is BlendNormal. Give blended sprites a layer of their own so they
	// batch.
	Blend Blend
	// Shader draws the sprite through a Kage shader (hit flash, outline,
	// palette swap). The zero handle, and a shader that is still loading or
	// failed, draw the sprite normally. In the Game Object schema it is an
	// asset key ("fx/flash.kage"); the handle is runtime-only (json:"-").
	//
	// +game:asset:kind=shader
	Shader ShaderHandle `json:"-"`
	// Params are passed to Shader as its "Params" vec4 uniform; their meaning
	// is up to the shader. Animate them instead of swapping shaders: changing
	// a field does not move the entity between archetypes.
	Params [4]float32 `json:",omitzero"`
	// Hidden skips the sprite. It is a field and not a tag so that toggling
	// it does not move the entity between archetypes.
	Hidden bool
}

// Animator plays an animation clip on the entity's Sprite: the anim
// package's AnimationSystem advances Time and writes Sprite.ID each tick.
//
// Animator is a standalone Game Object component; objects opt in by marking
// a field of this type as a component. Clip and Speed are authored data (Clip
// is an asset key such as "chars/hero.aseprite.json#run"); Time, Frame and
// Done are runtime state, in the schema but marked x-runtime, so they are
// never serialized.
//
// Restart a clip by setting Clip to the new handle and zeroing Time, Frame
// and Done.
//
// +game:component
type Animator struct {
	// Clip selects the clip. The zero handle, and a clip that is not loaded
	// yet, leave the Sprite untouched.
	//
	// +game:asset:kind=clip
	Clip ClipHandle `json:"-"`
	// Time is how long the clip has played, scaled by Speed.
	//
	// +game:runtime
	Time time.Duration `json:"-"`
	// Frame is the index of the current frame in the clip, written by the
	// system so that gameplay can react to it.
	//
	// +game:runtime
	Frame int32 `json:"-"`
	// Speed scales the playback rate. Zero means 1; negative values are
	// treated as 1 too, because the clip's direction comes from the clip. The
	// editor starts a new Animator at 1.
	//
	// +game:default=1
	Speed float32
	// Done is set when a non-looping clip has reached its last frame. The
	// system then stops advancing the entity.
	//
	// +game:runtime
	Done bool `json:"-"`
}

// SoundEmitter plays a sound at the entity: the sound package's AudioSystem
// starts it once the sound is loaded. A one-shot emitter starts once, sets
// Done, and plays to its end even if the entity is deleted; a looping one
// plays for as long as the entity has the component, and stops when the
// entity is deleted, the component removed or Done set.
// If the entity has a Transform, the sound is panned (and, with a listener
// range, attenuated) by its position relative to the sound.Listener.
//
// Sound, Volume, Bus and Loop are authored data (Sound is an asset key such
// as "sfx/waterfall.ogg"); Voice and Done are runtime state, never
// serialized. Replay a one-shot emitter by zeroing Done.
//
// +game:component
type SoundEmitter struct {
	// Sound selects the effect. The zero handle plays nothing; a sound that
	// is still loading starts when it is ready, a failed one sets Done.
	//
	// +game:asset:kind=sound
	Sound SoundHandle `json:"-"`
	// Volume scales the sound. Zero means 1 (full volume), like
	// Animator.Speed; a negative value is silent. The editor starts a new
	// emitter at 1.
	//
	// +game:default=1
	Volume float32
	// Bus is the mixer bus (a sound.Bus) whose volume applies. Zero is
	// sound.BusSFX.
	Bus uint8
	// Loop repeats the sound while the emitter exists.
	Loop bool
	// Voice identifies the playing voice in the AudioSystem; zero means none.
	//
	// +game:runtime
	Voice uint32 `json:"-"`
	// Done is set when a one-shot sound has started (or the sound failed to
	// load). The system then ignores the emitter.
	//
	// +game:runtime
	Done bool `json:"-"`
}

// EmitShape selects the area a particle emitter spawns particles in.
//
// +game:enum
type EmitShape uint8

// Emit shapes.
const (
	// EmitPoint spawns every particle at the emitter's position.
	EmitPoint EmitShape = iota
	// EmitCircle spawns inside a disc of radius ShapeSize.X.
	EmitCircle
	// EmitRect spawns inside a rectangle of ShapeSize.X by ShapeSize.Y
	// centered on the emitter.
	EmitRect
)

// Blend selects how a drawable is composited onto what is below it. It
// mirrors render.Blend value for value (gfx cannot import render); extract
// systems convert with render.Blend(b).
//
// +game:enum
type Blend uint8

// Blend modes.
const (
	// BlendNormal draws the source over what is below (alpha blending).
	BlendNormal Blend = iota
	// BlendAdditive adds the source color to what is below: glows, fire,
	// sparks.
	BlendAdditive
	// BlendMultiply multiplies what is below by the source color: white
	// keeps it, black darkens it to black.
	BlendMultiply
	// BlendScreen brightens like BlendAdditive but never exceeds white:
	// soft glows, light shafts, flashes.
	BlendScreen
	// BlendSubtract subtracts the source color from what is below:
	// darkening tints, negative light.
	BlendSubtract
	// BlendLighten keeps the brighter of source and destination per
	// channel: overlapping glows that do not add up.
	BlendLighten
	// BlendErase removes what is below by the source's alpha. Use it inside
	// a layer group to cut holes (fog of war, flashlight masks).
	BlendErase
)

// Particles plays a particle effect at the entity's Transform: the particles
// package's System simulates it every tick and its Extract draws the
// particles as batched sprites. The effect itself (image, colors, rate,
// lifetimes, speeds, shape, gravity) is an asset, an *.fx.json file; the
// component holds its handle, as Animator holds a clip, plus the few things
// that differ per placed instance. Particles are not entities; they live in
// pools owned by the system, so thousands of them cost no archetype churn.
//
// The Transform's Rot turns the emission direction, the spawn shape and the
// particles' start rotation; its Scale stretches the spawn shape and the
// particle size. Both apply when a particle spawns, not to live particles.
// An effect that is still loading spawns nothing; editing its file restarts
// the particles of every entity that plays it.
//
// An effect may hold several emitters (up to fxfile.MaxEmitters, in package
// particles/fxfile); they play together, and Layer, Stop, Seed and the
// overrides below apply to all of them.
//
// Randomness is deterministic: the same Seed (or entity) and inputs give
// the same particles. Pool, Time, Live and Done are runtime state, never
// serialized. Restart a finished effect by zeroing Time and Done.
//
// +game:component
type Particles struct {
	// Effect is the particle effect to play. The zero handle plays nothing.
	//
	// +game:asset:kind=effect
	Effect EffectHandle `json:"-"`
	// Layer is the draw order, as for Sprite. Give additive particles their
	// own layer so they batch.
	Layer int16
	// Stop stops spawning; live particles finish their life.
	Stop bool
	// Seed seeds the random numbers. Zero derives it from the entity.
	Seed uint32

	// Sprite overrides the effect's particle image. The zero handle uses the
	// effect's own; an instance cannot override back to the placeholder.
	//
	// +game:asset:kind=sprite
	Sprite SpriteHandle `json:"-"`
	// Shader overrides the effect's shader. The zero handle uses the
	// effect's own; an instance cannot override to no shader.
	//
	// +game:asset:kind=shader
	Shader ShaderHandle `json:"-"`

	// The fields below override the effect value of the same name (see
	// fxfile.Emitter for what each does), in the component's units:
	// radians, packed colors, enum numbers. Nil means "from the effect", so a
	// level lists only what it overrides and every other value follows the
	// file, hot reloads included. The pointees are immutable: clones share
	// them, so replace a pointer (see [Override]) and never write through it.

	// Params override the shader's Params vec4.
	Params *[4]float32 `json:",omitzero"`
	// Blend overrides the blend mode.
	Blend *Blend `json:",omitzero"`
	// Color overrides the tint at birth.
	//
	// +game:editor:color=true
	Color *Color `json:",omitzero"`
	// EndColor overrides the tint at death.
	//
	// +game:editor:color=true
	EndColor *Color `json:",omitzero"`
	// Scale overrides the particle size at birth.
	Scale *float32 `json:",omitzero"`
	// EndScale overrides the size at death.
	EndScale *float32 `json:",omitzero"`
	// FadeIn overrides the fraction of the life over which alpha ramps up.
	//
	// +game:editor:min=0,max=1,step=0.05
	FadeIn *float32 `json:",omitzero"`
	// FadeOut overrides the fraction of the life over which alpha ramps down.
	//
	// +game:editor:min=0,max=1,step=0.05
	FadeOut *float32 `json:",omitzero"`
	// Rate overrides the particles spawned per second.
	//
	// +game:editor:unit=/s
	Rate *float32 `json:",omitzero"`
	// Burst overrides the particles spawned at the start of each cycle.
	Burst *uint16 `json:",omitzero"`
	// Duration overrides the cycle length in seconds; zero is forever.
	//
	// +game:editor:unit=s
	Duration *float32 `json:",omitzero"`
	// Loop overrides whether the cycle restarts after Duration.
	Loop *bool `json:",omitzero"`
	// Max overrides the pool capacity; zero means 256.
	Max *uint16 `json:",omitzero"`
	// Shape overrides the spawn area.
	Shape *EmitShape `json:",omitzero"`
	// ShapeSize overrides the spawn area's size.
	ShapeSize *Vec2 `json:",omitzero"`
	// LifeMin overrides the shortest lifetime in seconds.
	//
	// +game:editor:unit=s
	LifeMin *float32 `json:",omitzero"`
	// LifeMax overrides the longest lifetime in seconds.
	//
	// +game:editor:unit=s
	LifeMax *float32 `json:",omitzero"`
	// Angle overrides the emission direction.
	//
	// +game:editor:unit=deg,step=5
	Angle *float32 `json:",omitzero"`
	// Spread overrides the full cone width around Angle.
	//
	// +game:editor:unit=deg,step=5
	Spread *float32 `json:",omitzero"`
	// SpeedMin overrides the slowest initial speed in units/s.
	//
	// +game:editor:unit=u/s
	SpeedMin *float32 `json:",omitzero"`
	// SpeedMax overrides the fastest initial speed in units/s.
	//
	// +game:editor:unit=u/s
	SpeedMax *float32 `json:",omitzero"`
	// Gravity overrides the constant acceleration in units/s².
	Gravity *Vec2 `json:",omitzero"`
	// Drag overrides the slowdown factor per second.
	Drag *float32 `json:",omitzero"`
	// SpinMin overrides the slowest rotation speed.
	//
	// +game:editor:unit=deg,step=5
	SpinMin *float32 `json:",omitzero"`
	// SpinMax overrides the fastest rotation speed.
	//
	// +game:editor:unit=deg,step=5
	SpinMax *float32 `json:",omitzero"`
	// RandomRotation overrides whether particles start rotated at random.
	RandomRotation *bool `json:",omitzero"`
	// Local overrides whether particles move with the emitter.
	Local *bool `json:",omitzero"`

	// Pool identifies the particle pool in the system; zero means none.
	//
	// +game:runtime
	Pool uint32 `json:"-"`
	// Time is how long the current cycle of the effect's first emitter has
	// run, in seconds. Zeroing it restarts the cycle of every emitter.
	//
	// +game:runtime
	Time float32 `json:"-"`
	// Live is the number of live particles of all emitters, written by the
	// system for gameplay and the inspector.
	//
	// +game:runtime
	Live uint16 `json:"-"`
	// Done is set when every emitter of a non-looping effect has finished
	// and its last particle died.
	//
	// +game:runtime
	Done bool `json:"-"`
}

// ScreenSprite is a Sprite drawn in screen space (HUD, UI): Transform.Pos is
// in logical screen pixels and the camera is ignored.
type ScreenSprite struct {
	// Sprite is the image and its drawing options.
	Sprite Sprite
}

// Insets are the widths of the four borders of a nine-slice image, in pixels
// of the source image.
type Insets struct {
	Left, Top, Right, Bottom uint16
}

// NineSlice draws an image at any size without distorting its border: the
// four corners keep their pixel size, the four edges stretch along one axis
// and the center stretches along both (or repeats, with Tile). The same
// 24x24 panel image then serves a dialog, a button and a health-bar frame.
// Resize a panel by writing Size; that never moves the entity between
// archetypes.
//
// Zero values draw normally: a zero Size is the image's own size, a zero
// Origin the top-left corner, a zero Tint white and zero Insets stretch the
// whole image. Transform.Scale multiplies everything, corners included, and
// Transform.Rot rotates the panel around its origin. Insets that exceed the
// image are clamped; when Size is smaller than the borders together, the
// borders shrink in proportion and the center is empty.
//
// Insets are measured on the frame as stored: use untrimmed art for
// atlas frames.
//
// +game:component
type NineSlice struct {
	// ID selects the image. In the Game Object schema it is an asset key
	// ("ui/panel.png"); the handle is runtime-only (json:"-").
	//
	// +game:asset:kind=sprite
	ID SpriteHandle `json:"-"`
	// Insets are the border widths in source pixels.
	Insets Insets
	// Size is the drawn size in pixels before Transform.Scale. Zero means the
	// image's own size.
	Size Vec2
	// Layer is the draw order, as for Sprite.
	Layer int16
	// Origin is the pivot as a fraction of Size. The zero Origin is the
	// top-left corner.
	Origin Vec2
	// Tint multiplies the image color. Zero means no tint.
	Tint Color
	// Blend is how the panel is composited onto what is below it. Zero is
	// BlendNormal.
	Blend Blend
	// Tile repeats the edges and the center at the image's pixel size instead
	// of stretching them. The last repeat in a row or column is cropped.
	Tile bool
	// Hidden skips the panel. It is a field and not a tag so that toggling it
	// does not move the entity between archetypes.
	Hidden bool
}

// ScreenNineSlice is a NineSlice drawn in screen space (HUD, UI):
// Transform.Pos is in logical screen pixels and the camera is ignored.
type ScreenNineSlice struct {
	// NineSlice is the image and its drawing options.
	NineSlice NineSlice
}

// Label draws text at the entity's Transform in screen space.
type Label struct {
	// Text is the string to draw.
	Text string
	// Font selects the font face. In the Game Object schema it is an asset key
	// ("fonts/ui.json"); the handle is runtime-only (json:"-").
	//
	// +game:asset:kind=font
	Font FontHandle `json:"-"`
	// Layer is the draw order, as for Sprite.
	Layer int16
	// Color is the text color. Zero means white.
	Color Color
}

// ShapeKind selects what a DebugShape draws.
//
// +game:enum
type ShapeKind uint8

// Debug shape kinds.
const (
	// ShapeRect is a rectangle of Size from the Transform's position.
	ShapeRect ShapeKind = iota
	// ShapeCircle is a circle of radius Size.X around the position.
	ShapeCircle
	// ShapeLine is a line from the position to position + Size.
	ShapeLine
)

// DebugShape draws an outline-style primitive at the entity's Transform in
// world space, for collider and path visualization.
type DebugShape struct {
	// Kind selects the primitive.
	Kind ShapeKind
	// Size is the width/height of a rect, the radius (in X) of a circle, or
	// the end offset of a line.
	Size Vec2
	// Color is the line color. Zero means white.
	Color Color
}

// PlayerSlot says which input player controls the entity: the index of an
// input.Player (Router.Player). Game systems read the player's State through
// it, so the same controller system drives every player's character.
//
// +game:component
type PlayerSlot struct {
	// Player is the player index, 0..input.MaxPlayers-1.
	Player uint8
}

// Override returns a pointer to v for an optional override field of
// [Particles]: p.Rate = gfx.Override(float32(30)). The value is shared by
// clones and must not be written through.
func Override[T any](v T) *T { return &v }
