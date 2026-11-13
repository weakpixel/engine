package animfile

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// Suffix is the file name suffix of an animation definition. Package anim
// routes ClipSet keys with it to the definition loader.
const Suffix = ".anim.json"

// Version is the only format version this package reads and writes.
const Version = 1

// File is an *.anim.json animation definition (version 1): gameplay data
// layered over an Aseprite export, its Sheet.
//
//	{
//	  "version": 1,
//	  "sheet": "hero.json",
//	  "pivot": {"x": 19, "y": 45},
//	  "defaultClip": "idle",
//	  "body": [{"x": -4, "y": -13, "w": 8, "h": 13}],
//	  "clips": {
//	    "slash": {"loop": false, "fps": 8,
//	              "shapes": [{"kind": "hit", "id": "slash", "frames": {"from": 2, "to": 3},
//	                          "rect": {"x": 6, "y": -11, "w": 37, "h": 10}}]}
//	  }
//	}
//
// All geometry is integer pixels: pivots in the trimmed sprite image, rects
// relative to the pivot, y down. Frame indices are in play order. Optional
// values are pointers so that "absent" differs from zero.
type File struct {
	Version     int    `json:"version"`
	Sheet       string `json:"sheet"`
	Pivot       *Point `json:"pivot,omitempty"`
	DefaultClip string `json:"defaultClip,omitempty"`
	Body        []Rect `json:"body,omitempty"`
	Clips       Clips  `json:"clips,omitempty"`
}

// Point is an integer point in pixels.
type Point struct {
	X int32 `json:"x"`
	Y int32 `json:"y"`
}

// Rect is an integer rectangle in pixels: X, Y is the top-left corner.
type Rect struct {
	X int32 `json:"x"`
	Y int32 `json:"y"`
	W int32 `json:"w"`
	H int32 `json:"h"`
}

// Clip holds the overrides of one clip.
type Clip struct {
	Loop      *bool    `json:"loop,omitempty"`
	Speed     *float64 `json:"speed,omitempty"`
	FPS       *int     `json:"fps,omitempty"`
	Durations []int    `json:"durations,omitempty"` // ms, one per frame
	Pivot     *Point   `json:"pivot,omitempty"`
	Shapes    []Shape  `json:"shapes,omitempty"`
}

// Shape is a hit or hurt box over a range of frames.
type Shape struct {
	Kind   string      `json:"kind"` // "hit" or "hurt"
	ID     string      `json:"id,omitempty"`
	Frames *FrameRange `json:"frames,omitempty"` // nil means every frame
	Rect   Rect        `json:"rect"`
}

// FrameRange is an inclusive range of play-order frame indices; a nil To
// means the single frame From.
type FrameRange struct {
	From int  `json:"from"`
	To   *int `json:"to,omitempty"`
}

// NamedClip is one entry of the "clips" object.
type NamedClip struct {
	Name string
	Clip Clip
}

// Clips is the "clips" object in file order. A Go map would lose the order
// and silently drop duplicate names; both matter for the error report (and
// for Encode, which writes them in slice order).
type Clips []NamedClip

// UnmarshalJSON implements json.Unmarshaler. Unknown fields inside a clip
// are errors.
func (c *Clips) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		return nil
	}
	// A custom unmarshaler gets a fresh decoder, so DisallowUnknownFields has
	// to be set again for the clip objects.
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if tok, err := dec.Token(); err != nil || tok != json.Delim('{') {
		return errors.New(`"clips" must be an object`)
	}
	for dec.More() {
		key, err := dec.Token()
		if err != nil {
			return err
		}
		name := key.(string)
		var d Clip
		if err := dec.Decode(&d); err != nil {
			return fmt.Errorf("clip %q: %w", name, err)
		}
		*c = append(*c, NamedClip{Name: name, Clip: d})
	}
	return nil
}

// MarshalJSON implements json.Marshaler: an object with the clips in slice
// order.
func (c Clips) MarshalJSON() ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteByte('{')
	for i, nc := range c {
		if i > 0 {
			buf.WriteByte(',')
		}
		key, err := json.Marshal(nc.Name)
		if err != nil {
			return nil, err
		}
		val, err := json.Marshal(nc.Clip)
		if err != nil {
			return nil, err
		}
		buf.Write(key)
		buf.WriteByte(':')
		buf.Write(val)
	}
	buf.WriteByte('}')
	return buf.Bytes(), nil
}

// Decode parses an animation definition. It rejects syntax errors, unknown
// fields, a float where an int is expected, a version other than [Version]
// and a missing "sheet" or one that is itself a definition. Everything that
// needs the sheet is checked by [Resolve].
func Decode(data []byte) (*File, error) {
	var f File
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&f); err != nil {
		return nil, err
	}
	var errs []error
	if f.Version != Version {
		errs = append(errs, fmt.Errorf(`"version" is %d, want %d`, f.Version, Version))
	}
	switch {
	case f.Sheet == "":
		errs = append(errs, errors.New(`"sheet" is required`))
	case strings.HasSuffix(f.Sheet, Suffix):
		errs = append(errs, fmt.Errorf(`"sheet" %q must be an Aseprite export, not another %s file`, f.Sheet, Suffix))
	}
	if err := errors.Join(errs...); err != nil {
		return nil, err
	}
	return &f, nil
}

// Encode writes f as indented JSON (two spaces, clips in slice order, a
// trailing newline). It does not validate f.
func Encode(f *File) ([]byte, error) {
	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}
