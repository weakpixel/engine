package input

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func TestListenCapturesAfterRelease(t *testing.T) {
	rg := newRig(t, Options{})
	target := Slot{Mapping: "gameplay", Action: rg.jump, Class: SlotKBM, Slot: 0}
	rg.key(KeyEnter, true) // the key that opened the rebind prompt is still down
	rg.tick()
	rg.p.Listen(ListenOptions{Target: target})
	mustPanic(t, "input: Listen: player 0 is already listening", func() { rg.p.Listen(ListenOptions{Target: target}) })
	rg.tick()
	if len(rg.r.Events().Captured) != 0 {
		t.Fatal("captured the held key")
	}
	rg.key(KeyEnter, false)
	rg.tick()
	rg.key(KeyK, true)
	rg.key(KeySpace, true)
	rg.tick()
	got := rg.r.Events().Captured
	if len(got) != 1 || got[0].Control != KeyControl(KeyK) && got[0].Control != KeyControl(KeySpace) || got[0].Target != target {
		t.Fatalf("captured = %+v", got)
	}
	if rg.st.Down(rg.jump) {
		t.Fatal("actions fired while listening")
	}
	if rg.p.Listening() {
		t.Fatal("still listening")
	}
	// The key used for capturing does not fire jump when listening ends.
	rg.tick()
	if rg.st.Down(rg.jump) {
		t.Fatal("captured key fired the action")
	}
}

func TestListenCancelAndMods(t *testing.T) {
	rg := newRig(t, Options{})
	rg.p.Listen(ListenOptions{Target: Slot{Mapping: "gameplay", Action: rg.save, Class: SlotKBM}})
	rg.tick()
	rg.key(KeyControlLeft, true)
	rg.tick()
	rg.key(KeyL, true)
	rg.tick()
	got := rg.r.Events().Captured
	if len(got) != 1 || got[0].Control != KeyControl(KeyL) || got[0].Mods != 1<<ModCtrl {
		t.Fatalf("captured = %+v", got)
	}
	if name := rg.r.BindingName(got[0].Control, got[0].Mods, FamilyKeyboard); name != "Ctrl+L" {
		t.Fatalf("name = %q", name)
	}

	rg.key(KeyControlLeft, false)
	rg.key(KeyL, false)
	rg.p.Listen(ListenOptions{Target: Slot{Mapping: "gameplay", Action: rg.save, Class: SlotKBM}})
	rg.tick()
	rg.key(KeyShiftLeft, true) // a modifier alone is captured on release
	rg.tick()
	rg.key(KeyShiftLeft, false)
	rg.tick()
	if got := rg.r.Events().Captured; len(got) != 1 || got[0].Control != KeyControl(KeyShiftLeft) {
		t.Fatalf("captured = %+v", got)
	}

	rg.p.Listen(ListenOptions{Target: Slot{Mapping: "gameplay", Action: rg.jump, Class: SlotKBM}})
	rg.tick()
	rg.key(KeyEscape, true)
	rg.tick()
	if got := rg.r.Events().Captured; len(got) != 1 || !got[0].Canceled {
		t.Fatalf("captured = %+v", got)
	}
}

func TestListenGamepad(t *testing.T) {
	rg := newRig(t, Options{})
	rg.connectPad(FirstPadDevice, "pad")
	rg.tick()
	rg.p.Listen(ListenOptions{Target: Slot{Mapping: "gameplay", Action: rg.jump, Class: SlotGamepad}})
	rg.tick()
	rg.key(KeyJ, true) // keyboard is not a gamepad slot
	rg.src.Frame.Pads[0].Axes[1] = -0.8
	rg.tick()
	got := rg.r.Events().Captured
	want := Control{Type: CtrlPadStick, Code: uint16(LeftStick), Part: PartYNeg}
	if len(got) != 1 || got[0].Control != want {
		t.Fatalf("captured = %+v", got)
	}
}

