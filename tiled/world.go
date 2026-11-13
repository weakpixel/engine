package tiled

import (
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"strings"
)

// WorldSuffix is the file name suffix of a world.
const WorldSuffix = ".world"

// World is a Tiled world: maps placed side by side.
type World struct {
	Maps []WorldMap
}

// WorldMap is one map of a [World].
type WorldMap struct {
	// FileName is the map's path relative to the world file.
	FileName string
	// X and Y are the map's top-left corner in world pixels; Width and Height
	// its size in pixels (informational: the map file is authoritative).
	X, Y          int
	Width, Height int
}

type rawWorld struct {
	Maps []struct {
		FileName string `json:"fileName"`
		X        int    `json:"x"`
		Y        int    `json:"y"`
		Width    int    `json:"width"`
		Height   int    `json:"height"`
	} `json:"maps"`
	Patterns []json.RawMessage `json:"patterns"`
}

// DecodeWorld decodes a world (*.world). Worlds that place maps by file name
// patterns are not supported: list the maps in "maps".
func DecodeWorld(data []byte) (*World, error) {
	var r rawWorld
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, fmt.Errorf("world: invalid JSON: %w", err)
	}
	var errs []error
	if len(r.Patterns) > 0 {
		errs = append(errs, unsupported("world patterns", `list the maps in "maps"`))
	}
	w := &World{Maps: make([]WorldMap, 0, len(r.Maps))}
	seen := make(map[string]bool, len(r.Maps))
	for i, m := range r.Maps {
		switch {
		case m.FileName == "":
			errs = append(errs, fmt.Errorf("map %d: fileName is empty", i))
			continue
		case !strings.HasSuffix(m.FileName, MapSuffix):
			errs = append(errs, fmt.Errorf("map %d: %q is not a %s map", i, m.FileName, MapSuffix))
		case seen[path.Clean(m.FileName)]:
			errs = append(errs, fmt.Errorf("map %d: %q is listed twice", i, m.FileName))
		}
		seen[path.Clean(m.FileName)] = true
		w.Maps = append(w.Maps, WorldMap{FileName: m.FileName, X: m.X, Y: m.Y, Width: m.Width, Height: m.Height})
	}
	if len(r.Maps) == 0 && len(r.Patterns) == 0 {
		errs = append(errs, errors.New("world lists no maps"))
	}
	return w, errors.Join(errs...)
}
