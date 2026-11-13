package fxedit

import (
	"strings"
	"testing"

	"github.com/weakpixel/engine/particles/fxfile"
)

// names returns the emitter names of the last written document.
func names(t *testing.T, r *rig) string {
	t.Helper()
	spec, err := fxfile.Decode(r.text)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, e := range spec.Emitters {
		out = append(out, e.Name)
	}
	return strings.Join(out, ",")
}

func TestEmitterList(t *testing.T) {
	r := untitled(t)
	r.set(t, IDEmitterName, "a")
	r.set(t, IDEmitterAdd, nil)
	r.set(t, IDEmitterName, "b")
	if got := names(t, r); got != "a,b" || r.Sel != 1 || r.Em.Name != "b" {
		t.Fatalf("after add: %q sel %d em %q", got, r.Sel, r.Em.Name)
	}
	r.set(t, IDEmitterDup, nil)
	if got := names(t, r); got != "a,b,b" || r.Sel != 2 {
		t.Fatalf("after duplicate: %q sel %d", got, r.Sel)
	}
	r.set(t, IDEmitterName, "c")
	r.set(t, IDEmitterUp, nil)
	if got := names(t, r); got != "a,c,b" || r.Sel != 1 {
		t.Fatalf("after move up: %q sel %d", got, r.Sel)
	}
	r.set(t, IDEmitterSel, 0)
	r.set(t, IDEmitterUp, nil) // already first: no edit
	n := len(r.edits)
	r.set(t, IDEmitterUp, nil)
	if len(r.edits) != n || r.Em.Name != "a" {
		t.Fatalf("moving the first emitter up edited the document")
	}
	r.set(t, IDEmitterDel, nil)
	if got := names(t, r); got != "c,b" || r.Sel != 0 || r.Em.Name != "c" {
		t.Fatalf("after delete: %q sel %d", got, r.Sel)
	}
	r.set(t, IDEmitterSel, 1)
	r.set(t, IDEmitterRate(), 12.0)
	if spec, _ := fxfile.Decode(r.text); spec.Emitters[1].Rate != 12 || spec.Emitters[0].Rate == 12 {
		t.Fatalf("a field edit hit the wrong emitter: %+v", spec.Emitters)
	}
	p := panel(t, r)
	if f, ok := p.Find(IDEmitterSel); !ok || len(f.Options) != 2 {
		t.Fatalf("emitter dropdown = %+v", f)
	}
}

func IDEmitterRate() string { return IDRate }

func TestEmitterLimits(t *testing.T) {
	r := untitled(t)
	if err := r.set(t, IDEmitterDel, nil); err == nil {
		t.Error("deleted the only emitter")
	}
	for range fxfile.MaxEmitters - 1 {
		if err := r.set(t, IDEmitterAdd, nil); err != nil {
			t.Fatal(err)
		}
	}
	if err := r.set(t, IDEmitterAdd, nil); err == nil || len(r.Ems) != fxfile.MaxEmitters {
		t.Errorf("added a ninth emitter: err=%v len=%d", err, len(r.Ems))
	}
	if err := r.set(t, IDEmitterSel, 99); err == nil {
		t.Error("selected an emitter that does not exist")
	}
}

func TestMuteAndSoloArePreviewOnly(t *testing.T) {
	r := untitled(t)
	r.set(t, IDEmitterName, "a")
	r.set(t, IDEmitterAdd, nil)
	r.set(t, IDEmitterName, "b")
	edits, rev := len(r.edits), r.Rev
	r.set(t, IDEmitterMute, true)
	if len(r.edits) != edits || r.Rev == rev {
		t.Fatalf("mute edited the document (%d edits) or left the preview alone (rev %d)", len(r.edits)-edits, r.Rev)
	}
	spec, _ := fxfile.Decode(r.previewText())
	if len(spec.Emitters) != 1 || spec.Emitters[0].Name != "a" {
		t.Fatalf("preview with b muted = %+v", spec.Emitters)
	}
	r.set(t, IDEmitterSel, 0)
	r.set(t, IDEmitterSolo, nil) // a already plays alone: this un-solos
	if spec, _ = fxfile.Decode(r.previewText()); len(spec.Emitters) != 2 {
		t.Fatalf("after unsolo = %d emitters", len(spec.Emitters))
	}
	r.set(t, IDEmitterSolo, nil)
	if spec, _ = fxfile.Decode(r.previewText()); len(spec.Emitters) != 1 || spec.Emitters[0].Name != "a" {
		t.Fatalf("solo a = %d emitters", len(spec.Emitters))
	}
	r.set(t, IDEmitterMute, true)
	r.set(t, IDEmitterSel, 1)
	r.set(t, IDEmitterMute, true)
	if spec, _ = fxfile.Decode(r.previewText()); len(spec.Emitters) != 1 {
		t.Fatalf("everything muted still needs a valid document: %+v", spec.Emitters)
	}
	if got := names(t, r); got != "a,b" {
		t.Fatalf("saved document = %q, mutes must not reach it", got)
	}
}

func TestSaveCopiesEveryBuiltinSprite(t *testing.T) {
	r := untitled(t)
	r.set(t, IDEmitterAdd, nil)
	other := BuiltinKeys()[len(BuiltinKeys())-1]
	next := r.Em
	next.Sprite = other
	if err := r.setEmitter(next); err != nil {
		t.Fatal(err)
	}
	if err := r.set(t, SaveGoID, nil); err != nil {
		t.Fatal(err)
	}
	spec, err := fxfile.Decode(r.text)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.files) != 2 || spec.Emitters[0].Sprite != "global/fx/smoke.png" || spec.Emitters[1].Sprite != "global/fx/smoke-2.png" {
		t.Fatalf("files %v, sprites %q %q", fileKeys(r.files), spec.Emitters[0].Sprite, spec.Emitters[1].Sprite)
	}
}

func TestResetEmitters(t *testing.T) {
	r := untitled(t)
	r.set(t, IDEmitterAdd, nil)
	r.set(t, IDEmitterAdd, nil)
	r.set(t, IDEmitterMute, true)
	r.set(t, IDEmitterSel, 1)
	if err := r.set(t, IDEmitterReset, nil); err != nil {
		t.Fatal(err)
	}
	if len(r.Ems) != 1 || r.Ems[0] != Presets()[DefaultPreset].Emitters[0] || r.Sel != 0 || r.muted(0) {
		t.Fatalf("after reset: %d emitters sel=%d muted=%v", len(r.Ems), r.Sel, r.Muted)
	}
	if spec, err := fxfile.Decode(r.text); err != nil || len(spec.Emitters) != 1 {
		t.Fatalf("document after reset: %v %+v", err, spec)
	}
}
