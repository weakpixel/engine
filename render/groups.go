package render

import (
	"cmp"
	"fmt"
	"image/color"
	"slices"

	"github.com/weakpixel/engine/gfx"
)

// GroupID identifies a layer group of a [GroupConfig]. It is the index
// returned by [GroupConfig.Add].
type GroupID int

// GroupOptions configures a layer group: the render layers From..To
// (inclusive) are drawn into an offscreen image instead of the screen, and
// that image is then composited onto what is below it, through Shader if one
// is set. A group is how a shader applies to a whole layer (water distortion,
// fog, a light map) instead of to one sprite.
type GroupOptions struct {
	// From and To are the first and last render layer of the group,
	// inclusive. A one-layer group has From == To.
	From, To int16
	// Shader composites the group image. The zero handle, and a shader that
	// is loading or failed, composite it unchanged. The shader gets the
	// group image as imageSrc0 (logical screen size, src in pixels),
	// Params as its "Params" vec4 uniform and, with SampleBelow, a copy of
	// what was drawn below the group as imageSrc1.
	Shader gfx.ShaderHandle
	// Params are passed to Shader as its "Params" vec4 uniform.
	Params [4]float32
	// Blend composites the group image onto what is below:
	// BlendNormal, BlendAdditive or BlendMultiply (light maps).
	Blend Blend
	// Clear is what the group image is cleared to every frame. The zero
	// value is transparent. A light map clears to the ambient light color.
	// A group whose Clear is not transparent is composited even when no
	// item falls into it, so the ambient light still applies.
	Clear color.RGBA
	// SampleBelow copies what is on the screen below the group, before the
	// group is composited, and passes the copy to Shader as imageSrc1 (for
	// refraction or water that distorts the scene behind it). It costs one
	// screen-sized copy per frame. Ignored without a shader.
	SampleBelow bool
	// NoComposite draws the group's layers into its image and never
	// composites it onto the scene, not even an empty group with a visible
	// Clear. The image is only seen by post passes that read it with
	// [TargetGroup] (masks, displacement maps, light maps to blur first).
	// Shader, Params, Blend and SampleBelow are ignored, and the group does
	// not count in [Stats].Groups. Without a pass that reads it the group
	// is drawn for nothing.
	NoComposite bool
}

// GroupConfig holds the layer groups of a Renderer (see
// [Renderer.SetGroups]). The zero value has no groups and is ready to use.
// Add groups before the first frame; Set may change a group's options at
// any time between frames, for example to animate Params.
type GroupConfig struct {
	groups []groupEntry
	// order holds group indices sorted by From. Draw walks it alongside the
	// sorted queue, so finding an item's group is a pointer bump, not a
	// lookup per item.
	order []GroupID
}

type groupEntry struct {
	opts GroupOptions
	// clear is opts.Clear converted to color.Color once, in Add or Set:
	// converting a color.RGBA to the interface on every frame's Fill would
	// allocate.
	clear color.Color
}

// Add adds a group and returns its ID.
//
// Panics with "render: group From > To" for an empty range and with
// "render: group layers overlap" when the range shares a layer with another
// group.
func (c *GroupConfig) Add(opts GroupOptions) GroupID {
	c.check(-1, opts)
	c.groups = append(c.groups, groupEntry{})
	id := GroupID(len(c.groups) - 1)
	c.groups[id] = groupEntry{opts: opts, clear: opts.Clear}
	c.order = append(c.order, id)
	c.sort()
	return id
}

// Set replaces the options of group id. Changing only Params, Shader, Blend
// or Clear does not allocate.
//
// Panics like [GroupConfig.Add], and with "render: unknown group" for an id
// that Add did not return.
func (c *GroupConfig) Set(id GroupID, opts GroupOptions) {
	c.entry(id)
	c.check(id, opts)
	e := &c.groups[id]
	moved := e.opts.From != opts.From
	if e.opts.Clear != opts.Clear {
		e.clear = opts.Clear
	}
	e.opts = opts
	if moved {
		c.sort()
	}
}

// Options returns the options of group id. Panics with "render: unknown
// group" for an id that Add did not return.
func (c *GroupConfig) Options(id GroupID) GroupOptions { return c.entry(id).opts }

// Len returns the number of groups.
func (c *GroupConfig) Len() int {
	if c == nil {
		return 0
	}
	return len(c.groups)
}

func (c *GroupConfig) entry(id GroupID) *groupEntry {
	if id < 0 || int(id) >= len(c.groups) {
		panic(fmt.Sprintf("render: unknown group %d", id))
	}
	return &c.groups[id]
}

// check validates opts for group self (-1 for a new group).
func (c *GroupConfig) check(self GroupID, opts GroupOptions) {
	if opts.From > opts.To {
		panic(fmt.Sprintf("render: group From > To (%d > %d)", opts.From, opts.To))
	}
	for i, g := range c.groups {
		if GroupID(i) == self {
			continue
		}
		if opts.From <= g.opts.To && g.opts.From <= opts.To {
			panic(fmt.Sprintf("render: group layers overlap (%d..%d and %d..%d)", opts.From, opts.To, g.opts.From, g.opts.To))
		}
	}
}

func (c *GroupConfig) sort() {
	slices.SortFunc(c.order, func(a, b GroupID) int {
		return cmp.Compare(c.groups[a].opts.From, c.groups[b].opts.From)
	})
}
