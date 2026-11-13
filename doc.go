// Package engine is the root of the Weakpixel Engine module: a small 2D game
// engine built on the ecs module and Ebiten. It holds no code; the Engine
// Runtime is the packages below it.
//
// Only the packages named below import Ebiten; the ecs packages never do.
// The pieces, in dependency order:
//
//   - gfx: plain-data components and value types. No Ebiten import; imports
//     only the asset module (for handle types).
//   - asset/library: the asset library layout over the asset module
//     (github.com/weakpixel/asset: stores, background loading, scopes).
//   - render: Ebiten renderer working on a queue of draw items, and the Ebiten
//     asset types and loaders (textures, atlases, fonts, shaders) for asset.
//     No ecs import.
//   - extract: systems that turn components into queue items during Update.
//   - input: actions, bindings files, players, rebinding. Stdlib only;
//     input/ebiteninput reads Ebiten, input/inputsys is the ECS glue.
//   - game: the Engine, which wires all of the above.
//
// See doc/guide/engine/getting-started.md for the Update/Draw contract.
package engine
