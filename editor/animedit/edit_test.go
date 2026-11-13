package animedit

import (
	"strings"
	"testing"

	"github.com/weakpixel/engine/anim/animfile"
)

func TestEdit(t *testing.T) {
	tests := []struct {
		name string
		do   func(f *animfile.File) error
		want string // substring of the encoded file
	}{
		{"default clip", func(f *animfile.File) error { return SetDefaultClip(f, "run") }, `"defaultClip": "run"`},
		{"remove default clip", func(f *animfile.File) error { return SetDefaultClip(f, "") }, ``},
		{"file pivot", func(f *animfile.File) error { return SetFilePivot(f, &animfile.Point{X: 5, Y: 6}) }, `"pivot": {
    "x": 5,
    "y": 6`},
		{"clip pivot lists the clip", func(f *animfile.File) error { return SetClipPivot(f, "idle", &animfile.Point{X: 1, Y: 1}) }, `"idle": {
      "pivot"`},
		{"loop off", func(f *animfile.File) error { return SetLoop(f, "run", LoopOff) }, `"loop": false`},
		{"loop sheet", func(f *animfile.File) error { return SetLoop(f, "run", LoopSheet) }, `"fps": 10`},
		{"fps", func(f *animfile.File) error { return SetFPS(f, "run", 24) }, `"fps": 24`},
		{"speed", func(f *animfile.File) error { return SetSpeed(f, "idle", 1.5) }, `"speed": 1.5`},
		{"durations from current", func(f *animfile.File) error {
			return SetFrameDuration(f, "idle", []int{100, 100}, 1, 250)
		}, `"durations": [
        100,
        250`},
		{"add body", func(f *animfile.File) error {
			_, err := AddBox(f, "idle", KindBody, 0, animfile.Rect{X: 0, Y: 0, W: 1, H: 1})
			return err
		}, `"w": 1,
      "h": 1`},
		{"add hit box on frame 1", func(f *animfile.File) error {
			ref, err := AddBox(f, "idle", KindHit, 1, animfile.Rect{X: 2, Y: -4, W: 3, H: 3})
			if err == nil && (ref.Body || ref.Index != 0) {
				t.Errorf("ref = %+v", ref)
			}
			return err
		}, `"idle": {
      "shapes"`},
		{"extend", func(f *animfile.File) error { return ExtendToFrame(f, "run", BoxRef{Index: 0}, 3) }, `"to": 3`},
		{"all frames", func(f *animfile.File) error { return SetShapeFrames(f, "run", BoxRef{Index: 0}, true, 0, nil) }, ``},
		{"shape id", func(f *animfile.File) error { return SetShapeID(f, "run", BoxRef{Index: 0}, "cut") }, `"id": "cut"`},
		{"shape kind", func(f *animfile.File) error { return SetShapeKind(f, "run", BoxRef{Index: 0}, KindHurt) }, ``},
		{"delete shape", func(f *animfile.File) error { return DeleteBox(f, "run", BoxRef{Index: 0}) }, ``},
		{"delete body", func(f *animfile.File) error { return DeleteBox(f, "run", BoxRef{Body: true}) }, ``},
		{"clear body", ClearBody, ``},
		{"clear durations", func(f *animfile.File) error { return ClearDurations(f, "run") }, ``},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := heroFile(t)
			if err := tc.do(f); err != nil {
				t.Fatal(err)
			}
			out := encode(t, f)
			if !strings.Contains(out, tc.want) {
				t.Errorf("encoded file lacks %q:\n%s", tc.want, out)
			}
			g, err := animfile.Decode([]byte(out))
			if err != nil {
				t.Fatalf("result does not decode: %v", err)
			}
			if _, findings := animfile.Diagnose(g, heroSheet(t)); hasError(findings) {
				t.Errorf("result has errors: %v", findings)
			}
		})
	}
}

func hasError(fs []animfile.Finding) bool {
	for _, f := range fs {
		if f.Level == animfile.LevelError {
			return true
		}
	}
	return false
}

func TestEditRefuses(t *testing.T) {
	tests := []struct {
		name string
		do   func(f *animfile.File) error
	}{
		{"negative fps", func(f *animfile.File) error { return SetFPS(f, "run", -1) }},
		{"negative speed", func(f *animfile.File) error { return SetSpeed(f, "run", -0.5) }},
		{"zero duration", func(f *animfile.File) error { return SetFrameDuration(f, "run", []int{1, 2}, 0, 0) }},
		{"duration frame out of range", func(f *animfile.File) error { return SetFrameDuration(f, "run", []int{1, 2}, 2, 5) }},
		{"empty rect", func(f *animfile.File) error { return SetBoxRect(f, "run", BoxRef{Index: 0}, animfile.Rect{W: 0, H: 1}) }},
		{"empty new box", func(f *animfile.File) error {
			_, err := AddBox(f, "run", KindHit, 0, animfile.Rect{W: 1})
			return err
		}},
		{"bad kind", func(f *animfile.File) error { return SetShapeKind(f, "run", BoxRef{Index: 0}, "soft") }},
		{"no such shape", func(f *animfile.File) error { return DeleteBox(f, "run", BoxRef{Index: 9}) }},
		{"body kind on a shape op", func(f *animfile.File) error { return SetShapeID(f, "run", BoxRef{Body: true}, "x") }},
		{"to before from", func(f *animfile.File) error { return SetShapeFrames(f, "run", BoxRef{Index: 0}, false, 3, ip(1)) }},
		{"negative from", func(f *animfile.File) error { return SetShapeFrames(f, "run", BoxRef{Index: 0}, false, -1, nil) }},
		{"loop mode", func(f *animfile.File) error { return SetLoop(f, "run", 7) }},
		{"unknown box kind", func(f *animfile.File) error {
			_, err := AddBox(f, "run", "soft", 0, animfile.Rect{W: 1, H: 1})
			return err
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := heroFile(t)
			before := encode(t, f)
			if err := tc.do(f); err == nil {
				t.Fatal("want an error")
			}
			if after := encode(t, f); after != before {
				t.Errorf("a refused edit changed the file:\n%s", after)
			}
		})
	}
}

func TestEditCopyToFrame(t *testing.T) {
	f := heroFile(t)
	ref, err := CopyToFrame(f, "run", BoxRef{Index: 0}, 4)
	if err != nil {
		t.Fatal(err)
	}
	s := f.Clips[0].Clip.Shapes[ref.Index]
	if s.Kind != KindHit || s.ID != "slash" || s.Frames == nil || s.Frames.From != 4 || s.Frames.To != nil {
		t.Errorf("copy = %+v", s)
	}
	if len(f.Clips[0].Clip.Shapes) != 3 {
		t.Errorf("shapes = %d, want 3", len(f.Clips[0].Clip.Shapes))
	}
}

func TestEditUnlistedClipEntryOnlyWhenNeeded(t *testing.T) {
	f := heroFile(t)
	for _, do := range []func() error{
		func() error { return SetFPS(f, "idle", 0) },
		func() error { return SetSpeed(f, "idle", 0) },
		func() error { return SetLoop(f, "idle", LoopSheet) },
		func() error { return ClearDurations(f, "idle") },
	} {
		if err := do(); err != nil {
			t.Fatal(err)
		}
	}
	if clipEntry(f, "idle") != nil {
		t.Error("clearing a value of an unlisted clip listed it")
	}
	if err := AddClipEntry(f, "idle"); err != nil || clipEntry(f, "idle") == nil {
		t.Errorf("AddClipEntry: %v", err)
	}
}
