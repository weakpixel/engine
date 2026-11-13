package input

import "testing"

// busyRig is a router with every kind of input active: keys and modifiers,
// mouse, a pad with sticks and triggers, two touches, three mappings.
func busyRig(t testing.TB) *rig {
	rg := newRig(t, Options{})
	rg.p.Push("vehicle")
	rg.connectPad(FirstPadDevice, "Xbox Wireless Controller")
	rg.tick()
	f := &rg.src.Frame
	for _, k := range []Key{KeyW, KeyD, KeyControlLeft, KeyS, KeyQ, KeyC} {
		f.Keys.Set(k)
	}
	f.Mouse.Buttons = 1
	f.Pads[0].Axes = [4]float32{0.5, 0.5, 0.3, -0.7}
	f.Pads[0].Triggers = [2]float32{0.2, 0.8}
	f.Pads[0].SetDown(PadSouth, true)
	f.Touches[0] = TouchFrame{ID: 1, X: 50, Y: 150}
	f.Touches[1] = TouchFrame{ID: 2, X: 352, Y: 160}
	f.NTouches = 2
	return rg
}

func TestUpdateAllocs(t *testing.T) {
	rg := busyRig(t)
	rg.tick() // first sight of the touches sizes the region table
	f := &rg.src.Frame
	n := 0
	allocs := testing.AllocsPerRun(200, func() {
		n++
		f.Mouse.X = float32(n % 7) // the mouse moves: delta and active device change
		f.Touches[0].X = 50 + float32(n%9)
		rg.r.Update()
	})
	if allocs != 0 {
		t.Fatalf("Router.Update: %v allocs/op, want 0", allocs)
	}
}

func TestJoinUpdateAllocs(t *testing.T) {
	rg := newRig(t, Options{Players: 4, Assign: AssignJoin})
	rg.r.Lobby().Push("menu")
	rg.connectPad(FirstPadDevice, "pad")
	rg.tick()
	allocs := testing.AllocsPerRun(200, func() { rg.r.Update() })
	if allocs != 0 {
		t.Fatalf("Router.Update with AssignJoin: %v allocs/op, want 0", allocs)
	}
}

func TestStateQueryAllocs(t *testing.T) {
	rg := busyRig(t)
	rg.tick()
	st := rg.st
	var sink float32
	allocs := testing.AllocsPerRun(1000, func() {
		if st.Down(rg.jump) || st.JustPressed(rg.save) || st.JustReleased(rg.attack) {
			sink++
		}
		x, y := st.Vec2(rg.move)
		sink += x + y + st.Axis(rg.throttle) + st.Progress(rg.charge) + st.Strength(rg.jump) + float32(st.HeldTicks(rg.jump))
		sink += st.Pointer().X + float32(len(st.Touch()))
		if st.PressedWithin(rg.jump, 6) {
			sink++
		}
		rg.p.ConsumePress(rg.attack)
	})
	if allocs != 0 {
		t.Fatalf("State queries: %v allocs/op, want 0", allocs)
	}
	_ = sink
}

func BenchmarkRouterUpdate(b *testing.B) {
	rg := busyRig(b)
	rg.tick()
	b.ReportAllocs()
	for b.Loop() {
		rg.r.Update()
	}
}