func TestBindPolicies(t *testing.T) {
	rg := newRig(t, Options{})
	jump := Slot{Mapping: "gameplay", Action: rg.jump, Class: SlotKBM, Slot: 0}
	mouseLeft := MouseButtonControl(MouseLeft)

	cf := rg.p.Conflicts(jump, mouseLeft, 0)
	if len(cf) != 1 || cf[0].Action != rg.attack {
		t.Fatalf("conflicts = %+v", cf)
	}
	if err := rg.p.Bind(jump, mouseLeft, 0, Swap); err != nil {
		t.Fatal(err)
	}
	rg.src.Frame.Mouse.SetDown(MouseLeft, true)
	rg.key(KeySpace, true)
	rg.tick()
	rg.tick()
	if !rg.st.Down(rg.jump) || !rg.st.Down(rg.attack) {
		t.Fatalf("swap: jump=%v attack=%v", rg.st.Down(rg.jump), rg.st.Down(rg.attack))
	}
	rg.src.Frame.Mouse.SetDown(MouseLeft, false)
	rg.tick()
	if rg.st.Down(rg.jump) || !rg.st.Down(rg.attack) {
		t.Fatal("after swap Space should attack and the mouse should jump")
	}
	rg.key(KeySpace, false)

	// Replace: Q (throttle's axis neg) for jump unbinds the throttle part.
	q := KeyControl(KeyQ)
	if err := rg.p.Bind(jump, q, 0, Replace); err != nil {
		t.Fatal(err)
	}
	rg.key(KeyQ, true)
	rg.tick()
	rg.tick()
	if !rg.st.Down(rg.jump) || rg.st.Axis(rg.throttle) != 0 {
		t.Fatalf("replace: jump=%v throttle=%v", rg.st.Down(rg.jump), rg.st.Axis(rg.throttle))
	}
	rg.key(KeyQ, false)

	// Locked: Escape belongs to pause.
	err := rg.p.Bind(jump, KeyEscapeControl, 0, Swap)
	if err == nil || !strings.Contains(err.Error(), "locked") {
		t.Fatalf("locked conflict: %v", err)
	}
	if err := rg.p.Bind(Slot{Mapping: "gameplay", Action: rg.pause, Class: SlotKBM}, KeyControl(KeyP), 0, Keep); err == nil {
		t.Fatal("rebinding a locked binding succeeded")
	}
	if err := rg.p.Bind(Slot{Mapping: "gameplay", Action: rg.move, Class: SlotKBM}, KeyControl(KeyP), 0, Keep); err == nil {
		t.Fatal("rebinding a composite as a whole succeeded")
	}
	if err := rg.p.Bind(jump, PadButtonControl(PadNorth), 0, Keep); err == nil {
		t.Fatal("a pad button in a kbm slot succeeded")
	}
}

func TestBindPartAndProfileRoundTrip(t *testing.T) {
	rg := newRig(t, Options{})
	up := Slot{Mapping: "gameplay", Action: rg.move, Class: SlotKBM, Slot: 0, Part: "up"}
	if err := rg.p.Bind(up, KeyControl(KeyZ), 0, Keep); err != nil {
		t.Fatal(err)
	}
	rg.key(KeyZ, true)
	rg.tick()
	if _, y := rg.st.Vec2(rg.move); y != -1 {
		t.Fatalf("Z should move up: y=%v", y)
	}
	rg.key(KeyZ, false)

	data, err := rg.p.Profile().Marshal()
	if err != nil {
		t.Fatal(err)
	}
	prof, err := ParseProfile(data)
	if err != nil {
		t.Fatalf("%v\n%s", err, data)
	}
	// A fresh router with the saved profile behaves the same.
	rg2 := newRig(t, Options{})
	rg2.p.SetProfile(prof)
	rg2.key(KeyZ, true)
	rg2.tick()
	if _, y := rg2.st.Vec2(rg2.move); y != -1 {
		t.Fatalf("loaded profile: y=%v", y)
	}
	if got := rg2.p.Slots("gameplay", rg2.move); len(got) != 3 || got[0].Controls[0] != KeyControl(KeyZ) {
		t.Fatalf("slots = %+v", got)
	}
	rg2.p.Reset("gameplay", rg2.move)
	rg2.tick()
	if _, y := rg2.st.Vec2(rg2.move); y != 0 {
		t.Fatal("reset kept the override")
	}
}

