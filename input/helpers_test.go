package input

import (
	"os"
	"strings"
	"testing"
)

func mustPanic(t *testing.T, want string, f func()) {
	t.Helper()
	defer func() {
		t.Helper()
		r := recover()
		if r == nil {
			t.Fatalf("no panic, want %q", want)
		}
		if msg, _ := r.(string); !strings.Contains(msg, want) {
			t.Fatalf("panic %v, want %q", r, want)
		}
	}()
	f()
}

// testActions is the action table of testdata/bindings.json.
type testActions struct {
	acts                                                         *Actions
	move, jump, attack, save, charge, throttle, aim, pause, join Action
	brake, uiMove, uiConfirm, uiBack                             Action
}

func newTestActions() *testActions {
	a := &Actions{}
	ta := &testActions{acts: a}
	ta.move = a.Vec2("move", "Walk.")
	ta.jump = a.Button("jump", "Jump.")
	ta.attack = a.Button("attack", "Attack.")
	ta.save = a.Button("save", "Quick save.", AllowMods())
	ta.charge = a.Button("charge", "Charge.")
	ta.throttle = a.Axis("throttle", "Throttle.")
	ta.aim = a.Vec2("aim", "Aim.", Delta())
	ta.pause = a.Button("pause", "Pause.")
	ta.join = a.Button("join", "Join.")
	ta.brake = a.Button("brake", "Brake.")
	ta.uiMove = a.Vec2("ui.move", "Menu navigation.")
	ta.uiConfirm = a.Button("ui.confirm", "Confirm.")
	ta.uiBack = a.Button("ui.back", "Back.")
	a.Freeze()
	return ta
}

func loadTestBindings(t testing.TB, acts *Actions) *Bindings {
	t.Helper()
	data, err := os.ReadFile("testdata/bindings.json")
	if err != nil {
		t.Fatal(err)
	}
	b, err := ParseBindings(data, acts)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// rig is a router on a ManualSource with the test bindings and the
// gameplay mapping active on player 0.
type rig struct {
	*testActions
	src *ManualSource
	r   *Router
	p   *Player
	st  *State
}

func newRig(t testing.TB, opts Options) *rig {
	t.Helper()
	ta := newTestActions()
	opts.Actions = ta.acts
	if opts.Assign == AssignJoin && opts.JoinAction == 0 {
		opts.JoinAction = ta.join
	}
	if opts.Bindings == nil {
		opts.Bindings = loadTestBindings(t, ta.acts)
	}
	src := &ManualSource{}
	src.Frame.Width, src.Frame.Height = 400, 200
	r := NewRouter(src, opts)
	p := r.Player(0)
	p.Push("gameplay")
	rg := &rig{testActions: ta, src: src, r: r, p: p, st: p.State()}
	rg.tick() // arm every binding: nothing is held yet
	return rg
}

func (rg *rig) tick() { rg.r.Update() }

func (rg *rig) key(k Key, down bool) {
	if down {
		rg.src.Frame.Keys.Set(k)
	} else {
		rg.src.Frame.Keys.Clear(k)
	}
}

// connectPad plugs in a standard pad with id and returns its frame.
func (rg *rig) connectPad(id DeviceID, name string) *PadFrame {
	f := &rg.src.Frame
	f.Pads[f.NPads] = PadFrame{Device: id, Standard: true}
	f.NPads++
	rg.src.Changes.Connected = append(rg.src.Changes.Connected, DeviceInfo{ID: id, Name: name, GUID: "guid-" + name})
	return &f.Pads[f.NPads-1]
}

func (rg *rig) disconnectPad(id DeviceID) {
	f := &rg.src.Frame
	for i := range f.Pads[:f.NPads] {
		if f.Pads[i].Device == id {
			copy(f.Pads[i:], f.Pads[i+1:f.NPads])
			f.NPads--
			break
		}
	}
	rg.src.Changes.Disconnected = append(rg.src.Changes.Disconnected, id)
}
