package render

import (
	_ "embed"
	"fmt"

	"github.com/hajimehoshi/ebiten/v2"
)

// builtinShader names a shader the engine ships, used by the passes of
// helpers such as [PostChain.AddBloom]; the zero value is none. Built-in
// shaders are not assets: they are compiled by the Renderer the first time a
// pass that uses them runs, so a helper works from the first frame.
type builtinShader uint8

const (
	bloomBright builtinShader = iota + 1
	bloomDown
	bloomBlur
	bloomAdd
	builtinCount
)

var (
	//go:embed shaders/bloom-bright.kage
	srcBloomBright []byte
	//go:embed shaders/bloom-down.kage
	srcBloomDown []byte
	//go:embed shaders/bloom-blur.kage
	srcBloomBlur []byte
	//go:embed shaders/bloom-add.kage
	srcBloomAdd []byte
)

// builtinSource returns the Kage source and the name of b.
func builtinSource(b builtinShader) ([]byte, string) {
	switch b {
	case bloomBright:
		return srcBloomBright, "bloom-bright"
	case bloomDown:
		return srcBloomDown, "bloom-down"
	case bloomBlur:
		return srcBloomBlur, "bloom-blur"
	case bloomAdd:
		return srcBloomAdd, "bloom-add"
	}
	panic(fmt.Sprintf("render: unknown built-in shader %d", b))
}

// builtinShader returns the compiled shader b, compiling it on first use.
// A compile error is a bug in the engine, so it panics.
func (r *Renderer) builtinShader(b builtinShader) *ebiten.Shader {
	if s := r.builtins[b]; s != nil {
		return s
	}
	src, name := builtinSource(b)
	s, err := ebiten.NewShader(src)
	if err != nil {
		panic(fmt.Sprintf("render: built-in shader %s: %v", name, err))
	}
	r.builtins[b] = s
	return s
}
