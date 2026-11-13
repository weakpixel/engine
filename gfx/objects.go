package gfx

// Transform and Sprite are authored as level data, so they are Game Object
// components, and so are Animator, SoundEmitter and Particles (standalone components,
// see components.go).
// Label is a component of the Text object. DebugShape, ScreenSprite and ScreenNineSlice
// are runtime-only.

// Visual is a positioned, drawn thing: the minimal Game Object of the
// renderer. Game-specific objects embed it with +game:embed.
// +game:object
type Visual struct {
	// Where the entity is.
	// +game:component
	Transform *Transform
	// How the entity looks.
	// +game:component
	Sprite *Sprite
}

// Panel is a NineSlice placed by a Transform: the Game Object of a UI panel
// or frame that resizes without distorting its border.
// +game:object
type Panel struct {
	// Where the panel is.
	// +game:component
	Transform *Transform
	// The image, its insets and size.
	// +game:component
	NineSlice *NineSlice
}

// AnimatedVisual is a Visual that plays animation clips: the Game Object of
// an animated sprite. Animator stays a standalone component, so plain Visual
// objects (and their serialized data) are unchanged.
// +game:object
type AnimatedVisual struct {
	// The positioned sprite.
	// +game:embed
	Visual
	// Which clip plays and how fast.
	// +game:component
	Animator *Animator
}

// Text is a Label placed by a Transform: the Game Object of a text entity.
// The Label is drawn in screen space, so Transform.Pos is in logical pixels.
// +game:object
type Text struct {
	// Where the text is.
	// +game:component
	Transform *Transform
	// The string and how it is drawn.
	// +game:component
	Label *Label
}

// SoundSource is a sound placed in the world: a waterfall loop, a one-shot
// stinger at a spot. SoundEmitter stays a standalone component too, so other
// objects can carry one.
// +game:object
type SoundSource struct {
	// Where the sound is; it pans by this position.
	// +game:component
	Transform *Transform
	// What plays and how loud.
	// +game:component
	SoundEmitter *SoundEmitter
}

// ParticleSource is a particle effect placed in the world: dust, embers,
// sparks. Particles stays a standalone component too, so other objects
// and prefabs can carry one.
// +game:object
type ParticleSource struct {
	// Where the particles spawn.
	// +game:component
	Transform *Transform
	// The effect to play.
	// +game:component
	Particles *Particles
}
