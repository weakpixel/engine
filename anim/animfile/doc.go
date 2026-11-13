// Package animfile reads and checks the file formats package anim loads,
// without Ebiten, the ECS or any GPU work: the Aseprite sprite sheet export
// ([DecodeSheet]) and the *.anim.json animation definition ([Decode],
// [Encode], [Resolve]).
//
// Package anim's loaders are thin adapters over this package, and
// `weakpixel anim` uses it to validate and create definitions on the command
// line, so the loader and the tool cannot disagree about what a valid file
// is. It imports the standard library only.
//
// The format reference is doc/guide/engine/animation.md.
package animfile
