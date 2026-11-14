// Package sheetfile is the grid sheet format, *.sheet.json: fixed-size
// cells over one image (doc/guide/engine/asset-kinds.md#grid-sheet). The
// atlas loader in package render and the sheet tool of wpix-editor
// both use it, so the tool shows exactly the frames the loader makes. It
// imports the standard library only.
package sheetfile

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"image"
)

// Suffix is the file name suffix of a grid sheet.
const Suffix = ".sheet.json"

// Pivot is the optional pivot of every frame, as a fraction of the frame.
type Pivot struct {
	X float32 `json:"x"`
	Y float32 `json:"y"`
}

// File is a grid sheet. Frames are named "0", "1", ... left to right, top to
// bottom.
type File struct {
	// Image is the texture, relative to the sheet's folder.
	Image string `json:"image"`
	// FrameW and FrameH are the size of one cell in pixels, at least 1.
	FrameW int `json:"frameW"`
	FrameH int `json:"frameH"`
	// Margin is the border around the grid in pixels, Spacing the gap
	// between cells; neither is negative.
	Margin  int `json:"margin,omitempty"`
	Spacing int `json:"spacing,omitempty"`
	// Pivot is nil for the default pivot.
	Pivot *Pivot `json:"pivot,omitempty"`
}

// Decode parses the text of a sheet and checks it with [File.Validate]. All
// problems of a well-formed file are reported together.
func Decode(data []byte) (*File, error) {
	var f File
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("grid sheet: %w", err)
	}
	if err := f.Validate(); err != nil {
		return nil, err
	}
	return &f, nil
}

// Encode returns the canonical text of f: two-space indent, zero margin and
// spacing left out, a trailing newline. It does not validate, so a tool can
// write a state that is still being edited; Decode does.
func Encode(f *File) ([]byte, error) {
	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

// Validate collects every problem of the fields, independent of any image.
func (f *File) Validate() error {
	var errs []error
	if f.Image == "" {
		errs = append(errs, errors.New(`grid sheet: "image" is required`))
	}
	if f.FrameW <= 0 || f.FrameH <= 0 {
		errs = append(errs, fmt.Errorf("grid sheet: frameW and frameH must be positive, got %dx%d", f.FrameW, f.FrameH))
	}
	if f.Margin < 0 || f.Spacing < 0 {
		errs = append(errs, fmt.Errorf("grid sheet: margin and spacing must not be negative, got %d and %d", f.Margin, f.Spacing))
	}
	return errors.Join(errs...)
}

// Equal reports whether a and b encode to the same text.
func (f *File) Equal(o *File) bool {
	a, _ := Encode(f)
	b, _ := Encode(o)
	return bytes.Equal(a, b)
}

// Grid is the layout of a valid sheet over an image of a given size.
type Grid struct {
	File *File
	// Cols and Rows are the cells that fit; both are 0 when not even one
	// does.
	Cols, Rows int
	// Size is the image size.
	Size image.Point
}

// Layout cuts an image of the given size by f: as many whole cells as fit
// inside the margin on both sides. The file must be valid.
func (f *File) Layout(size image.Point) Grid {
	g := Grid{File: f, Size: size}
	cols := (size.X - 2*f.Margin + f.Spacing) / (f.FrameW + f.Spacing)
	rows := (size.Y - 2*f.Margin + f.Spacing) / (f.FrameH + f.Spacing)
	if cols >= 1 && rows >= 1 {
		g.Cols, g.Rows = cols, rows
	}
	return g
}

// Count is the number of frames.
func (g Grid) Count() int { return g.Cols * g.Rows }

// Rect is the rectangle of frame i in image pixels; i must be in [0, Count).
func (g Grid) Rect(i int) image.Rectangle {
	f := g.File
	x := f.Margin + (i%g.Cols)*(f.FrameW+f.Spacing)
	y := f.Margin + (i/g.Cols)*(f.FrameH+f.Spacing)
	return image.Rect(x, y, x+f.FrameW, y+f.FrameH)
}

// At returns the frame under the image pixel p. It reports false for a pixel
// in the margin, in the spacing between cells or outside the grid.
func (g Grid) At(p image.Point) (int, bool) {
	f := g.File
	if g.Count() == 0 {
		return 0, false
	}
	x, y := p.X-f.Margin, p.Y-f.Margin
	if x < 0 || y < 0 {
		return 0, false
	}
	c, r := x/(f.FrameW+f.Spacing), y/(f.FrameH+f.Spacing)
	if c >= g.Cols || r >= g.Rows || x%(f.FrameW+f.Spacing) >= f.FrameW || y%(f.FrameH+f.Spacing) >= f.FrameH {
		return 0, false
	}
	return r*g.Cols + c, true
}

// Used is the part of the image the grid covers, from the margin to the last
// cell. Pixels beyond it are leftovers.
func (g Grid) Used() image.Rectangle {
	if g.Count() == 0 {
		return image.Rectangle{}
	}
	return g.Rect(0).Union(g.Rect(g.Count() - 1))
}

// Starter returns the sheet a new sheet tool starts from: 16 pixel cells, or
// the whole image when it is smaller, no margin and no spacing.
func Starter(image string, size image.Point) *File {
	return &File{Image: image, FrameW: max(1, min(16, size.X)), FrameH: max(1, min(16, size.Y))}
}
