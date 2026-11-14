package goddoc

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// godFileSuffix ends the name of every GOD file: zz__gen.god.json next to
// a package's sources, <group>.god.json in a specs folder.
const godFileSuffix = ".god.json"

// skippedDir reports whether the directory walk ignores a directory below
// its root, following the go tool: vendor, testdata, node_modules and
// directories starting with "." or "_".
func skippedDir(name string) bool {
	switch name {
	case "vendor", "testdata", "node_modules":
		return true
	}
	return strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_")
}

// collect expands inputs into the sorted, de-duplicated list of definition
// files they name. An input is a file, a directory (searched recursively) or a
// pattern ending in "..." ("./...", "dir/..."). The filesystem is walked
// directly, so no module loading takes place. Every bad input is reported, a
// directory or pattern that matches no definition file is an error.
func collect(inputs []string) ([]string, error) {
	var errs []error
	seen := map[string]bool{}
	var files []string
	add := func(path string) {
		if abs, err := filepath.Abs(path); err == nil {
			if seen[abs] {
				return
			}
			seen[abs] = true
		}
		files = append(files, path)
	}

	for _, input := range inputs {
		root, recursive := strings.CutSuffix(input, "...")
		if strings.Contains(root, "...") {
			errs = append(errs, fmt.Errorf("%s: \"...\" is only supported at the end of a pattern", input))
			continue
		}
		if recursive {
			root = strings.TrimSuffix(root, "/")
			if root == "" {
				root = "."
			}
		}

		info, err := os.Stat(root)
		switch {
		case err != nil:
			errs = append(errs, err)
		case !info.IsDir():
			if recursive {
				errs = append(errs, fmt.Errorf("%s: %s is not a directory", input, root))
				continue
			}
			add(root)
		default:
			found, err := walkDir(root)
			if err != nil {
				errs = append(errs, err)
				continue
			}
			if len(found) == 0 {
				errs = append(errs, fmt.Errorf("%s: no *%s files found", input, godFileSuffix))
			}
			for _, f := range found {
				add(f)
			}
		}
	}

	slices.Sort(files)
	return files, errors.Join(errs...)
}

// mergedGOD reports whether path is a merged GOD that wpix doc --format god
// wrote. A walk skips it: it repeats the package files next to it, and would
// keep a removed component alive. Named explicitly, it is read.
func mergedGOD(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	var f struct {
		Generator string `json:"generator"`
	}
	return json.Unmarshal(data, &f) == nil && f.Generator == generatorName
}

// walkDir returns the definition files below root. root itself is never
// skipped, so an explicit testdata directory can be documented.
func walkDir(root string) ([]string, error) {
	var found []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		switch {
		case err != nil:
			return err
		case d.IsDir():
			if path != root && skippedDir(d.Name()) {
				return filepath.SkipDir
			}
		case strings.HasSuffix(d.Name(), godFileSuffix) && !mergedGOD(path):
			found = append(found, path)
		}
		return nil
	})
	return found, err
}
