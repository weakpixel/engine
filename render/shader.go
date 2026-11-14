package render

import (
	"fmt"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/weakpixel/asset"
)

// Shader is a compiled Kage shader. Load it with [LoadShader] (or
// asset.Load[Shader]) from a .kage source file. A compile error fails the
// asset; use [ShaderStore.Err] or the Server's OnError to show it.
//
// The Renderer passes the uniforms [ShaderUniformParams] and
// [ShaderUniformTime]; a shader declares only the ones it uses. A shader that declares one of them with a
// different type fails to load, so the mismatch never panics while drawing.
//
// The placeholder (what a missing, loading or failed handle resolves to) has a
// nil Shader field, which means "no shader": draw code must treat nil as the
// default pipeline.
type Shader struct {
	Shader *ebiten.Shader
}

// ShaderID is the generic handle to a [Shader]. The zero handle means no
// shader. Components and items hold the concrete [gfx.ShaderHandle]; see
// [LoadShader].
type ShaderID = asset.Handle[Shader]

// shaderLoader keeps the Kage source from Decode; the compile happens in
// Finish, on the game goroutine, with the other Ebiten calls.
type shaderLoader struct{}

func (shaderLoader) Decode(_ *asset.DecodeContext, data []byte) ([]byte, error) {
	return data, nil
}

func (shaderLoader) Finish(_ *asset.FinishContext, src []byte) (Shader, error) {
	s, err := ebiten.NewShader(src)
	if err != nil {
		return Shader{}, err
	}
	if err := checkUniforms(s); err != nil {
		s.Deallocate()
		return Shader{}, err
	}
	return Shader{Shader: s}, nil
}

// ShaderUniformParams is the name of the uniform that carries an item's
// Params. Declare it as
//
//	var Params vec4
//
// to read [Item.Params] (gfx.Sprite.Params). A shader that does not declare
// it gets nothing; Ebiten ignores uniforms a shader does not declare.
const ShaderUniformParams = "Params"

// ShaderUniformTime is the name of the uniform that carries the time of the
// frame in seconds, as set with [Renderer.SetTime]. Declare it as
//
//	var Time float
//
// Every shader the Renderer runs (sprites, layer groups, post passes and the
// final pass) gets the same value during one frame.
const ShaderUniformTime = "Time"

// newUniforms returns a uniform map for the Renderer's shader draws, and the
// slice behind its Params entry. time backs the Time entry; the Renderer
// passes the same one-element slice to all its maps, so setting the time
// once per frame updates every map.
//
// Performance trick: storing a fresh []float32 (or float32) in a map[string]any
// boxes it and allocates on every draw. The map is built once with slices
// whose elements the Renderer overwrites in place, so a draw only copies four
// floats. Do not assign to the map per draw.
func newUniforms(time []float32) (map[string]any, []float32) {
	params := make([]float32, 4)
	return map[string]any{ShaderUniformParams: params, ShaderUniformTime: time}, params
}

// checkUniforms draws s once into a 1x1 image with the Renderer's uniforms.
// Ebiten panics when a declared uniform has a different type than the value
// passed (for example "var Params float"); this turns that panic into a load
// error. It runs once per load, in Finish on the game goroutine.
func checkUniforms(s *ebiten.Shader) (err error) {
	probe := ebiten.NewImage(1, 1)
	defer probe.Deallocate()
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("shader uniforms: %v (want: var %s vec4, var %s float)", r, ShaderUniformParams, ShaderUniformTime)
		}
	}()
	u, _ := newUniforms(make([]float32, 1))
	probe.DrawRectShader(1, 1, s, &ebiten.DrawRectShaderOptions{Uniforms: u})
	return nil
}

func (shaderLoader) Unload(s *Shader) {
	if s.Shader != nil {
		s.Shader.Deallocate()
	}
}
