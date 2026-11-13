package viewer

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io/fs"
	"math"
	"path"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/weakpixel/asset"
	"github.com/weakpixel/ecs"
	"github.com/weakpixel/engine/editor/fxedit"
	"github.com/weakpixel/engine/editor/toolshell"
	"github.com/weakpixel/engine/gfx"
	"github.com/weakpixel/engine/internal/shaderparam"
	"github.com/weakpixel/engine/render"
)

// sampleKey is the asset key of the built-in texture a shader is tried on.
// It is served from the shell's overlay file system, so it needs no file in
// the project.
const sampleKey = "view-builtin/sample.png"

const samplePx = 128

var (
	sampleOnce sync.Once
	samplePNG  []byte
)

// sampleImage returns the PNG of the built-in texture: red grows to the
// right, green downwards, blue follows a checker, and the corners are
// transparent, so shaders that work on colors, positions and alpha all show.
func sampleImage() []byte {
	sampleOnce.Do(func() {
		img := image.NewNRGBA(image.Rect(0, 0, samplePx, samplePx))
		const c = (samplePx - 1) / 2.0
		for y := range samplePx {
			for x := range samplePx {
				if math.Hypot(float64(x)-c, float64(y)-c) > samplePx/2 {
					continue // transparent corner
				}
				b := uint8(40)
				if (x/16+y/16)%2 == 0 {
					b = 220
				}
				img.SetNRGBA(x, y, color.NRGBA{R: uint8(x * 255 / (samplePx - 1)), G: uint8(y * 255 / (samplePx - 1)), B: b, A: 255})
			}
		}
		var buf bytes.Buffer
		if err := png.Encode(&buf, img); err != nil {
			panic("viewer: " + err.Error())
		}
		samplePNG = buf.Bytes()
	})
	return samplePNG
}

// Ids of the shader previewer's widgets.
const (
	idShaderStatus  = "shader.status"
	idShaderMarkers = "shader.markers"
	idShaderOn      = "shader.on"
	idShaderTex     = "shader.texture"
	idShaderReset   = "shader.reset"
	idShaderParam   = "shader.param."
	idShaderRaw     = "shader.raw."
)

// shaderView applies a .kage shader to a texture and offers one control per
// +shader:param marker of the file. It builds on the texture previewer, so
// pan, zoom, the grid and the border work as for any image. The values only
// live in the view; nothing is written.
type shaderView struct {
	textureView
	shaderKey string
	texKey    string
	textures  []string
	shader    gfx.ShaderHandle
	on        bool
	params    []shaderparam.Param
	paramsErr error
	vals      [4]float32
	version   uint32
	// shaderErr is the last compile error shown, "" when it compiles.
	shaderErr string
	// reported is the last marker problem sent to the host.
	reported string
}

// Open implements [toolshell.Tool].
func (v *shaderView) Open(s *toolshell.Shell, key string) error {
	v.shell, v.shaderKey, v.on = s, key, true
	if ov := s.Overlay(); ov != nil {
		ov.Set(sampleKey, sampleImage())
	}
	var images []string
	if files := s.Files(); files != nil {
		images, _ = fxedit.ClassifyAssets(files)
	} else {
		images, _ = fxedit.ListAssets(s.FS())
	}
	v.textures = images
	if s.Overlay() != nil {
		v.textures = append([]string{sampleKey}, images...)
	}
	if len(v.textures) == 0 {
		return errors.New("view: " + key + ": the project has no image to try the shader on")
	}
	v.readMarkers()
	for _, p := range v.params {
		for j, slot := range p.Slots {
			v.vals[slot] = float32(p.Default[j])
		}
	}
	h, err := render.LoadShaderSync(v.scope, key)
	v.shader = h
	if err != nil {
		v.shaderErr = err.Error()
		s.ReportError(fmt.Errorf("view: %s: %w", key, err))
	}
	v.version = v.shaderVersion()
	return v.setTexture(v.textures[0])
}

