package mcpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/weakpixel/engine/internal/jsonedit"
)

// writeDocument writes next as the text of the file abs (the asset key),
// whose text was old (nil for a new file). Without a game on that document it
// is a plain atomic write. With one, the game gets the text as a "document"
// message right after the write and answers whether it applied (plan D5): if
// it refuses, or does not answer, the file is put back as it was and the
// game's error is returned, so an agent cannot leave a level the game
// rejects. Other files (a prefab, a sheet) reach the game through the asset
// watcher.
func (t *tools) writeDocument(abs, key string, old, next []byte) error {
	s := t.game.current()
	if s == nil || s.cl.openPath() != key {
		return writeFileAtomic(abs, next)
	}
	if err := writeFileAtomic(abs, next); err != nil {
		return err
	}
	ctx := context.Background()
	err := s.cl.apply(ctx, "document", "", next)
	if err == nil {
		return nil
	}
	if old == nil {
		os.Remove(abs)
	} else if werr := writeFileAtomic(abs, old); werr != nil {
		return fmt.Errorf("the game refused the edit (%v) and the file could not be restored: %v", err, werr)
	}
	if old != nil && !s.cl.isClosed() {
		s.cl.apply(ctx, "document", "", old) // best effort: the game may have taken part of it
	}
	return fmt.Errorf("the game refused the new text of %s, so the file was left as it was: %w", key, err)
}

// viewportLoop writes the edits the developer makes in the game window to
// the document, until the game ends (plan D6).
func (t *tools) viewportLoop(ctx context.Context, s *gameSession) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.cl.closedCh:
			return
		case ev := <-s.cl.events:
			if err := t.applyViewportEvent(s, ev); err != nil {
				t.game.logs.add("server", fmt.Sprintf("could not write the %s from the game window to the file: %v", ev.Type, err))
			}
		}
	}
}

// applyViewportEvent writes one "edit", "create" or "field" message of the
// game to the open document with the same splices the edit tools use.
func (t *tools) applyViewportEvent(s *gameSession, ev viewportEvent) error {
	key := s.cl.openPath()
	if key == "" {
		return fmt.Errorf("no document is open")
	}
	_, err := t.editDocument(key, "", func(text []byte, doc *document, info docInfo) ([]byte, error) {
		switch ev.Type {
		case "create":
			if len(bytes.TrimSpace(ev.Object)) == 0 {
				return nil, fmt.Errorf("the create message has no object")
			}
			return jsonedit.Insert(text, ev.Object, -1, "objects")
		case "edit":
			i, err := findObject(doc, ev.ID, nil)
			if err != nil {
				return nil, err
			}
			path := componentPath(i, ev.Component, nil)
			if ev.Override && string(bytes.TrimSpace(ev.JSON)) == "{}" {
				if _, ok := doc.Objects[i].Components[ev.Component]; !ok {
					return text, nil // nothing was overridden
				}
				return jsonedit.Delete(text, path...)
			}
			return jsonedit.Set(text, ev.JSON, path...)
		case "field":
			if len(ev.Field) == 0 {
				return nil, fmt.Errorf("the field message names no field")
			}
			i, err := findObject(doc, ev.ID, nil)
			if err != nil {
				return nil, err
			}
			steps := make([]any, len(ev.Field)-1)
			for k, f := range ev.Field[1:] {
				steps[k] = f
			}
			value, err := json.Marshal(ev.Value)
			if err != nil {
				return nil, err
			}
			return t.setAt(text, info, i, doc.Objects[i], ev.Field[0], steps, value)
		}
		return nil, fmt.Errorf("unknown message %q", ev.Type)
	})
	return err
}

// assetPollInterval is how often the asset root is scanned for changes
// while a game runs (plan D10).
const assetPollInterval = 500 * time.Millisecond

// stamp identifies one version of a file.
type stamp struct {
	mod  time.Time
	size int64
}

// scanAssets returns the stamp of every file under root by asset key. Hidden
// files (the temporary files of an atomic write) are left out.
func scanAssets(root string) map[string]stamp {
	out := map[string]stamp{}
	filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if strings.HasPrefix(d.Name(), ".") && p != root {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			return nil
		}
		fi, err := d.Info()
		if err != nil {
			return nil
		}
		rel, err := filepath.Rel(root, p)
		if err == nil {
			out[filepath.ToSlash(rel)] = stamp{fi.ModTime(), fi.Size()}
		}
		return nil
	})
	return out
}

// changedKeys returns the keys that are new, gone or different in next.
func changedKeys(prev, next map[string]stamp) []string {
	var keys []string
	for k, v := range next {
		if p, ok := prev[k]; !ok || p != v {
			keys = append(keys, k)
		}
	}
	for k := range prev {
		if _, ok := next[k]; !ok {
			keys = append(keys, k)
		}
	}
	slices.Sort(keys)
	return keys
}

// watchAssets tells the game about changed asset files until ctx ends: the
// game reloads them (plan D10). The open document is not an asset change:
// when its text differs from what the game has, it is sent as a document.
func (t *tools) watchAssets(ctx context.Context, s *gameSession, every time.Duration) {
	root := t.p.AssetRoot
	if root == "" {
		return
	}
	prev := scanAssets(root)
	tick := time.NewTicker(every)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.cl.closedCh:
			return
		case <-tick.C:
		}
		next := scanAssets(root)
		keys := changedKeys(prev, next)
		prev = next
		if len(keys) > 0 {
			t.notifyChanged(ctx, s, keys)
		}
	}
}

// notifyChanged sends assetsChanged for keys, and the open document if its
// file changed under the game.
func (t *tools) notifyChanged(ctx context.Context, s *gameSession, keys []string) {
	open := s.cl.openPath()
	var assets []string
	docChanged := false
	for _, k := range keys {
		if k == open {
			docChanged = true
		} else {
			assets = append(assets, k)
		}
	}
	if len(assets) > 0 {
		if _, err := s.cl.sendSync(ctx, map[string]any{"type": "assetsChanged", "paths": assets}); err != nil && ctx.Err() == nil {
			t.game.logs.add("server", "assetsChanged: "+err.Error())
		}
	}
	if docChanged {
		t.syncOpenDocument(ctx, s)
	}
}

// syncOpenDocument sends the open document's file to the game when it
// differs from what the game has (a change by someone else than this server,
// such as the developer's editor).
func (t *tools) syncOpenDocument(ctx context.Context, s *gameSession) {
	t.docMu.Lock()
	defer t.docMu.Unlock()
	key := s.cl.openPath()
	abs, _, err := t.p.AssetPath(key)
	if err != nil {
		return
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		return
	}
	s.cl.mu.Lock()
	same := bytes.Equal(data, s.cl.docText)
	s.cl.mu.Unlock()
	if same {
		return
	}
	if err := s.cl.apply(ctx, "document", "", data); err != nil && ctx.Err() == nil {
		t.game.logs.add("server", fmt.Sprintf("the game did not take the changed %s: %v", key, err))
	}
}
