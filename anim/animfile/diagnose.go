package animfile

import (
	"fmt"
	"strings"
)

// MaxPortableTexture is the largest texture side every target can load:
// WebGL's MAX_TEXTURE_SIZE is 8192 on many laptops, phones and browsers.
const MaxPortableTexture = 8192

// Level is the severity of a [Finding].
type Level string

// Finding levels, as printed by "weakpixel anim validate".
const (
	// LevelError is anything the loader rejects.
	LevelError Level = "error"
	// LevelWarn is legal but probably not what the author wants.
	LevelWarn Level = "warn"
	// LevelInfo is something worth knowing that needs no change.
	LevelInfo Level = "info"
)

// Finding is one line of a [Diagnose] report.
type Finding struct {
	Level Level
	// Clip names the clip a warning or info is about; empty otherwise (an
	// error line already names its clip and shape).
	Clip string
	Msg  string
}

// Diagnose checks f against its sheet s the way the loader does and adds the
// warnings and notes of "weakpixel anim validate". It is the one place both
// the command and the editor take their findings from.
//
// If [Resolve] fails, the result is nil and the findings are its errors, one
// per line. Otherwise it returns the resolved definition and the warnings
// (texture too large, uneven trims under a pivot) and infos (tags the file
// does not list), in that order.
func Diagnose(f *File, s *Sheet) (*Resolved, []Finding) {
	res, err := Resolve(f, s.PlayOrder())
	if err != nil {
		var out []Finding
		for line := range strings.SplitSeq(err.Error(), "\n") {
			out = append(out, Finding{Level: LevelError, Msg: line})
		}
		return nil, out
	}
	var out []Finding
	if s.Size.X > MaxPortableTexture || s.Size.Y > MaxPortableTexture {
		out = append(out, Finding{Level: LevelWarn, Msg: fmt.Sprintf(
			"sheet %q is %dx%d; many laptops, phones and browsers cannot load a texture over %d on either side, "+
				"so the sheet fails to load there: pack it into rows", f.Sheet, s.Size.X, s.Size.Y, MaxPortableTexture)})
	}
	for i, rc := range res.Clips {
		if rc.Pivot == nil {
			continue
		}
		if a, b, uneven := UnevenTrim(s, s.Tags[i]); uneven {
			out = append(out, Finding{Level: LevelWarn, Clip: rc.Name, Msg: fmt.Sprintf(
				"clip %q: frames are trimmed differently (offsets %d,%d and %d,%d); "+
					"the pivot is measured from each frame's trimmed image, so the art shifts between frames",
				rc.Name, a.X, a.Y, b.X, b.Y)})
		}
	}
	for _, name := range res.Omitted {
		out = append(out, Finding{Level: LevelInfo, Clip: name, Msg: fmt.Sprintf(
			"tag %q is not listed in clips: it keeps the sheet's loop mode and timing", name)})
	}
	return res, out
}

// UnevenTrim reports whether the frames of tag have different trim offsets,
// and two offsets that differ.
func UnevenTrim(s *Sheet, tag SheetTag) (a, b Point, uneven bool) {
	a = s.Frames[tag.From].Offset
	for i := tag.From + 1; i <= tag.To; i++ {
		if o := s.Frames[i].Offset; o != a {
			return a, o, true
		}
	}
	return a, a, false
}

// Starter returns the definition "weakpixel anim create" writes for sheet s,
// whose path relative to the definition is ref: every tag in sheet order as
// {"loop": true}, which keeps the sheet's behavior (a ping-pong tag stays
// ping-pong) and shows where "false" goes; the first tag as defaultClip; no
// pivot, so the sheet's default origin applies, and no boxes.
func Starter(s *Sheet, ref string) *File {
	f := &File{Version: Version, Sheet: ref, DefaultClip: s.Tags[0].Name}
	for _, t := range s.Tags {
		loop := true
		f.Clips = append(f.Clips, NamedClip{Name: t.Name, Clip: Clip{Loop: &loop}})
	}
	return f
}
