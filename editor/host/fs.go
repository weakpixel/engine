package host

import (
	"errors"
	"io/fs"
	"testing/fstest"
)

// layeredFS reads files from FS (the workspace over HTTP) and falls back to
// fallback, the game's own files (nil for none), for files FS does not have
// (a font that exists only in memory). Directories come from list: HTTP
// cannot list them, and the asset library scans the listing when there is
// no catalog.
type layeredFS struct {
	fs.FS
	fallback fs.FS
	list     fs.FS
}

// newLayeredFS lays the workspace web over own. files are the asset keys of
// the workspace's files (the host page's index); the listing holds them and
// own's files, as empty entries: only names and directories matter there.
func newLayeredFS(web, own fs.FS, files []string) layeredFS {
	list := fstest.MapFS{}
	for _, f := range files {
		list[f] = &fstest.MapFile{}
	}
	if own != nil {
		_ = fs.WalkDir(own, ".", func(name string, de fs.DirEntry, err error) error {
			if err == nil && !de.IsDir() {
				list[name] = &fstest.MapFile{}
			}
			return nil
		})
	}
	return layeredFS{FS: web, fallback: own, list: list}
}

// isDir reports whether the listing has a directory name. A webview's
// resource server answers a directory URL with 401 (a plain server with
// 404), not a listing.
func (l layeredFS) isDir(name string) bool {
	st, err := fs.Stat(l.list, name)
	return err == nil && st.IsDir()
}

// Stat is needed explicitly: fs.WalkDir stats its root, and HTTP cannot
// answer for a directory.
func (l layeredFS) Stat(name string) (fs.FileInfo, error) {
	if l.isDir(name) {
		return fs.Stat(l.list, name)
	}
	st, err := fs.Stat(l.FS, name)
	if errors.Is(err, fs.ErrNotExist) && l.fallback != nil {
		return fs.Stat(l.fallback, name)
	}
	return st, err
}

func (l layeredFS) Open(name string) (fs.File, error) {
	if l.isDir(name) {
		return l.list.Open(name)
	}
	f, err := l.FS.Open(name)
	if errors.Is(err, fs.ErrNotExist) && l.fallback != nil {
		return l.fallback.Open(name)
	}
	return f, err
}

func (l layeredFS) ReadFile(name string) ([]byte, error) {
	data, err := fs.ReadFile(l.FS, name)
	if errors.Is(err, fs.ErrNotExist) && l.fallback != nil {
		return fs.ReadFile(l.fallback, name)
	}
	return data, err
}

func (l layeredFS) ReadDir(name string) ([]fs.DirEntry, error) {
	return fs.ReadDir(l.list, name)
}
