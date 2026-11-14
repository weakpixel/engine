package particles

import (
	"fmt"

	"github.com/weakpixel/asset"
	"github.com/weakpixel/engine/gfx"
	"github.com/weakpixel/engine/particles/fxfile"
	"github.com/weakpixel/engine/render"
)

// Effect is a loaded *.fx.json file: its emitters and the handles of their
// images and shaders. Load one with [LoadEffect]; it is immutable once loaded
// and a hot reload replaces it.
type Effect struct {
	// Emitters are the effect's emitters in file order; a Ready effect has
	// at least one.
	Emitters []EffectEmitter

	// scope holds the sprites and the shaders.
	scope *asset.Scope
}

// EffectEmitter is one emitter of an [Effect].
type EffectEmitter struct {
	// Emitter is the authored emitter. Its Sprite and Shader keys are kept
	// for tools; the simulation reads the handles below.
	fxfile.Emitter
	// SpriteRef and ShaderRef are the image and shader, loaded into the
	// effect's own scope, so they live as long as the effect. A shader that
	// is loading or failed draws the particles normally, as does a missing
	// image (the placeholder).
	SpriteRef gfx.SpriteHandle
	ShaderRef gfx.ShaderHandle
}

// RegisterLoaders registers [Effect] and its loader on srv. The render
// loaders are needed when an effect names a sprite or a shader: a missing
// one fails the effect with "type not registered". It panics like
// [asset.Register] if Effect is already registered.
func RegisterLoaders(srv *asset.Server) {
	asset.Register[Effect, fxfile.Spec](srv, effectLoader{srv: srv}, Effect{})
}

type effectLoader struct{ srv *asset.Server }

func (effectLoader) Decode(_ *asset.DecodeContext, data []byte) (fxfile.Spec, error) {
	return fxfile.Decode(data)
}

// Finish loads the sprite and the shader into a scope of the effect's own,
// without waiting for them: particles draw the placeholder until the image is
// ready, and a hot reload of the image itself updates the handle in place.
func (l effectLoader) Finish(ctx *asset.FinishContext, s fxfile.Spec) (Effect, error) {
	e := Effect{Emitters: make([]EffectEmitter, len(s.Emitters))}
	e.scope = l.srv.NewScope("effect " + ctx.Key.Path)
	fail := func(i int, what string, err error) (Effect, error) {
		e.scope.Release()
		return Effect{}, fmt.Errorf("effect json: emitters[%d]: %s: %w", i, what, err)
	}
	for i, em := range s.Emitters {
		ee := &e.Emitters[i]
		ee.Emitter = em
		var err error
		if em.Sprite != "" {
			if ee.SpriteRef, err = loadKey(e.scope, em.Sprite, render.LoadSprite); err != nil {
				return fail(i, "sprite", err)
			}
		}
		if em.Shader != "" {
			if ee.ShaderRef, err = loadKey(e.scope, em.Shader, render.LoadShader); err != nil {
				return fail(i, "shader", err)
			}
		}
	}
	return e, nil
}

func (effectLoader) Unload(e *Effect) {
	if e.scope != nil {
		e.scope.Release()
	}
}

// loadKey calls load, turning the asset package's panic for a bad key or an
// unregistered type into an error: the key comes from a data file.
func loadKey[H any](sc *asset.Scope, key string, load func(*asset.Scope, string) H) (h H, err error) {
	defer func() {
		if r := recover(); r != nil {
			msg, ok := r.(string)
			if !ok {
				panic(r)
			}
			err = fmt.Errorf("%s", msg)
		}
	}()
	return load(sc, key), nil
}
