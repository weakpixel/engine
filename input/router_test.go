package input

import (
	"math"
	"testing"
	"time"
)

func near(a, b float32) bool { return math.Abs(float64(a-b)) < 1e-4 }

func TestButtonEdges(t *testing.T) {
	rg := newRig(t, Options{})
	rg.key(KeySpace, true)
	rg.tick()
	if !rg.st.Down(rg.jump) || !rg.st.JustPressed(rg.jump) || rg.st.HeldTicks(rg.jump) != 1 || rg.st.Strength(rg.jump) != 1 {
		t.Fatal("press tick")
	}
	rg.tick()
	if !rg.st.Down(rg.jump) || rg.st.JustPressed(rg.jump) || rg.st.HeldTicks(rg.jump) != 2 {
		t.Fatal("held tick")
	}
	rg.key(KeySpace, false)
	rg.tick()
	if rg.st.Down(rg.jump) || !rg.st.JustReleased(rg.jump) || rg.st.HeldTicks(rg.jump) != 0 {
		t.Fatal("release tick")
	}
	rg.tick()
	if rg.st.JustReleased(rg.jump) {
		t.Fatal("release edge lasted two ticks")
	}
}

func TestFreshPressAtStart(t *testing.T) {
	ta := newTestActions()
	src := &ManualSource{}
	src.Frame.Keys.Set(KeySpace) // held before the game started
	r := NewRouter(src, Options{Actions: ta.acts, Bindings: loadTestBindings(t, ta.acts)})
	r.Player(0).Push("gameplay")
	st := r.Player(0).State()
	r.Update()
	r.Update()
	if st.Down(ta.jump) {
		t.Fatal("a key held at start fired")
	}
	src.Frame.Keys.Clear(KeySpace)
	r.Update()
	src.Frame.Keys.Set(KeySpace)
	r.Update()
	if !st.JustPressed(ta.jump) {
		t.Fatal("fresh press after release did not fire")
	}
}

func TestStateKindPanics(t *testing.T) {
	rg := newRig(t, Options{})
	mustPanic(t, `input: action "jump" is Button, not Vec2`, func() { rg.st.Vec2(rg.jump) })
	mustPanic(t, `input: action "move" is Vec2, not Button`, func() { rg.st.Down(rg.move) })
	mustPanic(t, "input: unknown action 999", func() { rg.st.Down(999) })
	mustPanic(t, "input: player index out of range: 3", func() { rg.r.Player(3) })
}

func TestDpadNormalized(t *testing.T) {
	rg := newRig(t, Options{})
	rg.key(KeyW, true)
	rg.key(KeyD, true)
	rg.tick()
	x, y := rg.st.Vec2(rg.move)
	if !near(x, float32(math.Sqrt2/2)) || !near(y, -float32(math.Sqrt2/2)) {
		t.Fatalf("diagonal = %v,%v", x, y)
	}
	rg.key(KeyA, true) // left and right: cancel out
	rg.tick()
	if x, y = rg.st.Vec2(rg.move); x != 0 || y != -1 {
		t.Fatalf("A+D+W = %v,%v", x, y)
	}
}

func TestLastWins(t *testing.T) {
	ta := newTestActions()
	b, err := ParseBindings([]byte(`{"version":1,"mappings":{"m":{"bindings":{
	  "throttle":[{"composite":"axis","neg":"key:A","pos":"key:D","whichWins":"last"}]}}}}`), ta.acts)
	if err != nil {
		t.Fatal(err)
	}
	src := &ManualSource{}
	r := NewRouter(src, Options{Actions: ta.acts, Bindings: b})
	r.Player(0).Push("m")
	r.Update()
	st := r.Player(0).State()
	src.Frame.Keys.Set(KeyA)
	r.Update()
	src.Frame.Keys.Set(KeyD)
	r.Update()
	if st.Axis(ta.throttle) != 1 {
		t.Fatalf("D pressed last: %v", st.Axis(ta.throttle))
	}
	src.Frame.Keys.Clear(KeyA)
	r.Update()
	src.Frame.Keys.Set(KeyA)
	r.Update()
	if st.Axis(ta.throttle) != -1 {
		t.Fatalf("A pressed last: %v", st.Axis(ta.throttle))
	}
}

