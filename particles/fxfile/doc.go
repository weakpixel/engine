// Package fxfile reads, checks and writes the *.fx.json particle effect
// file, without Ebiten, the ECS runtime or any GPU work: the authored
// emitters ([Emitter], [Spec]), [Decode] and [Encode].
//
// Package particles' asset loader, the fx tool (editor/fxedit) and
// `wpix mcp` all use this package, so they cannot disagree about what a
// valid effect file is. It imports gfx (for the blend, shape, color
// and vector types the gfx.Particles overrides share) and xmath only.
//
// The format reference is doc/guide/engine/particles.md#the-effect-file.
package fxfile
