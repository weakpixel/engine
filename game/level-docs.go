package game

import (
	"errors"
	"fmt"
	"io/fs"
	"slices"
	"strings"

	"github.com/weakpixel/engine/asset/library"
	"github.com/weakpixel/engine/tiled"
)

// documentSuffix is the suffix of a GOD level document.
const documentSuffix = ".level.json"

// pairKey returns the key of the GOD document paired with the map or world
// key: the same path with the .tmj or .world suffix replaced by .level.json.
func pairKey(key string) string {
	key = strings.TrimSuffix(key, tiled.MapSuffix)
	key = strings.TrimSuffix(key, tiled.WorldSuffix)
	return key + documentSuffix
}

// wantDocument reports whether l may start the document key, and marks it as
// tried: the opt-outs of l.opts and the one-spawn-per-key rule.
func (l *Level) wantDocument(key string) bool {
	if l.opts.NoDocuments || slices.Contains(l.opts.SkipDocuments, key) || slices.Contains(l.docKeys, key) {
		return false
	}
	l.docKeys = append(l.docKeys, key)
	return true
}

// hasFile reports whether the optional file key exists. A key of the
// package the level holds is looked up in the package's file list (a packed
// web build must not fetch a file just to learn it is missing); other keys
// stat Config.FS.
func (l *Level) hasFile(e *Engine, key string) bool {
	if l.load != nil && library.PackageOf(key) == l.load.Package().Name {
		return slices.Contains(l.load.Package().Files, key)
	}
	_, err := fs.Stat(e.cfg.FS, key)
	return err == nil
}

// startPairedDocument spawns the optional document paired with the map or
// world mapKey into l (see pairKey). A missing file is not an error; a file
// that does not spawn is recorded in l.Err and sent to OnAssetError, and the
// map still spawns.
func (e *Engine) startPairedDocument(l *Level, mapKey string) {
	key := pairKey(mapKey)
	if !l.wantDocument(key) {
		return
	}
	if !l.hasFile(e, key) {
		return
	}
	data, err := fs.ReadFile(e.cfg.FS, key)
	if err == nil {
		err = e.startDocument(l, key, data)
	}
	if err != nil {
		err = fmt.Errorf("game: LoadMap %q: %s: %w", l.path, key, err)
		l.partErr = errors.Join(l.partErr, err)
		e.reportAssetError(err)
	}
}
