package host

import (
	"fmt"
	"io/fs"

	"github.com/weakpixel/engine/asset/library"
	"github.com/weakpixel/engine/game"
	"github.com/weakpixel/engine/render"
)

// PrefabGame returns the built-in [Game] of the engine-only prefab editor:
// no systems, no extra assets and no sound, over the asset library of the
// workspace (the page's files, see [Page.Files]). The prefab stage runs no
// game code, so every project can use it. The prefab's components that the
// engine does not know are data only (see [Page.GOD]).
func PrefabGame() Game {
	return Game{
		Config: func(assets fs.FS) (game.Config, error) {
			if assets == nil {
				return game.Config{}, fmt.Errorf("host: the prefab editor needs the workspace's assets (a host page with assetBase)")
			}
			lib, err := library.Open(assets)
			if err != nil {
				return game.Config{}, err
			}
			return game.Config{NoAudio: true, Library: lib, Width: 320, Height: 180, Camera: render.Camera{Zoom: 1}}, nil
		},
	}
}
