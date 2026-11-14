// Package particles loads particle effects (*.fx.json files), simulates them
// and draws them.
//
// An effect is an asset, [Effect], loaded with [LoadEffect] into the
// gfx.EffectHandle that a gfx.Particles component holds, as gfx.Animator
// holds a clip. Package fxfile (particles/fxfile) reads and writes the
// file without Ebiten; the file format is documented in
// doc/guide/engine/particles.md.
//
// The simulation has two halves. [System] spawns, moves and removes the
// particles of every entity that has a gfx.Transform and a gfx.Particles;
// [Extract] queues one render.KindSprite item per live particle, which the
// renderer batches into few draw calls. Particles are not entities: they
// live in pools owned by the System, one per entity, so a burst of thousands
// creates no archetype churn and no deferred deletes.
//
// The package makes no Ebiten calls. The simulation is deterministic: each
// pool owns a PCG random source seeded from gfx.Particles.Seed or the
// entity, so the same level and inputs give the same particles.
package particles
