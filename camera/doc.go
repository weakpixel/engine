// Package camera is a quick-start follow camera: mark an entity with a
// [Target], pick a [FollowMode] in a [Rig] and run the [System] in the camera
// stage; it moves the render.Camera after the target every tick.
//
// A tick runs a small pipeline on the Rig's own (unclamped) follow position:
// the goal (the target's position plus Target.Offset) goes through the
// optional look-ahead and dead-zone stages, a motion (hard, smooth or
// spring) moves the follow position toward the result, and the camera's Pos
// becomes the follow position clamped to Rig.Bounds. Each FollowMode is a
// preset of that pipeline; Rig.DeadZone.On and Rig.LookAhead.On add a stage
// to any preset.
//
// game.Engine runs the System first in its camera stage and owns the Rig
// (Config.CameraRig, Engine.CameraRig). Without a game.Engine, add the
// System to a schedule after a step that copies Camera.PrevPos = Camera.Pos;
// see doc/guide/engine/camera.md.
//
// The package imports ecs, ecs/system, gfx, render and extract, never game.
package camera
