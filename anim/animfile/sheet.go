package animfile

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"
)

// AllClip is the name of the one clip of a sheet that has no tags: every
// frame, in order, looping.
const AllClip = "all"

// LoopMode says what a clip does at its last frame. It mirrors anim.LoopMode
// (same names, same order), which package anim converts to.
type LoopMode uint8

// Loop modes.
const (
	// Loop restarts from the first frame.
	Loop LoopMode = iota
	// Once stops on the last frame.
	Once
	// PingPong plays forward, then backward, without repeating the end frames.
	PingPong
)

// Sheet is an Aseprite "Sprite sheet" export (File > Export Sprite Sheet,
// JSON data with "Meta: Tags" checked), hash or array form:
//
//	{
//	  "frames": { "hero 0.aseprite": { "frame": {...}, "duration": 100,
//	                                   "spriteSourceSize": {"x":73,"y":54,...} }, ... },
//	  "meta": { "image": "hero.png",
//	            "frameTags": [ {"name":"run","from":2,"to":5,"direction":"forward"} ] }
//	}
//
// Only what animation needs is read: per-frame durations and trim offsets,
// and the tags. The frame rectangles belong to the atlas loader, which reads
// the same file and keeps the same (file) order.
type Sheet struct {
	// Frames are the frames in file order.
	Frames []SheetFrame
	// Tags are the clips in file order. A file without tags has one tag,
	// AllClip, over every frame.
	Tags []SheetTag
	// Size is the sheet image's size in pixels ("meta.size"); zero when
	// absent.
	Size Point
}

// SheetFrame is one frame of a Sheet.
type SheetFrame struct {
	// Name is the frame's key (hash form) or "filename" (array form).
	Name string
	// Duration is the positive frame duration.
	Duration time.Duration
	// Trimmed reports whether the export trimmed the frame's transparent
	// border ("trimmed").
	Trimmed bool
	// Offset is where the trimmed image starts in the untrimmed frame
	// ("spriteSourceSize" x, y); zero when absent.
	Offset Point
}

// SheetTag is one Aseprite tag: a clip over the inclusive file frame range
// From..To.
//
// Directions: forward and reverse loop; pingpong and pingpong_reverse
// ping-pong. Reverse directions play the frames backward. A tag's "repeat"
// is ignored.
type SheetTag struct {
	Name     string
	From, To int
	Reverse  bool
	Loop     LoopMode
}

type sheetFile struct {
	Frames json.RawMessage `json:"frames"`
	Meta   struct {
		FrameTags []sheetTagJSON `json:"frameTags"`
		Size      struct {
			W int32 `json:"w"`
			H int32 `json:"h"`
		} `json:"size"`
	} `json:"meta"`
}

type sheetTagJSON struct {
	Name      string `json:"name"`
	From      int    `json:"from"`
	To        int    `json:"to"`
	Direction string `json:"direction"`
}

type sheetFrameJSON struct {
	Filename         string `json:"filename"` // array form only
	Duration         int    `json:"duration"`
	Trimmed          bool   `json:"trimmed"`
	SpriteSourceSize *struct {
		X, Y int32
	} `json:"spriteSourceSize"`
}

// DecodeSheet parses and checks an Aseprite export. Unknown fields are
// allowed (the export has many). The frames are checked first; if they are
// fine, every tag problem is reported, joined into one error.
func DecodeSheet(data []byte) (*Sheet, error) {
	var f sheetFile
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, err
	}
	frames, err := sheetFrames(f.Frames)
	if err != nil {
		return nil, err
	}
	tags, err := sheetTags(f.Meta.FrameTags, len(frames))
	if err != nil {
		return nil, err
	}
	size := Point{X: f.Meta.Size.W, Y: f.Meta.Size.H}
	return &Sheet{Frames: frames, Tags: tags, Size: size}, nil
}

