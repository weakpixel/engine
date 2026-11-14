//go:build !js

package game

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

// defaultShotSink writes png to req.Path, or to a new name in dir, and
// returns the path written. It runs on the encoder goroutine.
func defaultShotSink(dir string, req ShotRequest, at time.Time, tick uint64, png []byte) (string, error) {
	if req.Path != "" {
		if err := os.MkdirAll(filepath.Dir(req.Path), 0o755); err != nil {
			return "", fmt.Errorf("game: screenshot: %w", err)
		}
		if err := os.WriteFile(req.Path, png, 0o644); err != nil {
			return "", fmt.Errorf("game: screenshot: %w", err)
		}
		return req.Path, nil
	}
	return writeNewShot(dir, shotName(at, tick), png)
}

// writeNewShot writes png into dir (created if missing) under name, or
// name-2, name-3, ... when that exists. O_EXCL makes the choice safe
// against other captures written at the same time.
func writeNewShot(dir, name string, png []byte) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("game: screenshot: %w", err)
	}
	for n := 1; ; n++ {
		path := filepath.Join(dir, shotCandidate(name, n))
		f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if errors.Is(err, fs.ErrExist) {
			continue
		}
		if err != nil {
			return "", fmt.Errorf("game: screenshot: %w", err)
		}
		_, err = f.Write(png)
		if cerr := f.Close(); err == nil {
			err = cerr
		}
		if err != nil {
			return "", fmt.Errorf("game: screenshot: %w", err)
		}
		return path, nil
	}
}