func TestChangedDefaultReachesUntouchedActions(t *testing.T) {
	rg := newRig(t, Options{})
	if err := rg.p.Bind(Slot{Mapping: "gameplay", Action: rg.jump, Class: SlotKBM}, KeyControl(KeyK), 0, Keep); err != nil {
		t.Fatal(err)
	}
	data, _ := rgFile(t)
	data = bytes.Replace(data, []byte(`"attack": [{ "control": "mouse:left" }`), []byte(`"attack": [{ "control": "key:F" }`), 1)
	b, err := ParseBindings(data, rg.acts)
	if err != nil {
		t.Fatal(err)
	}
	rg.r.SetBindings(b)
	rg.tick()
	rg.key(KeyF, true)
	rg.key(KeyK, true)
	rg.tick()
	if !rg.st.Down(rg.attack) || !rg.st.Down(rg.jump) {
		t.Fatalf("attack=%v jump=%v", rg.st.Down(rg.attack), rg.st.Down(rg.jump))
	}
}

func TestStaleOverrideSkipped(t *testing.T) {
	var got []error
	rg := newRig(t, Options{OnError: func(err error) { got = append(got, err) }})
	rg.p.SetProfile(&Profile{Overrides: []Override{
		{Mapping: "gameplay", Action: "fly", Class: "kbm", Binding: &BindingSpec{Control: "key:F"}},
		{Mapping: "gone", Action: "jump", Class: "kbm", Binding: &BindingSpec{Control: "key:F"}},
		{Mapping: "gameplay", Action: "jump", Class: "kbm", Binding: &BindingSpec{Control: "key:K"}},
	}})
	if len(got) != 2 || !strings.Contains(errors.Join(got...).Error(), `unknown action "fly"`) {
		t.Fatalf("errors = %v", got)
	}
	rg.key(KeyK, true)
	rg.tick()
	if !rg.st.Down(rg.jump) {
		t.Fatal("the valid override was not applied")
	}
}

func TestNamesAndGlyphs(t *testing.T) {
	rg := newRig(t, Options{})
	rg.src.Names = map[Key]string{KeyQ: "a"} // AZERTY
	for _, tc := range []struct {
		c    Control
		fam  Family
		want string
	}{
		{KeyControl(KeyQ), FamilyKeyboard, "A"},
		{PadButtonControl(PadSouth), FamilyXbox, "A"},
		{PadButtonControl(PadSouth), FamilyPlayStation, "Cross"},
		{PadButtonControl(PadSouth), FamilyNintendo, "B"},
		{PadButtonControl(PadStart), FamilyGeneric, "Menu"},
		{Control{Type: CtrlPadStick, Code: 0, Part: PartYNeg}, FamilyXbox, "Left Stick Up"},
		{MouseButtonControl(MouseRight), FamilyKeyboard, "Right Click"},
		{Control{Type: CtrlMouseWheel, Part: PartYNeg}, FamilyKeyboard, "Wheel Up"},
	} {
		if got := rg.r.ControlName(tc.c, tc.fam); got != tc.want {
			t.Errorf("%v/%v = %q, want %q", tc.c, tc.fam, got, tc.want)
		}
	}
	if g := rg.r.GlyphKey(PadButtonControl(PadSouth), FamilyXbox); g != "xbox/pad_south" {
		t.Fatalf("glyph = %q", g)
	}
	jump := Control{Type: CtrlTouch, Code: 0} // regions sort by name: jump, move
	if s := rg.r.Bindings().ControlString(jump); s != "touch:jump" {
		t.Fatalf("touch control = %q", s)
	}
}

func rgFile(t *testing.T) ([]byte, error) {
	t.Helper()
	return readFile("testdata/bindings.json")
}