// sheetFrames reads the "frames" value, an object or an array, in file order
// (a Go map would lose it). Every frame needs a positive "duration".
func sheetFrames(raw json.RawMessage) ([]SheetFrame, error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return nil, errors.New(`"frames" is required`)
	}
	var frames []sheetFrameJSON
	switch raw[0] {
	case '[':
		if err := json.Unmarshal(raw, &frames); err != nil {
			return nil, err
		}
	case '{':
		dec := json.NewDecoder(bytes.NewReader(raw))
		if _, err := dec.Token(); err != nil { // the opening '{'
			return nil, err
		}
		for dec.More() {
			key, err := dec.Token()
			if err != nil {
				return nil, err
			}
			var fr sheetFrameJSON
			if err := dec.Decode(&fr); err != nil {
				return nil, err
			}
			fr.Filename = key.(string)
			frames = append(frames, fr)
		}
	default:
		return nil, errors.New(`"frames" must be an object or an array`)
	}
	if len(frames) == 0 {
		return nil, errors.New(`no "frames"`)
	}
	out := make([]SheetFrame, len(frames))
	var errs []error
	for i, fr := range frames {
		if fr.Duration <= 0 {
			errs = append(errs, fmt.Errorf("frame %d (%q) has no positive duration", i, fr.Filename))
		}
		out[i] = SheetFrame{Name: fr.Filename, Duration: time.Duration(fr.Duration) * time.Millisecond, Trimmed: fr.Trimmed}
		if s := fr.SpriteSourceSize; s != nil {
			out[i].Offset = Point{X: s.X, Y: s.Y}
		}
	}
	return out, errors.Join(errs...)
}

// sheetTags validates the tags against the frame count, collecting every
// problem. No tags means one clip of all frames.
func sheetTags(tags []sheetTagJSON, frames int) ([]SheetTag, error) {
	if len(tags) == 0 {
		return []SheetTag{{Name: AllClip, From: 0, To: frames - 1, Loop: Loop}}, nil
	}
	out := make([]SheetTag, 0, len(tags))
	seen := make(map[string]bool, len(tags))
	var errs []error
	for _, t := range tags {
		tag := SheetTag{Name: t.Name, From: t.From, To: t.To}
		switch t.Direction {
		case "", "forward":
			tag.Loop = Loop
		case "reverse":
			tag.Loop, tag.Reverse = Loop, true
		case "pingpong":
			tag.Loop = PingPong
		case "pingpong_reverse":
			tag.Loop, tag.Reverse = PingPong, true
		default:
			errs = append(errs, fmt.Errorf("tag %q: unknown direction %q", t.Name, t.Direction))
		}
		switch {
		case t.Name == "":
			errs = append(errs, errors.New("a tag has no name"))
		case seen[t.Name]:
			errs = append(errs, fmt.Errorf("duplicate tag name %q", t.Name))
		}
		seen[t.Name] = true
		if t.From < 0 || t.To < t.From || t.To >= frames {
			errs = append(errs, fmt.Errorf("tag %q: frames %d..%d are outside the file's %d frames", t.Name, t.From, t.To, frames))
			continue
		}
		out = append(out, tag)
	}
	return out, errors.Join(errs...)
}

// FrameIndices returns the tag's file frame indices in play order (backward
// for a reverse tag).
func (t SheetTag) FrameIndices() []int {
	idx := make([]int, 0, t.To-t.From+1)
	for i := t.From; i <= t.To; i++ {
		idx = append(idx, i)
	}
	if t.Reverse {
		slices.Reverse(idx)
	}
	return idx
}

// PlayOrder returns the sheet's tags as [Resolve] input: names, loop modes
// and durations in play order.
func (s *Sheet) PlayOrder() []Tag {
	tags := make([]Tag, len(s.Tags))
	for i, t := range s.Tags {
		tag := Tag{Name: t.Name, Loop: t.Loop}
		for _, fi := range t.FrameIndices() {
			tag.Durations = append(tag.Durations, s.Frames[fi].Duration)
		}
		tags[i] = tag
	}
	return tags
}
