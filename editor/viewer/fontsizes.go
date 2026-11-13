package viewer

import (
	"io/fs"
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// Limits of the scan for the sizes a project uses; a project with more
// files than this is not scanned past it.
const (
	maxScanFiles = 300
	maxScanBytes = 1 << 20
	maxSizes     = 12
)

// defaultSizes are shown when the project uses the font nowhere.
var defaultSizes = []float64{12, 16, 24, 32}

// scanFile reports whether name is a file whose text names fonts: levels and
// prefabs hold the keys of Label.Font.
func scanFile(name string) bool {
	return strings.HasSuffix(name, ".level.json") || strings.HasSuffix(name, ".prefab.json")
}

// projectFiles lists the files to scan: files when the host gave a list
// (the page build cannot list directories), else every level and prefab
// under fsys.
func projectFiles(fsys fs.FS, files []string) []string {
	var out []string
	if files != nil {
		for _, f := range files {
			if scanFile(f) {
				out = append(out, f)
			}
		}
		return out
	}
	if fsys == nil {
		return nil
	}
	_ = fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && scanFile(p) {
			out = append(out, p)
		}
		if len(out) >= maxScanFiles {
			return fs.SkipAll
		}
		return nil
	})
	return out
}

// fontSizes returns, sorted, the sizes in which files name the font at asset
// key font ("fonts/ui.ttf"): every "fonts/ui.ttf#16" gives 16. At most
// maxScanFiles files are read and a size is a positive finite number.
func fontSizes(fsys fs.FS, files []string, font string) []float64 {
	if fsys == nil {
		return nil
	}
	re := regexp.MustCompile(regexp.QuoteMeta(font) + `#(\d+(?:\.\d+)?)`)
	var sizes []float64
	for i, name := range files {
		if i >= maxScanFiles {
			break
		}
		data, err := fs.ReadFile(fsys, name)
		if err != nil || len(data) > maxScanBytes {
			continue
		}
		for _, m := range re.FindAllSubmatch(data, -1) {
			if size, err := strconv.ParseFloat(string(m[1]), 64); err == nil && size > 0 && !math.IsInf(size, 0) {
				sizes = append(sizes, size)
			}
		}
	}
	return sortedSizes(sizes)
}

// sortedSizes sorts sizes and removes duplicates and everything past
// maxSizes.
func sortedSizes(sizes []float64) []float64 {
	slices.Sort(sizes)
	sizes = slices.Compact(sizes)
	if len(sizes) > maxSizes {
		sizes = sizes[:maxSizes]
	}
	return sizes
}

// layoutRows stacks rows of the given heights with gap between them and
// returns each row's top and the total height.
func layoutRows(heights []float32, gap float32) (tops []float32, total float32) {
	for _, h := range heights {
		tops = append(tops, total)
		total += h + gap
	}
	return tops, max(total-gap, 0)
}
