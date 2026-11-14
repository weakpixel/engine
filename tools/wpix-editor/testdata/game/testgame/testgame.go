// Package testgame is the editor test game: a small game over the
// artificial asset library of testdata/assets, with one level that places
// Game Objects of the engine's own components and a prefab. Its main package
// (the parent folder) is what an editor host such as the VSCode extension
// builds with -tags editor in its browser tests; nothing in it comes from a
// demo game.
package testgame

import (
	"io/fs"
	"testing/fstest"

	"github.com/weakpixel/engine/asset/library"
	"github.com/weakpixel/engine/editor/host"
	"github.com/weakpixel/engine/game"
	"github.com/weakpixel/engine/render"
	testassets "github.com/weakpixel/engine/testdata/assets"
)

// Asset keys of the files this package adds to the test library.
const (
	// Level is the level the editor opens: a guard, two villager prefab
	// instances and a plain sprite, beside the Tiled map world-1/level-1.tmj.
	Level = "world-1/level-1.level.json"
	// Prefab is the villager prefab the level places twice.
	Prefab = "global/prefabs/villager.prefab.json"
)

// LevelIDs are the ids of the objects of Level, in file order.
var LevelIDs = []string{"guard", "villager-left", "villager-right", "crate"}

// Files returns a new copy of the game's asset library: the test library of
// testdata/assets with Level and Prefab.
func Files() fstest.MapFS {
	files := testassets.Lib()
	// The bindings file binds actions of another game; this one has none.
	delete(files, "global/input/hero.bindings.json")
	files[Level] = &fstest.MapFile{Data: []byte(level)}
	files[Prefab] = &fstest.MapFile{Data: []byte(prefab)}
	return files
}

// Game describes the test game to the editor host. fsys holds its assets;
// nil means Files.
func Game(fsys fs.FS) host.Game {
	if fsys == nil {
		fsys = Files()
	}
	return host.Game{
		FS:    func() (fs.FS, error) { return fsys, nil },
		Level: Level,
		Config: func(assets fs.FS) (game.Config, error) {
			lib, err := library.Open(assets)
			if err != nil {
				return game.Config{}, err
			}
			return game.Config{Library: lib, Width: 320, Height: 180, Camera: render.Camera{Zoom: 1}}, nil
		},
	}
}

const level = `{
  "name": "Level 1",
  "objects": [
    {
      "id": "guard",
      "group": "gfx",
      "class": "AnimatedVisual",
      "components": {
        "gfx:Transform": {"Pos": {"X": 24, "Y": 40}, "PrevPos": {"X": 24, "Y": 40}, "Rot": 0, "Scale": {"X": 1, "Y": 1}},
        "gfx:Sprite": {"Layer": 2, "Origin": {"X": 0.5, "Y": 0.5}, "FlipX": false, "FlipY": false, "Tint": 0, "Blend": 0, "Hidden": false},
        "gfx:Animator": {"Clip": "global/hero/hero.anim.json#idle", "Speed": 1}
      }
    },
    {
      "id": "villager-left",
      "prefab": "global/prefabs/villager.prefab.json",
      "components": {"gfx:Transform": {"Pos": {"X": 48, "Y": 40}}}
    },
    {
      "id": "villager-right",
      "prefab": "global/prefabs/villager.prefab.json",
      "components": {"gfx:Transform": {"Pos": {"X": 72, "Y": 40}}, "gfx:Sprite": {"FlipX": true}}
    },
    {
      "id": "crate",
      "group": "gfx",
      "class": "Visual",
      "components": {
        "gfx:Transform": {"Pos": {"X": 8, "Y": 56}, "PrevPos": {"X": 8, "Y": 56}, "Rot": 0, "Scale": {"X": 1, "Y": 1}},
        "gfx:Sprite": {"ID": "temple/temple.png", "Layer": 1, "Origin": {"X": 0, "Y": 0}, "FlipX": false, "FlipY": false, "Tint": 0, "Blend": 0, "Hidden": false}
      }
    }
  ]
}
`

const prefab = `{
  "name": "villager",
  "objects": [
    {
      "id": "villager",
      "group": "gfx",
      "class": "AnimatedVisual",
      "components": {
        "gfx:Transform": {"Pos": {"X": 0, "Y": 0}, "PrevPos": {"X": 0, "Y": 0}, "Rot": 0, "Scale": {"X": 1, "Y": 1}},
        "gfx:Sprite": {"Layer": 4, "Origin": {"X": 0.5, "Y": 0.5}, "FlipX": false, "FlipY": false, "Tint": 0, "Blend": 0, "Hidden": false},
        "gfx:Animator": {"Clip": "global/hero/hero.anim.json#idle", "Speed": 1}
      }
    }
  ]
}
`
