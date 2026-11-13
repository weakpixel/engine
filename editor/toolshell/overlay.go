package toolshell

import (
	"bytes"
	"io"
	"io/fs"
	"path"
	"sync"
	"time"
)

// OverlayFS is an asset filesystem that serves a few in-memory files over a
// base filesystem. Tools use it for assets that are code, not files, such as
// the particle editor's built-in sprites: the engine loads them like any
// other asset. Hand the same OverlayFS to [Config.FS] and to the engine's
// game.Config.FS, and to [Config.Overlay] so tools can reach it. It is safe
// for concurrent use: the asset workers read while the tool sets.
type OverlayFS struct {
	base  fs.FS
	mu    sync.RWMutex
	files map[string][]byte
}

// NewOverlayFS returns an overlay over base, which may be nil.
func NewOverlayFS(base fs.FS) *OverlayFS {
	return &OverlayFS{base: base, files: map[string][]byte{}}
}

// Set serves data under the slash-separated asset key name.
func (o *OverlayFS) Set(name string, data []byte) {
	o.mu.Lock()
	o.files[name] = data
	o.mu.Unlock()
}

// Open implements [fs.FS]: an overlay file, else the base's.
func (o *OverlayFS) Open(name string) (fs.File, error) {
	o.mu.RLock()
	data, ok := o.files[name]
	o.mu.RUnlock()
	if ok {
		return &memFile{Reader: bytes.NewReader(data), name: path.Base(name), size: int64(len(data))}, nil
	}
	if o.base == nil {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
	}
	return o.base.Open(name)
}

// memFile is an open overlay file.
type memFile struct {
	*bytes.Reader
	name string
	size int64
}

func (f *memFile) Stat() (fs.FileInfo, error) { return f, nil }
func (f *memFile) Close() error               { return nil }

// fs.FileInfo of a memFile.
func (f *memFile) Name() string       { return f.name }
func (f *memFile) Size() int64        { return f.size }
func (f *memFile) Mode() fs.FileMode  { return 0o444 }
func (f *memFile) ModTime() time.Time { return time.Time{} }
func (f *memFile) IsDir() bool        { return false }
func (f *memFile) Sys() any           { return nil }

var _ io.Reader = (*memFile)(nil)