// readMarkers reads the markers from the shader's source; a problem is kept
// and sent to the host once.
func (v *shaderView) readMarkers() {
	v.params, v.paramsErr = nil, nil
	if fsys := v.shell.FS(); fsys != nil {
		if data, err := fs.ReadFile(fsys, v.shaderKey); err == nil {
			v.params, v.paramsErr = shaderparam.Parse(data)
		}
	}
	if msg := errText(v.paramsErr); msg != v.reported {
		v.reported = msg
		if v.paramsErr != nil {
			v.shell.ReportError(fmt.Errorf("view: %s: %w", v.shaderKey, v.paramsErr))
		}
	}
}

func errText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func (v *shaderView) shaderVersion() uint32 {
	return render.ShaderStoreOf(v.shell.Engine().Assets()).Generic().Version(asset.HandleOf[render.Shader](v.shader.Ref))
}

// setTexture shows the texture at key.
func (v *shaderView) setTexture(key string) error {
	v.close(v.shell)
	v.texKey = key
	return v.openImage(v.shell, key)
}

// Sync implements [toolshell.Syncer]: it keeps the sprite's shader and
// Params in step with the controls and notices a reloaded shader.
func (v *shaderView) Sync(s *toolshell.Shell) {
	v.textureView.Sync(s)
	if v.ready {
		if sp := ecs.GetCompOrNil[gfx.Sprite](s.Engine().World(), v.entity); sp != nil {
			sp.Params = v.vals
			sp.Shader = gfx.ShaderHandle{}
			if v.on {
				sp.Shader = v.shader
			}
		}
	}
	if ver := v.shaderVersion(); ver != v.version {
		v.version = ver
		v.readMarkers()
	}
	st := render.ShaderStoreOf(s.Engine().Assets())
	if msg := errText(st.Err(v.shader)); msg != v.shaderErr {
		v.shaderErr = msg
		if msg != "" {
			s.ReportError(fmt.Errorf("view: %s: %s", v.shaderKey, msg))
		}
	}
}

// Panel implements [toolshell.Tool].
func (v *shaderView) Panel() toolshell.Panel {
	status := "compiled"
	if v.shaderErr != "" {
		status = v.shaderErr
	}
	thumbs := map[string]string{sampleKey: "data:image/png;base64," + base64.StdEncoding.EncodeToString(sampleImage())}
	shader := toolshell.Section{ID: "shader", Label: "Shader", Fields: []toolshell.Field{
		toolshell.Label(idShaderStatus, "Status", status),
		toolshell.Checkbox(idShaderOn, "Enabled", v.on).WithHint("Off draws the plain texture"),
		toolshell.Asset(idShaderTex, "Texture", toolshell.AssetImage, v.texKey, v.textures).WithThumbs(thumbs),
	}}
	if v.paramsErr != nil {
		shader.Fields = append(shader.Fields, toolshell.Label(idShaderMarkers, "Markers", v.paramsErr.Error()))
	}
	fields := v.paramFields()
	fields = append(fields, toolshell.Button(idShaderReset, "Reset").WithHint("Back to the defaults of the markers"))
	return toolshell.Panel{Title: path.Base(v.shaderKey), Sections: []toolshell.Section{
		shader,
		{ID: "params", Label: "Params", Fields: fields},
		v.imageSection(),
		v.shell.ViewSection(),
	}}
}

