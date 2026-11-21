// Package editorapp is what the native and the js/wasm wpix-editor share:
// the kind table and the resolution of a file to its asset root and key. It
// imports neither Cobra nor debugui, so the wasm build links neither.
package editorapp

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/weakpixel/engine/project"
)

// Resolve finds the asset root of file and the file's asset key. The root is
// assets.root of the settings file found from the file's folder, or the
// file's folder without one. A file outside the root is an error naming both.
// With allowMissing a file that does not exist is not an error (a new file
// of a kind with a starter); its folder must exist.
func Resolve(file string, allowMissing bool) (abs, root, key string, err error) {
	abs, err = filepath.Abs(file)
	if err != nil {
		return "", "", "", err
	}
	if st, err := os.Stat(abs); err != nil {
		if !(allowMissing && errors.Is(err, fs.ErrNotExist)) {
			return "", "", "", err
		}
		if st, err := os.Stat(filepath.Dir(abs)); err != nil || !st.IsDir() {
			return "", "", "", fmt.Errorf("the folder of %s does not exist", file)
		}
	} else if st.IsDir() {
		return "", "", "", fmt.Errorf("%s is a folder", file)
	}
	dir := filepath.Dir(abs)
	settings, _, err := project.Load(dir)
	if err != nil {
		return "", "", "", err
	}
	root = dir
	if settings != nil && settings.AssetsRoot != "" {
		root = settings.AssetsRoot
	}
	rel, err := filepath.Rel(root, abs)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", "", "", errors.New("file " + abs + " is outside the asset root " + root)
	}
	return abs, root, filepath.ToSlash(rel), nil
}