func TestModifierSpecificity(t *testing.T) {
	rg := newRig(t, Options{})
	rg.key(KeyControlLeft, true)
	rg.key(KeyS, true)
	rg.tick()
	if !rg.st.JustPressed(rg.save) {
		t.Fatal("Ctrl+S did not save")
	}
	if _, y := rg.st.Vec2(rg.move); y != 0 {
		t.Fatalf("Ctrl+S also moved: y=%v", y)
	}
	rg.key(KeyControlLeft, false)
	rg.tick()
	if _, y := rg.st.Vec2(rg.move); y != 1 {
		t.Fatalf("S alone: y=%v", y)
	}
	if rg.st.Down(rg.save) {
		t.Fatal("S alone saved")
	}
}

func TestStrictMods(t *testing.T) {
	ta := newTestActions()
	b, err := ParseBindings([]byte(`{"version":1,"mappings":{"m":{"strictMods":true,"bindings":{
	  "jump":[{"control":"key:J"}], "attack":[{"control":"key:ShiftLeft"}]}}}}`), ta.acts)
	if err != nil {
		t.Fatal(err)
	}
	src := &ManualSource{}
	r := NewRouter(src, Options{Actions: ta.acts, Bindings: b})
	r.Player(0).Push("m")
	r.Update()
	st := r.Player(0).State()
	src.Frame.Keys.Set(KeyShiftLeft)
	src.Frame.Keys.Set(KeyJ)
	r.Update()
	if st.Down(ta.jump) {
		t.Fatal("strictMods: Shift+J fired J")
	}
	if !st.Down(ta.attack) {
		t.Fatal("strictMods blocked a binding on Shift itself")
	}
}

func TestStickProcessors(t *testing.T) {
	rg := newRig(t, Options{})
	pad := rg.connectPad(FirstPadDevice, "Xbox Wireless Controller")
	pad.Axes = [4]float32{0.1, 0, 0, 0} // inside the 0.2 deadzone
	rg.tick()
	if x, y := rg.st.Vec2(rg.move); x != 0 || y != 0 {
		t.Fatalf("deadzone: %v,%v", x, y)
	}
	rg.src.Frame.Pads[0].Axes = [4]float32{0.6, 0, 0, 0}
	rg.tick()
	if x, _ := rg.st.Vec2(rg.move); !near(x, 0.5) {
		t.Fatalf("rescaled deadzone: %v, want 0.5", x)
	}
	// Profile deadzone replaces the file's.
	rg.p.Profile().Settings.Deadzone = 0.5
	rg.tick()
	if x, _ := rg.st.Vec2(rg.move); !near(x, 0.2) {
		t.Fatalf("profile deadzone: %v, want 0.2", x)
	}
	// Right stick: deadzone 0.15 then quadratic curve.
	rg.p.Profile().Settings.Deadzone = 0
	rg.src.Frame.Pads[0].Axes = [4]float32{0, 0, 0, 0.575}
	rg.tick()
	if _, y := rg.st.Vec2(rg.aim); !near(y, 0.25) {
		t.Fatalf("curve: %v, want 0.25", y)
	}
}

func TestSensitivityAndInvertY(t *testing.T) {
	rg := newRig(t, Options{})
	rg.p.Profile().Settings = Settings{Sensitivity: 2, InvertY: true}
	rg.src.Frame.Mouse.X, rg.src.Frame.Mouse.Y = 10, 10
	rg.tick()
	if x, y := rg.st.Vec2(rg.aim); x != 20 || y != -20 {
		t.Fatalf("aim = %v,%v, want 20,-20", x, y)
	}
	rg.tick()
	if x, y := rg.st.Vec2(rg.aim); x != 0 || y != 0 {
		t.Fatalf("no movement: %v,%v", x, y)
	}
}

