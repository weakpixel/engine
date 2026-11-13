package sheetedit

import (
	"fmt"
	"image"
	_ "image/gif" // the formats the engine's texture loader reads
	_ "image/jpeg"
	_ "image/png"
	"os"
	"path/filepath"

	"github.com/weakpixel/engine/sheetfile"
)

// NewSheet returns the text of the starter sheet of the image at imagePath,
// to be saved as sheetPath. The "image" value is the image's path relative to
// the sheet's folder.
func NewSheet(imagePath, sheetPath string) ([]byte, error) {
	f, err := os.Open(imagePath)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	cfg, _, err := image.DecodeConfig(f)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", imagePath, err)
	}
	absImg, err := filepath.Abs(imagePath)
	if err != nil {
		return nil, err
	}
	absSheet, err := filepath.Abs(sheetPath)
	if err != nil {
		return nil, err
	}
	ref, err := filepath.Rel(filepath.Dir(absSheet), absImg)
	if err != nil {
		return nil, err
	}
	return sheetfile.Encode(sheetfile.Starter(filepath.ToSlash(ref), image.Pt(cfg.Width, cfg.Height)))
}
