package camera_test

import (
	"fmt"
	"image"
	"time"

	"github.com/weakpixel/ecs"
	"github.com/weakpixel/ecs/system"
	"github.com/weakpixel/engine/camera"
	"github.com/weakpixel/engine/extract"
	"github.com/weakpixel/engine/gfx"
	"github.com/weakpixel/engine/render"
)

// Without game.Engine: the camera stage needs the camera snapshot before it
// (game.Engine does that in its snapshot stage) and the *render.Camera and
// *extract.Screen resources.
func ExampleSystem() {
	w := ecs.NewWorld()
	cam := &render.Camera{}
	ecs.SetResource(w, cam)
	ecs.SetResource(w, &extract.Screen{Size: image.Pt(320, 180)})
	ecs.SetResource(w, &camera.Rig{
		Mode:   camera.FollowSmooth,
		Bounds: gfx.Rect{Max: gfx.Vec2{X: 1000, Y: 1000}},
	})

	hero := ecs.NewEntity2(w, camera.Target{}, gfx.Transform{Pos: gfx.Vec2{X: 500, Y: 500}})

	var s system.Schedule
	s.AddStage("snapshot", extract.NewSnapshotTransforms(w).Run,
		func(*system.Context) { cam.PrevPos = cam.Pos })
	// ... input and simulation stages go here ...
	s.AddStage("camera")
	s.AddRunner(camera.NewSystem(w))

	s.Run(w, time.Second/60) // the first tick with a target snaps
	fmt.Println(cam.Pos)

	ecs.GetComp[gfx.Transform](w, hero).Pos = gfx.Vec2{X: 0, Y: 500}
	for range 120 {
		s.Run(w, time.Second/60)
	}
	fmt.Println(cam.Pos) // eased left, stopped by the bounds
	// Output:
	// {500 500}
	// {160 500}
}

// The presets cover the common cameras; the On flags add a stage to any of
// them. This one eases after the target but ignores small moves inside the
// dead zone, and leads it in its direction of motion.
func ExampleRig() {
	rig := camera.Rig{
		Mode:      camera.FollowSmooth,
		Smooth:    camera.Smooth{Time: 250 * time.Millisecond},
		DeadZone:  camera.DeadZone{On: true, HalfSize: gfx.Vec2{X: 24, Y: 16}},
		LookAhead: camera.LookAhead{On: true, Distance: 32},
		// Jump instead of sweeping when the target teleports.
		TeleportDistance: 400,
	}
	fmt.Println(rig.Mode)
	// Output: smooth
}