func TestTriggerAsAxisAndThreshold(t *testing.T) {
	rg := newRig(t, Options{})
	pad := rg.connectPad(FirstPadDevice, "pad")
	pad.Triggers[1] = 0.3
	rg.tick()
	if v := rg.st.Axis(rg.throttle); !near(v, 0.3) {
		t.Fatalf("throttle = %v", v)
	}
	rg.key(KeyQ, true) // the keyboard axis is stronger: largest magnitude wins
	rg.tick()
	if v := rg.st.Axis(rg.throttle); v != -1 {
		t.Fatalf("throttle = %v, want -1", v)
	}
}

func TestInteractions(t *testing.T) {
	ta := newTestActions()
	b, err := ParseBindings([]byte(`{"version":1,"mappings":{"m":{"bindings":{
	  "jump":   [{"control":"key:J","interaction":{"tap":"100ms"}}],
	  "charge": [{"control":"key:J","interaction":{"hold":"100ms"}}],
	  "attack": [{"control":"key:A","interaction":{"doubleTap":"100ms"}}],
	  "brake":  [{"control":"key:B","interaction":{"release":true}}],
	  "pause":  [{"control":"key:P","interaction":{"repeat":{"delay":"100ms","every":"50ms"}}}]}}}}`), ta.acts)
	if err != nil {
		t.Fatal(err)
	}
	src := &ManualSource{}
	r := NewRouter(src, Options{Actions: ta.acts, Bindings: b, TPS: 60})
	r.Player(0).Push("m")
	r.Update()
	st := r.Player(0).State()
	step := func(k Key, down bool, n int) {
		if down {
			src.Frame.Keys.Set(k)
		} else {
			src.Frame.Keys.Clear(k)
		}
		for range n {
			r.Update()
		}
	}

	t.Run("tap", func(t *testing.T) {
		step(KeyJ, true, 3)
		if st.Down(ta.jump) {
			t.Fatal("tap fired while held")
		}
		step(KeyJ, false, 1)
		if !st.JustPressed(ta.jump) {
			t.Fatal("short press was not a tap")
		}
		step(KeyJ, false, 1)
		if st.Down(ta.jump) {
			t.Fatal("tap lasted two ticks")
		}
	})
	t.Run("hold", func(t *testing.T) {
		step(KeyJ, true, 3) // 100ms = 6 ticks
		if st.Down(ta.charge) || !near(st.Progress(ta.charge), 0.5) {
			t.Fatalf("hold early: down=%v progress=%v", st.Down(ta.charge), st.Progress(ta.charge))
		}
		step(KeyJ, true, 3)
		if !st.JustPressed(ta.charge) {
			t.Fatal("hold did not fire at 6 ticks")
		}
		step(KeyJ, false, 1)
		if st.Down(ta.jump) {
			t.Fatal("tap fired after a hold")
		}
	})
	t.Run("doubleTap", func(t *testing.T) {
		step(KeyA, true, 1)
		if st.Down(ta.attack) {
			t.Fatal("first tap fired")
		}
		step(KeyA, false, 2)
		step(KeyA, true, 1)
		if !st.JustPressed(ta.attack) {
			t.Fatal("double tap did not fire")
		}
		step(KeyA, false, 10)
		step(KeyA, true, 1)
		if st.Down(ta.attack) {
			t.Fatal("a third press fired again")
		}
		step(KeyA, false, 10)
	})
	t.Run("release", func(t *testing.T) {
		step(KeyB, true, 2)
		if st.Down(ta.brake) {
			t.Fatal("release fired on press")
		}
		step(KeyB, false, 1)
		if !st.JustPressed(ta.brake) {
			t.Fatal("release did not fire")
		}
	})
	t.Run("repeat", func(t *testing.T) {
		var pulses []int
		src.Frame.Keys.Set(KeyP)
		for i := range 14 {
			r.Update()
			if st.JustPressed(ta.pause) {
				pulses = append(pulses, i)
			}
		}
		// Press at 0, first repeat after 6 ticks, then every 3.
		want := []int{0, 6, 9, 12}
		if len(pulses) != len(want) {
			t.Fatalf("pulses at %v, want %v", pulses, want)
		}
		for i := range want {
			if pulses[i] != want[i] {
				t.Fatalf("pulses at %v, want %v", pulses, want)
			}
		}
	})
}

