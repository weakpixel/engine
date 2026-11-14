package library

import (
	"image"
	_ "image/jpeg" // register the decoders DecodeConfig needs
	_ "image/png"
	"io/fs"
)

// imageSize reads only the header of the image at p. ok is false for a
// file that cannot be read or decoded; its loader reports that.
func imageSize(fsys fs.FS, p string) (w, h int, ok bool) {
	f, err := fsys.Open(p)
	if err != nil {
		return 0, 0, false
	}
	defer f.Close()
	cfg, _, err := image.DecodeConfig(f)
	if err != nil {
		return 0, 0, false
	}
	return cfg.Width, cfg.Height, true
}
