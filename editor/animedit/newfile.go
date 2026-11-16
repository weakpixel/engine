package animedit

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/weakpixel/engine/anim/animfile"
)

// NewDefinition returns the text of the starter definition of the Aseprite
// export at sheetPath, to be saved as defPath (the same text "wpix anim
// create" writes). The "sheet" value is the export's path relative to the
// definition's folder.
func NewDefinition(sheetPath, defPath string) ([]byte, error) {
	data, err := os.ReadFile(sheetPath)
	if err != nil {
		return nil, err
	}
	sheet, err := animfile.DecodeSheet(data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", sheetPath, err)
	}
	absSheet, err := filepath.Abs(sheetPath)
	if err != nil {
		return nil, err
	}
	absDef, err := filepath.Abs(defPath)
	if err != nil {
		return nil, err
	}
	ref, err := filepath.Rel(filepath.Dir(absDef), absSheet)
	if err != nil {
		return nil, err
	}
	return animfile.Encode(animfile.Starter(sheet, filepath.ToSlash(ref)))
}
