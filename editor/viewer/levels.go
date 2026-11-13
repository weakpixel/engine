package viewer

import (
	"io/fs"
	"path"
	"slices"
	"strings"

	"github.com/weakpixel/engine/asset/library"
	"github.com/weakpixel/engine/editor/toolshell"
	"github.com/weakpixel/engine/tiled"
)

// levelSuffix is the file name suffix of a level.
const levelSuffix = ".level.json"

// fileLookup answers the two questions the level search asks about the
// project: whether a file exists, and which map files a world lists.
type fileLookup struct {
	exists    func(key string) bool
	worldMaps func(world string) []string
}

// lookupOf reads the project through the shell: the page build's file list
// when there is one, else probes the filesystem. No directory is listed.
func lookupOf(s *toolshell.Shell) fileLookup {
	var l fileLookup
	if files := s.Files(); files != nil {
		set := make(map[string]bool, len(files))
		for _, f := range files {
			set[f] = true
		}
		l.exists = func(key string) bool { return set[key] }
	} else {
		l.exists = func(key string) bool {
			if s.FS() == nil {
				return false
			}
			st, err := fs.Stat(s.FS(), key)
			return err == nil && !st.IsDir()
		}
	}
	l.worldMaps = func(world string) []string {
		if s.FS() == nil {
			return nil
		}
		data, err := fs.ReadFile(s.FS(), world)
		if err != nil {
			return nil
		}
		w, err := tiled.DecodeWorld(data)
		if err != nil {
			return nil
		}
		return worldMapKeys(world, w)
	}
	return l
}

// worldMapKeys returns the asset keys of the maps of world w, which lives at
// asset key world.
func worldMapKeys(world string, w *tiled.World) []string {
	keys := make([]string, len(w.Maps))
	for i, m := range w.Maps {
		keys[i] = path.Join(path.Dir(world), m.FileName)
	}
	return keys
}

// groupOf returns the library package ("world-1") of the file at key when
// the file sits directly in the package's folder, else "".
func groupOf(key string) string {
	pkg := library.PackageOf(key)
	if pkg == "" || path.Dir(key) != pkg {
		return ""
	}
	return pkg
}

// levelsOfMap lists the levels that use the map at key (rule D3 of the Tiled
// previews plan): the level with the map's base name beside it
// ("level-1.level.json" for "level-1.tmj"), and the package's base level
// ("<pkg>/<pkg>.level.json") when its backdrop is this map ("<pkg>.tmj") or a
// world ("<pkg>.world") that lists it. A map not directly in a package
// folder has none.
func levelsOfMap(l fileLookup, key string) []string {
	group := groupOf(key)
	if group == "" {
		return nil
	}
	var out []string
	if own := strings.TrimSuffix(key, tiled.MapSuffix) + levelSuffix; l.exists(own) {
		out = append(out, own)
	}
	id := path.Base(group)
	groupLevel := path.Join(group, id+levelSuffix)
	if !l.exists(groupLevel) || slices.Contains(out, groupLevel) {
		return out
	}
	backdrop := path.Join(group, id+tiled.MapSuffix) == key
	if world := path.Join(group, id+tiled.WorldSuffix); !backdrop && l.exists(world) {
		backdrop = slices.Contains(l.worldMaps(world), key)
	}
	if backdrop {
		out = append(out, groupLevel)
	}
	return out
}

// levelsOfWorld lists the package's base level of the world at key, whose
// backdrop it is: "<pkg>.level.json" beside "<pkg>.world".
func levelsOfWorld(l fileLookup, key string) []string {
	group := groupOf(key)
	if group == "" || key != path.Join(group, path.Base(group)+tiled.WorldSuffix) {
		return nil
	}
	if groupLevel := path.Join(group, path.Base(group)+levelSuffix); l.exists(groupLevel) {
		return []string{groupLevel}
	}
	return nil
}
