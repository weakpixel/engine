// Package anim implements `weakpixel anim`. It checks and creates *.anim.json animation definitions
// (doc/guide/engine/animation.md) without loading images or starting Ebiten.
//
// Usage:
//
//	weakpixel anim validate <file.anim.json>...
//	weakpixel anim create --file <sheet.json> [-o <out.anim.json>|-] [--force]
//
// validate checks each definition against its Aseprite sheet JSON with the
// same rules as the anim loader (package animfile). It prints one line per
// finding, "<file>: error|warn|info: ...", then a summary line. Tags the
// definition does not list are info, not errors.
//
// create writes a starter definition that lists every tag of the sheet as
// {"loop": true}, with defaultClip set to the first tag and no pivot or
// boxes. The default output is the sheet path with ".json" replaced by
// ".anim.json"; "-o -" writes to stdout. An existing file is only replaced
// with --force.
//
// Exit codes: 0 ok (warnings and info allowed), 1 errors, 2 usage error.
package anim
