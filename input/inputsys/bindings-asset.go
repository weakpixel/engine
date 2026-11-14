package inputsys

import (
	"github.com/weakpixel/asset"
	"github.com/weakpixel/engine/input"
)

// This file is the bindings asset: bindings files loaded through the asset
// system. It lives here, with the rest of the engine wiring, so that package
// input needs no asset system.

// BindingsHandle refers to compiled [input.Bindings] loaded through the asset
// system. It is 8 bytes, comparable and pointer-free; the zero value means
// "none". No component holds one, so it lives here rather than in gfx.
type BindingsHandle struct{ asset.Ref }

// bindingsLoader decodes and compiles a bindings file on a worker: parsing
// and validation are pure CPU work against the frozen (read-only) Actions.
type bindingsLoader struct{ acts *input.Actions }

func (l bindingsLoader) Decode(_ *asset.DecodeContext, data []byte) (*input.Bindings, error) {
	return input.ParseBindings(data, l.acts)
}

func (l bindingsLoader) Finish(_ *asset.FinishContext, b *input.Bindings) (input.Bindings, error) {
	return *b, nil
}

func (bindingsLoader) Unload(*input.Bindings) {}

// RegisterLoaders makes bindings files ("input/bindings.json") loadable on
// srv, validated against acts. The placeholder is empty bindings, so a
// handle that is loading or failed binds nothing. It panics with
// "input: RegisterLoaders: nil Actions" or "input: RegisterLoaders: Actions
// not frozen", and like [asset.Register] if it runs twice on srv.
func RegisterLoaders(srv *asset.Server, acts *input.Actions) {
	if acts == nil {
		panic("input: RegisterLoaders: nil Actions")
	}
	if !acts.Frozen() {
		panic("input: RegisterLoaders: Actions not frozen")
	}
	asset.Register(srv, bindingsLoader{acts: acts}, *input.NewBindings(acts))
}

// LoadBindings returns the handle for a bindings file and adds a hold for
// sc. It never blocks and panics like [asset.Load].
func LoadBindings(sc *asset.Scope, key string) BindingsHandle {
	return BindingsHandle{Ref: asset.Load[input.Bindings](sc, key).Ref()}
}

// LoadBindingsSync is LoadBindings followed by loading until the file is
// ready or failed, like [asset.LoadSync]. It blocks.
func LoadBindingsSync(sc *asset.Scope, key string) (BindingsHandle, error) {
	h, err := asset.LoadSync[input.Bindings](sc, key)
	return BindingsHandle{Ref: h.Ref()}, err
}

// BindingsStore resolves [BindingsHandle] values. It is a small value; copy
// it freely.
type BindingsStore struct{ s *asset.Store[input.Bindings] }

// BindingsStoreOf returns the bindings store of srv. It panics like
// [asset.StoreOf] if [RegisterLoaders] has not run on srv.
func BindingsStoreOf(srv *asset.Server) BindingsStore {
	return BindingsStore{asset.StoreOf[input.Bindings](srv)}
}

// Get returns the bindings, or the empty placeholder; see [asset.Store.Get]
// for the pointer validity rule.
func (s BindingsStore) Get(h BindingsHandle) *input.Bindings {
	return s.s.Get(asset.HandleOf[input.Bindings](h.Ref))
}

// Lookup is Get plus the load state.
func (s BindingsStore) Lookup(h BindingsHandle) (*input.Bindings, asset.State) {
	return s.s.Lookup(asset.HandleOf[input.Bindings](h.Ref))
}

// Ready reports whether h refers to loaded bindings.
func (s BindingsStore) Ready(h BindingsHandle) bool {
	return s.s.Ready(asset.HandleOf[input.Bindings](h.Ref))
}

// Err returns why the file failed to load, or nil.
func (s BindingsStore) Err(h BindingsHandle) error {
	return s.s.Err(asset.HandleOf[input.Bindings](h.Ref))
}

// Version changes whenever the asset is committed again (first load, and
// hot reload once the asset system has it).
func (s BindingsStore) Version(h BindingsHandle) uint32 {
	return s.s.Version(asset.HandleOf[input.Bindings](h.Ref))
}
