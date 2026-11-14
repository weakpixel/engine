package input

import (
	"bytes"
	"os"
	"testing"
)

func readFile(name string) ([]byte, error) { return os.ReadFile(name) }

// script drives a rig through a fixed input sequence and returns the jump
// and move values of every tick.
func script(rg *rig) []string {
	var out []string
	for i := range 40 {
		rg.key(KeySpace, i%7 < 3)
		rg.key(KeyD, i > 10 && i < 30)
		if i == 20 {
			rg.connectPad(FirstPadDevice, "pad")
		}
		if i > 20 {
			rg.src.Frame.Pads[0].Axes[0] = float32(i%5) / 4
		}
		rg.tick()
		x, y := rg.st.Vec2(rg.move)
		out = append(out, stateLine(rg.st.Down(rg.jump), rg.st.JustPressed(rg.jump), x, y))
	}
	return out
}

func stateLine(down, pressed bool, x, y float32) string {
	b := []byte{'-', '-'}
	if down {
		b[0] = 'D'
	}
	if pressed {
		b[1] = 'P'
	}
	return string(b) + " " + string(rune('0'+int(x*4))) + string(rune('0'+int(y*4+4)))
}

func TestRecordReplay(t *testing.T) {
	ta := newTestActions()
	b := loadTestBindings(t, ta.acts)
	src := &ManualSource{}
	r := NewRouter(src, Options{Actions: ta.acts, Bindings: b})
	rec := &Recorder{}
	r.Record(rec) // from the very first tick
	r.Player(0).Push("gameplay")
	live := &rig{testActions: ta, src: src, r: r, p: r.Player(0), st: r.Player(0).State()}
	want := script(live)
	if rec.Ticks() != 40 {
		t.Fatalf("recorded %d ticks", rec.Ticks())
	}
	if len(rec.entries) >= 40 {
		t.Fatalf("idle ticks were not folded: %d entries", len(rec.entries))
	}

	var buf bytes.Buffer
	if _, err := rec.WriteTo(&buf); err != nil {
		t.Fatal(err)
	}
	loaded, err := ReadRecording(&buf)
	if err != nil {
		t.Fatal(err)
	}
	rs := NewReplaySource(loaded)
	r2 := NewRouter(rs, Options{Actions: ta.acts, Bindings: b})
	r2.Player(0).Push("gameplay")
	st := r2.Player(0).State()
	for i := range 40 {
		r2.Update()
		x, y := st.Vec2(ta.move)
		if got := stateLine(st.Down(ta.jump), st.JustPressed(ta.jump), x, y); got != want[i] {
			t.Fatalf("tick %d: replay %q, live %q", i, got, want[i])
		}
	}
	if !rs.Done() {
		t.Fatal("replay not done")
	}
}

func TestRecorderCap(t *testing.T) {
	rec := &Recorder{MaxTicks: 10}
	var f Frame
	var ch DeviceChanges
	for i := range 30 {
		f.Mouse.X = float32(i)
		rec.add(&f, &ch)
	}
	if rec.Ticks() != 10 || rec.entries[0].Frame.Mouse.X != 20 {
		t.Fatalf("ticks=%d first=%v", rec.Ticks(), rec.entries[0].Frame.Mouse.X)
	}
}
