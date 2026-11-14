package viewer

import (
	"encoding/json"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/weakpixel/ecs"
	"github.com/weakpixel/engine/editor/toolshell"
	"github.com/weakpixel/engine/game"
	"github.com/weakpixel/engine/gfx"
	"github.com/weakpixel/engine/input"
)

const flashKage = `//kage:unit pixels
package main

// +shader:param:slot=x,label=Amount,min=0,max=1,default=0.5
// +shader:param:slot=y,label=Pulse,widget=toggle
// +shader:param:slot=z,label=Tilt,widget=angle
var Params vec4
var Time float

func Fragment(dst vec4, src vec2, color vec4) vec4 {
	c := imageSrc0At(src)
	return mix(c, vec4(1, 1, 1, c.a), Params.x)
}
`

// openShader runs the view tool on a shader in a project with one image and
// returns the engine, the viewer and the messages the shell sends.
func openShader(t *testing.T, kage string) (*game.Engine, *Viewer, *recorder, *toolshell.Shell) {
	t.Helper()
	files := fstest.MapFS{"fx/flash.kage": {Data: []byte(kage)}, "art/box.png": {Data: pngBytes(t, 32, 16)}}
	tr := &recorder{}
	v := New().(*Viewer)
	overlay := toolshell.NewOverlayFS(files)
	sh := toolshell.New(toolshell.Config{Name: "view", Tool: v, Key: "fx/flash.kage", FS: overlay, Overlay: overlay, Transport: tr})
	e := game.New(game.Config{NoAudio: true, Width: 320, Height: 180, FS: overlay, Tool: sh, Input: game.InputConfig{Source: &input.ManualSource{}}})
	t.Cleanup(e.Close)
	if err := e.Update(); err != nil {
		t.Fatal(err)
	}
	return e, v, tr, sh
}

func spriteOf(t *testing.T, e *game.Engine) gfx.Sprite {
	t.Helper()
	for _, sp := range ecs.NewView1[gfx.Sprite](e.World()).Each() {
		return *sp
	}
	t.Fatal("no sprite")
	return gfx.Sprite{}
}

func setField(t *testing.T, e *game.Engine, v *Viewer, id, val string) {
	t.Helper()
	if err := v.Set(id, json.RawMessage(val)); err != nil {
		t.Fatalf("set %s: %v", id, err)
	}
	if err := e.Update(); err != nil {
		t.Fatal(err)
	}
}

func TestShaderViewAppliesShaderAndMarkerParams(t *testing.T) {
	e, v, _, _ := openShader(t, flashKage)
	if err := v.Panel().Check(); err != nil {
		t.Fatalf("panel: %v", err)
	}
	if got := label(t, v, idShaderStatus); got != "compiled" {
		t.Fatalf("status = %q", got)
	}
	sp := spriteOf(t, e)
	if sp.Shader == (gfx.ShaderHandle{}) {
		t.Error("the sprite has no shader")
	}
	if sp.Params != [4]float32{0.5, 0, 0, 0} {
		t.Errorf("Params = %v, want the marker default in slot x", sp.Params)
	}
	setField(t, e, v, idShaderParam+"0", "0.25")
	setField(t, e, v, idShaderParam+"1", "true")
	setField(t, e, v, idShaderParam+"2", "180")
	if p := spriteOf(t, e).Params; p[0] != 0.25 || p[1] != 1 || p[2] < 3.14 || p[2] > 3.15 {
		t.Errorf("Params = %v after the controls", p)
	}
	setField(t, e, v, idShaderParam+"0", "5") // clamped to max
	if p := spriteOf(t, e).Params; p[0] != 1 {
		t.Errorf("Params.x = %v, want 1 (max)", p[0])
	}
	setField(t, e, v, idShaderOn, "false")
	if spriteOf(t, e).Shader != (gfx.ShaderHandle{}) {
		t.Error("Enabled off still draws the shader")
	}
	setField(t, e, v, idShaderOn, "true")
	setField(t, e, v, idShaderReset, "null")
	if p := spriteOf(t, e).Params; p != [4]float32{0.5, 0, 0, 0} {
		t.Errorf("Params = %v after Reset", p)
	}
	if err := v.Set(idShaderParam+"9", json.RawMessage("1")); err == nil {
		t.Error("an unknown marker widget is accepted")
	}
}

func TestShaderViewWithoutMarkersHasFourNumbers(t *testing.T) {
	plain := strings.NewReplacer("// +shader:param:slot=x,label=Amount,min=0,max=1,default=0.5\n", "", "// +shader:param:slot=y,label=Pulse,widget=toggle\n", "", "// +shader:param:slot=z,label=Tilt,widget=angle\n", "").Replace(flashKage)
	e, v, _, _ := openShader(t, plain)
	for i := range 4 {
		if _, ok := v.Panel().Find(idShaderRaw + string(rune('0'+i))); !ok {
			t.Errorf("no plain field %d", i)
		}
	}
	setField(t, e, v, idShaderRaw+"3", "0.75")
	if p := spriteOf(t, e).Params; p[3] != 0.75 {
		t.Errorf("Params = %v", p)
	}
}

func TestShaderViewSwitchesTexture(t *testing.T) {
	e, v, _, _ := openShader(t, flashKage)
	if got := label(t, v, "image.size"); got != "128 × 128 px" {
		t.Errorf("default texture size = %q, want the built-in sample", got)
	}
	setField(t, e, v, idShaderTex, `"art/box.png"`)
	if got := label(t, v, "image.size"); got != "32 × 16 px" {
		t.Errorf("size = %q after picking art/box.png", got)
	}
	n := 0
	for range ecs.NewView1[gfx.Sprite](e.World()).Each() {
		n++
	}
	if n != 1 {
		t.Errorf("%d sprites, want 1", n)
	}
	if sp := spriteOf(t, e); sp.Shader == (gfx.ShaderHandle{}) {
		t.Error("the shader is lost with the texture")
	}
	if err := v.Set(idShaderTex, json.RawMessage(`"nope.png"`)); err == nil {
		t.Error("a texture outside the project is accepted")
	}
}

func TestShaderViewReportsCompileAndMarkerErrors(t *testing.T) {
	// Valid Go syntax that does not compile, so both problems are reported.
	bad := "package main\n// +shader:param:slot=q\nvar Params vec4\nfunc Fragment(dst vec4, src vec2, color vec4) vec4 {\n\treturn missing()\n}\n"
	_, v, tr, _ := openShader(t, bad)
	if got := label(t, v, idShaderStatus); got == "compiled" || got == "" {
		t.Errorf("status = %q, want the compile error", got)
	}
	if _, ok := v.Panel().Find(idShaderMarkers); !ok {
		t.Error("no markers label for a broken marker")
	}
	var errs []string
	for _, m := range tr.msgs {
		if m["type"] == "error" {
			errs = append(errs, m["message"].(string))
		}
	}
	joined := strings.Join(errs, "\n")
	if !strings.Contains(joined, "fx/flash.kage") || !strings.Contains(joined, "line 2") {
		t.Errorf("errors %q should name the file and the marker line", joined)
	}
}