func TestMenuBlocksAndFreshPress(t *testing.T) {
	rg := newRig(t, Options{})
	rg.key(KeySpace, true)
	rg.tick()
	if !rg.st.Down(rg.jump) {
		t.Fatal("jump")
	}
	rg.p.Push("menu")
	rg.tick()
	if rg.st.Down(rg.jump) || !rg.st.JustReleased(rg.jump) {
		t.Fatal("blocking menu did not release jump")
	}
	if rg.st.Down(rg.uiConfirm) {
		t.Fatal("a key held when the menu opened confirmed")
	}
	rg.key(KeySpace, false)
	rg.tick()
	rg.key(KeySpace, true)
	rg.tick()
	if !rg.st.JustPressed(rg.uiConfirm) {
		t.Fatal("confirm after a fresh press")
	}
	// Closing the menu with the key that is also jump does not jump.
	rg.p.Pop("menu")
	rg.tick()
	if rg.st.Down(rg.jump) {
		t.Fatal("closing the menu jumped")
	}
	rg.key(KeySpace, false)
	rg.tick()
	rg.key(KeySpace, true)
	rg.tick()
	if !rg.st.JustPressed(rg.jump) {
		t.Fatal("jump after release")
	}
}

func TestVec2PassesThroughContextSwitch(t *testing.T) {
	rg := newRig(t, Options{})
	rg.key(KeyD, true)
	rg.p.Push("menu")
	rg.tick()
	if x, _ := rg.st.Vec2(rg.move); x != 0 {
		t.Fatal("menu did not block move")
	}
	rg.p.Pop("menu")
	rg.tick()
	if x, _ := rg.st.Vec2(rg.move); x != 1 {
		t.Fatal("held D did not walk after the menu closed")
	}
}

func TestPriorityConsumes(t *testing.T) {
	rg := newRig(t, Options{})
	rg.p.Push("vehicle")
	rg.tick()
	rg.key(KeySpace, true)
	rg.tick()
	if !rg.st.Down(rg.brake) || rg.st.Down(rg.jump) {
		t.Fatalf("vehicle should consume Space: brake=%v jump=%v", rg.st.Down(rg.brake), rg.st.Down(rg.jump))
	}
	rg.key(KeyEscape, true)
	rg.tick()
	if !rg.st.Down(rg.pause) {
		t.Fatal("pause should pass through the non-blocking vehicle mapping")
	}
}

func TestMenuRepeatGatesVec2(t *testing.T) {
	rg := newRig(t, Options{})
	rg.p.Push("menu")
	rg.tick()
	rg.key(KeyArrowDown, true)
	var hits []int
	for i := range 10 {
		rg.tick()
		if _, y := rg.st.Vec2(rg.uiMove); y != 0 {
			hits = append(hits, i)
		}
	}
	if len(hits) != 3 || hits[0] != 0 || hits[1] != 6 || hits[2] != 9 {
		t.Fatalf("ui.move pulses at %v, want [0 6 9]", hits)
	}
}

