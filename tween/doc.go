// Package tween animates component fields over time: "move this enemy to
// (120, 40) with OutBack in 0.4 seconds", "pulse this coin forever", "ease
// the HP bar to the new value".
//
// Tweens are not components. A [Tweens] resource owns every running tween,
// in one typed pool per [Lens], and hands out an [ID] to control it. A Lens
// names the field a tween writes (Pos of gfx.Transform, Tint of gfx.Sprite,
// or a field of the game's own component) with typed functions, so there are
// no strings, no reflection and no unsafe, and the value type is checked at
// compile time. Any number of tweens can run on one entity.
//
//	tw := ecs.GetResource[tween.Tweens](ctx.World)
//	tween.To(tw, e, tween.Scale, gfx.Vec2{X: 1.3, Y: 1.3}, 0.1).Yoyo().Repeat(1)
//
// [System] advances the tweens once per tick; the game.Engine runs it at the
// start of its camera stage and publishes the Tweens as a resource. The
// package makes no Ebiten calls, uses no randomness and no wall clock, and
// allocates nothing per tick once its pools have grown.
//
// The guide is doc/guide/engine/tweens.md.
package tween
