package fxedit

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/weakpixel/engine/editor/toolshell"
	"github.com/weakpixel/engine/gfx"
	"github.com/weakpixel/engine/internal/shaderparam"
	"github.com/weakpixel/engine/particles/fxfile"
)

// rig is a State with its hooks recorded.
type rig struct {
	*State
	edits    [][]byte
	created  []string
	files    []SaveFile
	text     []byte
	existing map[string]bool
}

func newRig(t *testing.T, key string, text []byte) *rig {
	t.Helper()
	r := &rig{existing: map[string]bool{}}
	r.State = NewState(key, Hooks{
		Edit:   func(text []byte) { r.edits = append(r.edits, text); r.text = text },
		Exists: func(k string) bool { return r.existing[k] },
		Create: func(k string, text []byte, files []SaveFile) error {
			r.created, r.files, r.text = append(r.created, k), files, text
			return nil
		},
	})
	r.text = text
	if err := r.Load(text); err != nil {
		t.Fatal(err)
	}
	return r
}

func untitled(t *testing.T) *rig {
	return newRig(t, "", text(Presets()[0].Emitters[0]))
}

func (r *rig) set(t *testing.T, id string, v any) error {
	t.Helper()
	raw, _ := json.Marshal(v)
	return r.Set(id, raw)
}

func fieldIDs(p toolshell.Panel) []string {
	var ids []string
	for _, s := range p.Sections {
		for _, f := range s.Fields {
			ids = append(ids, f.ID)
		}
	}
	return ids
}

func view(t *testing.T) toolshell.Section {
	return toolshell.Section{ID: "view", Label: "View", Fields: []toolshell.Field{toolshell.Button("view.fit", "Fit")}}
}