func TestFocusLossReleases(t *testing.T) {
	rg := newRig(t, Options{})
	rg.key(KeySpace, true)
	rg.tick()
	rg.src.Frame.Unfocused = true
	rg.tick()
	if rg.st.Down(rg.jump) || !rg.st.JustReleased(rg.jump) {
		t.Fatal("focus loss did not release")
	}
}

func TestUnknownMappingIsInactive(t *testing.T) {
	rg := newRig(t, Options{})
	rg.p.SetMappings("not-loaded-yet")
	rg.key(KeySpace, true)
	rg.tick()
	if rg.st.Down(rg.jump) {
		t.Fatal("an unknown mapping bound something")
	}
}

func TestHotPlugLostAndRestored(t *testing.T) {
	rg := newRig(t, Options{})
	rg.connectPad(FirstPadDevice, "Xbox Wireless Controller")
	rg.tick()
	ev := rg.r.Events()
	if len(ev.DevicesConnected) != 1 || ev.DevicesConnected[0].Family != FamilyXbox {
		t.Fatalf("connected = %+v", ev.DevicesConnected)
	}
	rg.disconnectPad(FirstPadDevice)
	rg.tick()
	if len(ev.DevicesLost) != 1 || ev.DevicesLost[0].Player != 0 {
		t.Fatalf("lost = %+v", ev.DevicesLost)
	}
	rg.connectPad(FirstPadDevice+1, "Xbox Wireless Controller") // same pad, new id
	rg.tick()
	if len(ev.DevicesRestored) != 1 || ev.DevicesRestored[0].Device.ID != FirstPadDevice+1 {
		t.Fatalf("restored = %+v", ev.DevicesRestored)
	}
	rg.src.Frame.Pads[0].SetDown(PadSouth, true)
	rg.tick()
	if !rg.st.JustPressed(rg.jump) {
		t.Fatal("restored pad does not drive player 0")
	}
}

func TestJoinPolicy(t *testing.T) {
	rg := newRig(t, Options{Players: 2, Assign: AssignJoin})
	rg.r.Lobby().Push("menu")
	rg.connectPad(FirstPadDevice, "pad A")
	rg.connectPad(FirstPadDevice+1, "pad B")
	rg.tick()
	if rg.p.Joined() {
		t.Fatal("player 0 joined without pressing join")
	}
	// The lobby drives menus before anyone joined.
	rg.src.Frame.Pads[0].SetDown(PadSouth, true)
	rg.tick()
	if !rg.r.Any().JustPressed(rg.uiConfirm) {
		t.Fatal("lobby does not drive Any")
	}
	rg.src.Frame.Pads[0].SetDown(PadSouth, false)
	rg.src.Frame.Pads[1].SetDown(PadStart, true)
	rg.tick()
	if !rg.p.Joined() || len(rg.r.Events().PlayersJoined) != 1 {
		t.Fatal("pad B did not join player 0")
	}
	if got := rg.p.Devices(); len(got) != 1 || got[0] != FirstPadDevice+1 {
		t.Fatalf("player 0 devices = %v", got)
	}
	rg.key(KeyEnter, true)
	rg.tick()
	p1 := rg.r.Player(1)
	if !p1.Joined() || len(p1.Devices()) != 2 {
		t.Fatalf("keyboard+mouse did not join player 1 together: %v", p1.Devices())
	}
	p1.Leave()
	if p1.Joined() || len(rg.r.Lobby().Devices()) != 4 { // touch, pad A, keyboard, mouse
		t.Fatalf("leave: lobby has %v", rg.r.Lobby().Devices())
	}
}

