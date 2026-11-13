// Package anim plays sprite animations from Aseprite exports.
//
// It has two halves. The data half is [Clip] and [ClipSet]: assets that
// [RegisterLoaders] makes loadable from an Aseprite "Sprite sheet" JSON
// export, one clip per tag. The runtime half is [AnimationSystem], which
// advances gfx.Animator components and writes the current frame into
// gfx.Sprite.ID. The renderer never sees animation, only the current sprite.
//
// The package makes no Ebiten calls: it reads render.Atlas values, so the
// sprites are already cut and cached by the render loaders.
package anim