func panel(t *testing.T, r *rig) toolshell.Panel {
	t.Helper()
	p := r.Panel("fx", view(t))
	if err := p.Check(); err != nil {
		t.Fatalf("Panel.Check: %v", err)
	}
	if _, err := json.Marshal(p); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestPanelOfANewSmokeEffect(t *testing.T) {
	r := untitled(t)
	p := panel(t, r)
	var secs []string
	for _, s := range p.Sections {
		secs = append(secs, s.ID)
	}
	if got := strings.Join(secs, ","); got != "emitters,effect,emitter,look,emission,shape,motion,advanced,save,view" {
		t.Errorf("sections = %s", got)
	}
	ids := strings.Join(fieldIDs(p), " ")
	for _, want := range []string{IDPreset, IDApply, IDKeep, IDRestart, "fx.live", IDImage, IDShader, IDBlend, IDColor, IDChange, IDEndColor, IDScale, IDRate, IDDuration, IDMax, IDShape, IDRadius, IDLife, IDAngle, IDSpeed, IDSpin, IDLocal, SavePathID, SaveGoID} {
		if !strings.Contains(ids, want) {
			t.Errorf("no field %s in %s", want, ids)
		}
	}
	for _, hidden := range []string{IDLoop, IDWidth, IDParamPrefix, IDRawPrefix, "pool", "time", "done"} {
		if strings.Contains(ids, hidden) {
			t.Errorf("field %s shows for a smoke effect", hidden)
		}
	}
	f, _ := p.Find(IDAngle)
	if string(f.Value) != "-90" {
		t.Errorf("angle shows %s, want -90 degrees", f.Value)
	}
	if f.Min == nil || f.Max == nil || *f.Min != -180 || *f.Max != 180 {
		t.Errorf("angle is not a -180..180 slider: %+v", f)
	}
	if f, _ := p.Find(IDRadius); f.Min == nil || f.Max == nil || *f.Min != 0 {
		t.Errorf("radius is not a slider from 0: %+v", f)
	}
	if f, _ := p.Find(IDDrag); f.Min == nil || *f.Min != 0 || f.Max != nil {
		t.Errorf("drag is not bounded below by 0 alone: %+v", f)
	}
	if f, _ := p.Find("fx.live"); !strings.HasPrefix(string(f.Value), `"0 / 128`) {
		t.Errorf("live = %s", f.Value)
	}
	if sec := p.Sections[8]; sec.Label != "Save as" || sec.Collapsed {
		t.Errorf("save section = %+v", sec)
	}
	if r.SavePath != "global/fx/smoke.fx.json" {
		t.Errorf("SavePath = %q", r.SavePath)
	}
}

func TestSetEveryEmitterField(t *testing.T) {
	r := untitled(t)
	r.Params = nil
	values := map[string]any{
		IDImage: "art/p.png", IDShader: "fx/glow.kage", IDBlend: 1, IDColor: "#aabbccdd", IDChange: true,
		IDEndColor: "#00000000", IDEndScale: 2.0, IDScale: 0.5, IDFadeIn: 0.1, IDFadeOut: 0.2,
		IDRate: 5.0, IDBurst: 3, IDDuration: 2.0, IDLoop: true, IDMax: 50,
		IDShape: 2, IDWidth: 4.0, IDHeight: 2.0, IDLife: []float64{1, 2}, IDAngle: 45.0, IDSpread: 90.0,
		IDSpeed: []float64{5, 9}, IDGravX: 1.0, IDGravY: 2.0, IDDrag: 0.5, IDSpin: []float64{-10, 10},
		IDRotate: false, IDLocal: true, IDRawPrefix + "1": 0.5,
	}
	for id, v := range values {
		before := len(r.edits)
		if err := r.set(t, id, v); err != nil {
			t.Fatalf("%s: %v", id, err)
		}
		if len(r.edits) != before+1 {
			t.Errorf("%s made %d document edits, want 1", id, len(r.edits)-before)
		}
		if _, err := fxfile.Decode(r.text); err != nil {
			t.Fatalf("%s: the document does not parse: %v\n%s", id, err, r.text)
		}
	}
	e := r.Em
	if e.Sprite != "art/p.png" || e.Shader != "fx/glow.kage" || e.Blend != gfx.BlendAdditive || e.Max != 50 || e.Params[1] != 0.5 || !e.Local {
		t.Errorf("emitter after the edits: %+v", e)
	}
	// The document holds the same emitter.
	back, err := fxfile.Decode(r.text)
	if err != nil || back.Emitters[0] != e {
		t.Errorf("document emitter differs: %v\n%+v\n%+v", err, back, e)
	}
}

func TestApplyMultiPreset(t *testing.T) {
	r := untitled(t)
	multi := slices.IndexFunc(Presets(), Preset.Multi)
	p := Presets()[multi]
	_ = r.set(t, IDEmitterMute, true)
	if err := r.set(t, IDPreset, multi); err != nil {
		t.Fatal(err)
	}
	if err := r.set(t, IDApply, nil); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(r.Ems, p.Emitters) || r.Sel != 0 || len(r.Muted) != 0 || len(r.edits) != 1 || !r.Restart {
		t.Fatalf("apply %s: %d emitters sel=%d muted=%v edits=%d restart=%v", p.Name, len(r.Ems), r.Sel, r.Muted, len(r.edits), r.Restart)
	}
	if r.SavePath != "global/fx/"+presetFile(multi) {
		t.Errorf("SavePath = %q", r.SavePath)
	}
	// A preset of one emitter also removes the other emitters.
	_ = r.set(t, IDEmitterSel, 1)
	_ = r.set(t, IDPreset, 1)
	if err := r.set(t, IDApply, nil); err != nil {
		t.Fatal(err)
	}
	if len(r.Ems) != 1 || r.Ems[0] != Presets()[1].Emitters[0] || r.Sel != 0 {
		t.Errorf("single preset on a multi effect: %+v sel=%d", r.Ems, r.Sel)
	}
}

func TestSetRoutes(t *testing.T) {
	r := untitled(t)
	if err := r.set(t, IDPreset, 1); err != nil || r.Preset != 1 {
		t.Fatal(err, r.Preset)
	}
	if r.SavePath != "global/fx/sparks.fx.json" {
		t.Errorf("SavePath follows the preset: %q", r.SavePath)
	}
	if len(r.edits) != 0 {
		t.Error("choosing a preset edited the document")
	}
	if err := r.set(t, IDApply, nil); err != nil {
		t.Fatal(err)
	}
	if r.Em.Burst != 30 || len(r.edits) != 1 || !r.Restart {
		t.Errorf("apply: %+v edits=%d restart=%v", r.Em, len(r.edits), r.Restart)
	}
	r.Restart = false
	if err := r.set(t, IDKeep, false); err != nil || r.Keep {
		t.Error("keep not set")
	}
	_ = r.set(t, IDRate, 3.0)
	if r.Restart {
		t.Error("a rate change restarted the effect")
	}
	_ = r.set(t, IDMax, 10)
	if r.Restart {
		t.Error("a Max change restarts through the hot reload, not the restart flag")
	}
	if err := r.set(t, IDRestart, nil); err != nil || !r.Restart {
		t.Error("restart button")
	}
	if err := r.set(t, IDPreset, 99); err == nil {
		t.Error("preset 99 accepted")
	}
	if err := r.set(t, "nope", 1); err == nil {
		t.Error("unknown id accepted")
	}
	if err := r.set(t, IDRate, -1); err == nil {
		t.Error("a nonsense value was accepted")
	}
	// A typed path stops the preset from renaming it.
	_ = r.set(t, SavePathID, "global/fx/mine")
	_ = r.set(t, IDPreset, 2)
	if r.SavePath != "global/fx/mine" {
		t.Errorf("SavePath = %q", r.SavePath)
	}
}

func TestVisibilityInPanel(t *testing.T) {
	r := untitled(t)
	_ = r.set(t, IDDuration, 2.0)
	_ = r.set(t, IDChange, true)
	_ = r.set(t, IDShader, "fx/glow.kage")
	ids := strings.Join(fieldIDs(panel(t, r)), " ")
	for _, want := range []string{IDLoop, IDEndColor, IDEndScale, IDRawPrefix + "0", IDRawPrefix + "3"} {
		if !strings.Contains(ids, want) {
			t.Errorf("%s should show:\n%s", want, ids)
		}
	}
	_ = r.set(t, IDShape, 0)
	ids = strings.Join(fieldIDs(panel(t, r)), " ")
	if strings.Contains(ids, IDRadius) || strings.Contains(ids, IDWidth) {
		t.Error("a point has no size")
	}
}

func TestShaderMarkersInPanel(t *testing.T) {
	r := untitled(t)
	_ = r.set(t, IDShader, "fx/glow.kage")
	r.Params, _ = shaderparam.Parse([]byte("package main\n// +shader:param:slot=x,label=Amount,min=0,max=1\n// +shader:param:slot=y,widget=toggle\n// +shader:param:slot=zw,label=Spin,widget=angle\n// +shader:param:slot=a\nvar Params vec4\n"))
	p := panel(t, r)
	f, ok := p.Find(IDParamPrefix + "0")
	if !ok || f.Label != "Amount" || f.Min == nil || *f.Max != 1 {
		t.Errorf("marker field = %+v", f)
	}
	if f, _ := p.Find(IDParamPrefix + "1"); f.Widget != toolshell.WidgetCheckbox {
		t.Errorf("toggle = %+v", f)
	}
	if _, ok := p.Find(IDRawPrefix + "0"); ok {
		t.Error("plain numbers show next to markers")
	}
	if err := r.set(t, IDParamPrefix+"0", 0.25); err != nil || r.Em.Params[0] != 0.25 {
		t.Errorf("param edit: %v %v", err, r.Em.Params)
	}
	_ = r.set(t, IDParamPrefix+"1", true)
	if r.Em.Params[1] != 1 {
		t.Error("toggle not written")
	}
	r.Params, r.ParamsErr = nil, errors.New("line 3: slot q")
	if !strings.Contains(strings.Join(r.Findings(), "|"), "shader fx/glow.kage: line 3: slot q") {
		t.Errorf("findings = %v", r.Findings())
	}
	panel(t, r)
}

func TestSaveAs(t *testing.T) {
	r := untitled(t)
	_ = r.set(t, SavePathID, "global/fx/puff")
	r.existing["global/fx/puff.fx.json"] = true
	err := r.set(t, SaveGoID, nil)
	if err == nil || !strings.Contains(err.Error(), "exists") || !r.Confirm {
		t.Fatalf("existing target: %v confirm=%v", err, r.Confirm)
	}
	p := panel(t, r)
	if f, _ := p.Find(SaveGoID); f.Label != "Overwrite?" {
		t.Errorf("button = %q", f.Label)
	}
	if len(r.created) != 0 {
		t.Fatal("wrote before the confirm")
	}
	if err := r.set(t, SaveGoID, nil); err != nil {
		t.Fatal(err)
	}
	if len(r.created) != 1 || r.created[0] != "global/fx/puff.fx.json" {
		t.Fatalf("created = %v", r.created)
	}
	if len(r.files) != 1 || r.files[0].Path != "global/fx/puff.png" {
		t.Errorf("files = %v", r.files)
	}
	if strings.Contains(string(r.text), BuiltinPrefix) || !strings.Contains(string(r.text), `"sprite": "global/fx/puff.png"`) {
		t.Errorf("saved text names the built-in sprite:\n%s", r.text)
	}
	if _, err := fxfile.Decode(r.text); err != nil {
		t.Fatal(err)
	}
	if r.Key != "global/fx/puff.fx.json" || r.Em.Sprite != "global/fx/puff.png" || r.Confirm {
		t.Errorf("after saving: key=%q sprite=%q", r.Key, r.Em.Sprite)
	}
	// The next panel is a saved effect: "Save a copy".
	if sec := panel(t, r).Sections[8]; sec.Label != "Save a copy" || !sec.Collapsed {
		t.Errorf("section after saving = %+v", sec)
	}
	if err := r.set(t, SavePathID, "  "); err != nil {
		t.Fatal(err)
	}
	if err := r.set(t, SaveGoID, nil); err == nil {
		t.Error("empty path accepted")
	}
}

func TestSaveCopyOfASavedEffectDoesNotEditTheDocument(t *testing.T) {
	r := newRig(t, "fx/a.fx.json", text(Presets()[0].Emitters[0]))
	r.Em.Sprite = "art/mine.png"
	_ = r.set(t, IDImage, "art/mine.png")
	edits := len(r.edits)
	_ = r.set(t, SavePathID, "fx/b.fx.json")
	if err := r.set(t, SaveGoID, nil); err != nil {
		t.Fatal(err)
	}
	if len(r.edits) != edits || len(r.files) != 0 || r.Key != "fx/a.fx.json" {
		t.Errorf("copy: edits %d->%d files=%v key=%q", edits, len(r.edits), r.files, r.Key)
	}
	_ = r.set(t, IDImage, SoftCircle)
	if f := strings.Join(r.Findings(), "|"); !strings.Contains(f, "built in") {
		t.Errorf("a saved file naming a built-in sprite: %q", f)
	}
}

func TestInvalidDocumentKeepsTheLastGoodModel(t *testing.T) {
	r := untitled(t)
	rate := r.Em.Rate
	if err := r.Load([]byte(`{"emitters": [`)); err == nil {
		t.Fatal("broken text loaded")
	}
	if r.Em.Rate != rate || r.Stale == "" {
		t.Errorf("model lost: %+v stale=%q", r.Em.Rate, r.Stale)
	}
	if err := r.set(t, IDRate, 3.0); !errors.Is(err, ErrStale) {
		t.Errorf("edit while stale: %v", err)
	}
	if p := panel(t, r); !strings.Contains(p.Status, "document has errors") {
		t.Errorf("status = %q", p.Status)
	}
	if err := r.Load(text(Presets()[1].Emitters[0])); err != nil || r.Stale != "" {
		t.Errorf("recovery: %v %q", err, r.Stale)
	}
	if r.Em.Burst != 30 {
		t.Errorf("model after recovery: %+v", r.Em)
	}
}

// TestOnlyAnEffectDocumentLoads: a document with no emitter is stale, and so is a prefab.
func TestOnlyAnEffectDocumentLoads(t *testing.T) {
	for name, doc := range map[string]string{
		"no emitter": `{"emitters":[]}`,
		"a prefab":   `{"objects":[]}`,
	} {
		r := &rig{State: NewState("fx/x.fx.json", Hooks{})}
		if err := r.Load([]byte(doc)); err == nil || r.Stale == "" || r.Loaded {
			t.Errorf("%s: err=%v stale=%q loaded=%v", name, err, r.Stale, r.Loaded)
		}
	}
}

func TestFindingsForMissingAssets(t *testing.T) {
	r := untitled(t)
	r.SetCandidates([]string{"art/p.png"}, []string{"fx/glow.kage"})
	if len(r.Findings()) != 0 {
		t.Errorf("findings = %v", r.Findings())
	}
	_ = r.set(t, IDImage, "art/gone.png")
	_ = r.set(t, IDShader, "fx/gone.kage")
	f := strings.Join(r.Findings(), "|")
	if !strings.Contains(f, "image art/gone.png does not exist") || !strings.Contains(f, "shader fx/gone.kage does not exist") {
		t.Errorf("findings = %q", f)
	}
	p := panel(t, r) // a missing file is shown, not a failure
	if f, _ := p.Find(IDImage); string(f.Value) != `"art/gone.png"` {
		t.Errorf("image = %s", f.Value)
	}
	if _, ok := p.Find("problem.0"); !ok {
		t.Error("no problems section")
	}
}

func TestListAssets(t *testing.T) {
	fsys := fstestMap("a.png", "d/b.JPG", "d/c.kage", ".hidden/x.png", "readme.md")
	imgs, shs := ListAssets(fsys)
	if fmt.Sprint(imgs) != "[a.png d/b.JPG]" || fmt.Sprint(shs) != "[d/c.kage]" {
		t.Errorf("images=%v shaders=%v", imgs, shs)
	}
	if a, b := ListAssets(nil); a != nil || b != nil {
		t.Error("nil fs lists something")
	}
}

func TestNormalizeSavePath(t *testing.T) {
	for in, want := range map[string]string{
		"": "", "  ": "", "a": "a.fx.json", "a.json": "a.fx.json", "a.fx.json": "a.fx.json",
		"x\\y": "x/y.fx.json", "./a": "a.fx.json",
	} {
		if got := normalizeSavePath(in); got != want {
			t.Errorf("normalizeSavePath(%q) = %q, want %q", in, got, want)
		}
	}
}

func fstestMap(names ...string) fstest.MapFS {
	m := fstest.MapFS{}
	for _, n := range names {
		m[n] = &fstest.MapFile{Data: []byte("x")}
	}
	return m
}

func TestWrapDegrees(t *testing.T) {
	for _, tt := range []struct{ in, want float64 }{{-90, -90}, {180, 180}, {270, -90}, {-270, 90}, {450, 90}, {-180, -180}} {
		if got := wrapDegrees(tt.in); got != tt.want {
			t.Errorf("wrapDegrees(%v) = %v, want %v", tt.in, got, tt.want)
		}
	}
}

func TestBuiltinShaderFindings(t *testing.T) {
	em := Emitter{Sprite: "art/p.png", Shader: Burn}
	saved := newRig(t, "fx/a.fx.json", text(em))
	if got := strings.Join(saved.Findings(), "\n"); !strings.Contains(got, "the shader is built in") || strings.Contains(got, "does not exist") {
		t.Errorf("saved: %q", got)
	}
	untitled := newRig(t, "", text(em))
	if got := strings.Join(untitled.Findings(), "\n"); strings.Contains(got, "shader") {
		t.Errorf("untitled: %q", got)
	}
}