// paramFields is one field per marker, or four plain numbers for a shader
// without markers.
func (v *shaderView) paramFields() []toolshell.Field {
	num := func(id, label string, f float64) toolshell.Field {
		return toolshell.Number(id, label, f).WithStep(0.05).WithDigits(3)
	}
	if len(v.params) == 0 {
		fields := make([]toolshell.Field, 4)
		for i := range fields {
			fields[i] = num(idShaderRaw+strconv.Itoa(i), "Params "+strconv.Itoa(i), float64(v.vals[i]))
		}
		return fields
	}
	var fields []toolshell.Field
	for i, p := range v.params {
		id := idShaderParam + strconv.Itoa(i)
		switch p.Widget {
		case "color":
			c := [4]float64{0, 0, 0, 1}
			for j, slot := range p.Slots {
				c[j] = float64(v.vals[slot])
			}
			fields = append(fields, toolshell.Color(id, p.Label, fmt.Sprintf("#%02x%02x%02x%02x", ch(c[0]), ch(c[1]), ch(c[2]), ch(c[3]))))
		case "toggle":
			fields = append(fields, toolshell.Checkbox(id, p.Label, v.vals[p.Slots[0]] != 0))
		case "angle":
			fields = append(fields, toolshell.Number(id, p.Label, fxedit.Degrees(v.vals[p.Slots[0]])).WithDigits(1))
		default:
			f := num(id, p.Label, float64(v.vals[p.Slots[0]]))
			if p.HasRange {
				f = f.Range(p.Min, p.Max)
			}
			fields = append(fields, f)
		}
	}
	return fields
}

func ch(f float64) int { return int(math.Min(math.Max(f, 0), 1)*255 + 0.5) }

// Set implements [toolshell.Tool].
func (v *shaderView) Set(id string, val json.RawMessage) error {
	switch {
	case id == idShaderOn:
		return json.Unmarshal(val, &v.on)
	case id == idShaderReset:
		v.vals = [4]float32{}
		for _, p := range v.params {
			for j, slot := range p.Slots {
				v.vals[slot] = float32(p.Default[j])
			}
		}
		return nil
	case id == idShaderTex:
		var key string
		if err := json.Unmarshal(val, &key); err != nil {
			return fmt.Errorf("view: want a texture key: %w", err)
		}
		if !slices.Contains(v.textures, key) {
			return fmt.Errorf("view: %q is not a texture of the project", key)
		}
		return v.setTexture(key)
	case strings.HasPrefix(id, idShaderRaw):
		i, err := strconv.Atoi(strings.TrimPrefix(id, idShaderRaw))
		if err != nil || i < 0 || i > 3 {
			return fmt.Errorf("view: unknown widget %q", id)
		}
		var f float64
		if err := json.Unmarshal(val, &f); err != nil {
			return fmt.Errorf("view: want a number: %w", err)
		}
		v.vals[i] = float32(f)
		return nil
	case strings.HasPrefix(id, idShaderParam):
		i, err := strconv.Atoi(strings.TrimPrefix(id, idShaderParam))
		if err != nil || i < 0 || i >= len(v.params) {
			return fmt.Errorf("view: unknown widget %q", id)
		}
		return v.setParam(v.params[i], val)
	}
	return fmt.Errorf("view: unknown widget %q", id)
}

// setParam writes the value of a marker's control into its slots.
func (v *shaderView) setParam(p shaderparam.Param, val json.RawMessage) error {
	switch p.Widget {
	case "color":
		var s string
		if err := json.Unmarshal(val, &s); err != nil {
			return fmt.Errorf("view: want a color string: %w", err)
		}
		c, err := fxedit.ParseColor(s)
		if err != nil {
			return err
		}
		comp := [4]float32{float32(c>>24) / 255, float32(c>>16&255) / 255, float32(c>>8&255) / 255, float32(c&255) / 255}
		for j, slot := range p.Slots {
			v.vals[slot] = comp[j]
		}
	case "toggle":
		var on bool
		if err := json.Unmarshal(val, &on); err != nil {
			return fmt.Errorf("view: want true or false: %w", err)
		}
		v.vals[p.Slots[0]] = map[bool]float32{false: 0, true: 1}[on]
	default:
		var f float64
		if err := json.Unmarshal(val, &f); err != nil {
			return fmt.Errorf("view: want a number: %w", err)
		}
		if p.Widget == "angle" {
			f = f * math.Pi / 180
		} else if p.HasRange {
			f = math.Min(math.Max(f, p.Min), p.Max)
		}
		v.vals[p.Slots[0]] = float32(f)
	}
	return nil
}