func TestActiveDeviceAndPointer(t *testing.T) {
	rg := newRig(t, Options{})
	rg.connectPad(FirstPadDevice, "DualSense Wireless Controller")
	rg.tick()
	rg.src.Frame.Pads[0].SetDown(PadSouth, true)
	rg.tick()
	if d := rg.p.ActiveDevice(); d.Family != FamilyPlayStation {
		t.Fatalf("active = %+v", d)
	}
	if ev := rg.r.Events().ActiveDeviceChanged; len(ev) != 1 || ev[0] != 0 {
		t.Fatalf("event = %v", ev)
	}
	rg.key(KeyW, true)
	rg.tick()
	if d := rg.p.ActiveDevice(); d.ID != KeyboardDevice {
		t.Fatalf("active = %+v", d)
	}
	rg.src.Frame.Mouse.X, rg.src.Frame.Mouse.Y = 12, 34
	rg.tick()
	if pt := rg.st.Pointer(); !pt.Valid || pt.X != 12 || pt.Touch {
		t.Fatalf("pointer = %+v", pt)
	}
}

func TestTouchStickAndButton(t *testing.T) {
	rg := newRig(t, Options{}) // screen 400x200
	f := &rg.src.Frame
	f.Touches[0] = TouchFrame{ID: 7, X: 50, Y: 150} // in the floating stick area
	f.NTouches = 1
	rg.tick()
	if x, y := rg.st.Vec2(rg.move); x != 0 || y != 0 {
		t.Fatalf("stick at rest: %v,%v", x, y)
	}
	f.Touches[0].X = 60 // radius 0.1*200 = 20 px
	rg.tick()
	if x, _ := rg.st.Vec2(rg.move); !near(x, 0.5) {
		t.Fatalf("stick: %v, want 0.5", x)
	}
	f.Touches[0].X = 352 // slid onto the jump button: still the stick
	f.Touches[0].Y = 160
	rg.tick()
	if x, _ := rg.st.Vec2(rg.move); x < 0.99 || rg.st.Down(rg.jump) {
		t.Fatalf("stick ownership: x=%v jump=%v", x, rg.st.Down(rg.jump))
	}
	f.Touches[1] = TouchFrame{ID: 8, X: 352, Y: 160} // the jump circle
	f.NTouches = 2
	rg.tick()
	if !rg.st.JustPressed(rg.jump) {
		t.Fatal("touch jump")
	}
	tc := rg.st.Touch()
	if len(tc) != 2 || tc[0].Name != "jump" || !tc[0].Pressed || tc[1].Name != "move" || tc[1].CX != 50 || tc[1].KnobX < 69.9 {
		t.Fatalf("touch controls = %+v", tc)
	}
	if d := rg.p.ActiveDevice(); d.ID != TouchDevice {
		t.Fatalf("active = %+v", d)
	}
}

func TestRumble(t *testing.T) {
	rg := newRig(t, Options{})
	rg.connectPad(FirstPadDevice, "pad")
	rg.tick()
	rg.p.Profile().Settings.Vibration = 0.5
	rg.p.Rumble(1, 0.2, 100*time.Millisecond)
	rg.p.Rumble(0.4, 0.6, 50*time.Millisecond)
	rg.tick()
	v := rg.src.Vibrations
	if len(v) != 1 || v[0] != (Vibration{FirstPadDevice, 0.5, 0.3, 100 * time.Millisecond}) {
		t.Fatalf("vibrations = %+v", v)
	}
	rg.p.Profile().Settings.Vibration = -1
	rg.p.Rumble(1, 1, time.Second)
	rg.tick()
	if len(rg.src.Vibrations) != 1 {
		t.Fatal("vibration off still rumbled")
	}
}

func TestAnyCombinesPlayers(t *testing.T) {
	rg := newRig(t, Options{})
	rg.key(KeySpace, true)
	rg.tick()
	if !rg.r.Any().JustPressed(rg.jump) {
		t.Fatal("Any")
	}
}

func TestSetBindingsKeepsHeldState(t *testing.T) {
	rg := newRig(t, Options{})
	rg.key(KeySpace, true)
	rg.tick()
	rg.r.SetBindings(loadTestBindings(t, rg.acts))
	rg.tick()
	if rg.st.JustPressed(rg.jump) {
		t.Fatal("reloading bindings fired a new press")
	}
}

