// +game:group:name=collision
//
// Package collision finds overlapping colliders: an entity with a
// gfx.Transform and a [Collider] (a shape, the layers it is on and the
// layers it reacts to) is tested against the others every tick. The
// [System] keeps the World's [Space]: it sends [Enter] and [Exit] events
// through the ECS event queue, keeps the current [Contact]s and answers
// point, rect and ray queries, also against tile grids ([TileGrid]).
//
// Detection only: nothing is moved. The broad phase ([Broadphase], a
// uniform [Grid] by default), the narrow phase ([Space.RegisterPair]) and
// the pair filter ([Config].Filter) can each be replaced or extended.
//
// The rule for who reacts: a collider detects another when its Mask has a
// layer the other is on, and each detecting side gets its own event, with
// itself as Self. See doc/guide/engine/collision.md.
//
// Package game never imports collision: add [NewSystem] to the stage that
// should run it.
package collision

//go:generate go run github.com/weakpixel/engine/tools/weakpixel gen
