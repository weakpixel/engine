package game

import (
	"strings"
	"testing"

	"github.com/weakpixel/asset"
)

func TestStoresMatchStoreOf(t *testing.T) {
	e := New(Config{NoAudio: true})
	t.Cleanup(e.Close)
	st := e.Stores()
	// Each accessor must return the store the XStoreOf function returns; the
	// generic view exposes the underlying *asset.Store for comparison.
	for name, pair := range map[string][2]any{
		"Sprites":  {st.Sprites().Generic(), StoresOf(e.Assets()).Sprites().Generic()},
		"Fonts":    {st.Fonts().Generic(), StoresOf(e.Assets()).Fonts().Generic()},
		"Atlases":  {st.Atlases().Generic(), StoresOf(e.Assets()).Atlases().Generic()},
		"Shaders":  {st.Shaders().Generic(), StoresOf(e.Assets()).Shaders().Generic()},
		"Clips":    {st.Clips().Generic(), StoresOf(e.Assets()).Clips().Generic()},
		"ClipSets": {st.ClipSets().Generic(), StoresOf(e.Assets()).ClipSets().Generic()},
		"Tilesets": {st.Tilesets().Generic(), StoresOf(e.Assets()).Tilesets().Generic()},
		"Maps":     {st.Maps().Generic(), StoresOf(e.Assets()).Maps().Generic()},
		"Worlds":   {st.Worlds().Generic(), StoresOf(e.Assets()).Worlds().Generic()},
		"Sounds":   {st.Sounds().Generic(), StoresOf(e.Assets()).Sounds().Generic()},
		"Music":    {st.Music().Generic(), StoresOf(e.Assets()).Music().Generic()},
	} {
		if pair[0] != pair[1] {
			t.Errorf("%s: Engine.Stores and StoresOf(srv) disagree", name)
		}
	}
	st.Bindings()
	st.Prefabs()
}

func TestStoresUnregisteredKindPanicsOnlyWhenAsked(t *testing.T) {
	st := StoresOf(asset.NewServer(asset.Options{})) // no loaders registered
	defer func() {
		if r := recover(); r == nil || !strings.Contains(r.(string), "asset: type not registered") {
			t.Fatalf("panic = %v", r)
		}
	}()
	st.Clips()
}