func TestPressedWithin(t *testing.T) {
	// tap presses Space on tick 1 of the test and releases it on tick 2,
	// then runs until tick n; it returns PressedWithin(jump, window).
	tap := func(rg *rig, n int, window uint32) bool {
		rg.key(KeySpace, true)
		rg.tick()
		rg.key(KeySpace, false)
		for range n - 1 {
			rg.tick()
		}
		return rg.st.PressedWithin(rg.jump, window)
	}
	tests := []struct {
		name   string
		n      int
		window uint32
		want   bool
	}{
		{"press tick, window 1", 1, 1, true},
		{"next tick, window 1", 2, 1, false},
		{"last tick of window", 6, 6, true},
		{"after window", 7, 6, false},
		{"window 0", 1, 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tap(newRig(t, Options{}), tt.n, tt.window); got != tt.want {
				t.Fatalf("PressedWithin = %v, want %v", got, tt.want)
			}
		})
	}
	t.Run("never pressed", func(t *testing.T) {
		rg := newRig(t, Options{})
		if rg.st.PressedWithin(rg.jump, 1000) {
			t.Fatal("buffered press without a press")
		}
	})
	t.Run("held key counts once", func(t *testing.T) {
		rg := newRig(t, Options{})
		rg.key(KeySpace, true)
		for range 4 {
			rg.tick()
		}
		if rg.st.PressedWithin(rg.jump, 3) {
			t.Fatal("holding renewed the press")
		}
	})
	t.Run("survives a blocking mapping", func(t *testing.T) {
		rg := newRig(t, Options{})
		rg.key(KeySpace, true)
		rg.tick()
		rg.p.Push("menu")
		rg.tick()
		if !rg.st.PressedWithin(rg.jump, 2) {
			t.Fatal("press lost when the menu opened")
		}
	})
	t.Run("Listen forgets it", func(t *testing.T) {
		rg := newRig(t, Options{})
		rg.key(KeySpace, true)
		rg.tick()
		rg.p.Listen(ListenOptions{Target: Slot{Mapping: "gameplay", Action: rg.attack, Class: SlotKBM}})
		rg.p.CancelListen()
		if rg.st.PressedWithin(rg.jump, 10) {
			t.Fatal("press survived Listen")
		}
	})
	t.Run("kind panics", func(t *testing.T) {
		rg := newRig(t, Options{})
		mustPanic(t, `input: action "move" is Vec2, not Button`, func() { rg.st.PressedWithin(rg.move, 1) })
		mustPanic(t, `input: action "move" is Vec2, not Button`, func() { rg.p.ConsumePress(rg.move) })
		mustPanic(t, "input: unknown action 999", func() { rg.p.ConsumePress(999) })
	})
}

func TestConsumePress(t *testing.T) {
	rg := newRig(t, Options{})
	rg.key(KeySpace, true)
	rg.tick()
	rg.p.ConsumePress(rg.jump)
	if rg.st.JustPressed(rg.jump) || rg.st.PressedWithin(rg.jump, 10) {
		t.Fatal("consumed press still reported")
	}
	if !rg.st.Down(rg.jump) || rg.st.HeldTicks(rg.jump) != 1 {
		t.Fatal("ConsumePress changed Down or HeldTicks")
	}
	if !rg.r.Any().JustPressed(rg.jump) || !rg.r.Any().PressedWithin(rg.jump, 1) {
		t.Fatal("ConsumePress changed Any")
	}
	rg.tick()
	if rg.st.PressedWithin(rg.jump, 10) {
		t.Fatal("consumed press came back while held")
	}
	rg.key(KeySpace, false)
	rg.tick()
	rg.key(KeySpace, true)
	rg.tick()
	if !rg.st.JustPressed(rg.jump) || !rg.st.PressedWithin(rg.jump, 1) {
		t.Fatal("new press after consume")
	}
}
