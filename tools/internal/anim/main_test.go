package anim

import (
	"bytes"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"

	"github.com/weakpixel/asset"
	"github.com/weakpixel/engine/anim"
	"github.com/weakpixel/engine/render"
	testassets "github.com/weakpixel/engine/testdata/assets"
)

// Exit codes, as cli.Execute maps a nil and a plain error.
const (
	exitOK    = 0
	exitError = 1
)

// sheetJSON is an array-form export of four frames, all trimmed to start at
// (2,1), with the tags idle (0..1) and attack (2..3, ping-pong).
const sheetJSON = `{"frames":[
	{"filename":"f0","frame":{"x":0,"y":0,"w":4,"h":4},"trimmed":true,"spriteSourceSize":{"x":2,"y":1,"w":4,"h":4},"sourceSize":{"w":8,"h":8},"duration":100},
	{"filename":"f1","frame":{"x":4,"y":0,"w":4,"h":4},"trimmed":true,"spriteSourceSize":{"x":2,"y":1,"w":4,"h":4},"sourceSize":{"w":8,"h":8},"duration":100},
	{"filename":"f2","frame":{"x":8,"y":0,"w":4,"h":4},"trimmed":true,"spriteSourceSize":{"x":2,"y":1,"w":4,"h":4},"sourceSize":{"w":8,"h":8},"duration":100},
	{"filename":"f3","frame":{"x":12,"y":0,"w":4,"h":4},"trimmed":true,"spriteSourceSize":{"x":2,"y":1,"w":4,"h":4},"sourceSize":{"w":8,"h":8},"duration":100}
],"meta":{"image":"sheet.png","frameTags":[
	{"name":"idle","from":0,"to":1,"direction":"forward"},
	{"name":"attack","from":2,"to":3,"direction":"pingpong"}
]}}`

// writeFiles writes name -> content into a new temp dir and returns it.
func writeFiles(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// runValidate runs validate and returns the exit code, stdout and stderr.
func runValidate(files ...string) (int, string, string) {
	var stdout bytes.Buffer
	return exitCode(Validate(ValidateOptions{Files: files}, &stdout)), stdout.String(), ""
}

// runCreate runs create and returns the exit code, stdout and stderr; an
// error is the stderr text.
func runCreate(opts CreateOptions) (int, string, string) {
	var stdout, stderr bytes.Buffer
	err := Create(opts, &stdout, &stderr)
	if err != nil {
		stderr.WriteString(err.Error())
	}
	return exitCode(err), stdout.String(), stderr.String()
}

// exitCode is 0 for no error and 1 for an error, as cli.Execute maps them.
func exitCode(err error) int {
	if err != nil {
		return exitError
	}
	return exitOK
}

func mustContain(t *testing.T, s, want string) {
	t.Helper()
	if !strings.Contains(s, want) {
		t.Fatalf("output does not contain %q:\n%s", want, s)
	}
}

func TestValidate(t *testing.T) {
	// The same sheet with frame 0 trimmed differently, for the pivot warning.
	mixed := strings.Replace(sheetJSON, `"spriteSourceSize":{"x":2,"y":1,"w":4,"h":4},"sourceSize":{"w":8,"h":8},"duration":100},
	{"filename":"f1"`, `"spriteSourceSize":{"x":0,"y":3,"w":4,"h":4},"sourceSize":{"w":8,"h":8},"duration":100},
	{"filename":"f1"`, 1)
	dir := writeFiles(t, map[string]string{
		"sheet.json":         sheetJSON,
		"mixed.json":         mixed,
		"ok.anim.json":       `{"version":1,"sheet":"sheet.json","clips":{"idle":{"fps":8}}}`,
		"sub/up.anim.json":   `{"version":1,"sheet":"../sheet.json","clips":{"idle":{},"attack":{}}}`,
		"bad.anim.json":      `{"version":1,"sheet":"sheet.json","clips":{"jump":{},"idle":{"fps":0}}}`,
		"syntax.anim.json":   `{"version":1,`,
		"unknown.anim.json":  `{"version":1,"sheet":"sheet.json","pivto":{"x":1,"y":1}}`,
		"nosheet.anim.json":  `{"version":1,"sheet":"missing.json"}`,
		"badsheet.anim.json": `{"version":1,"sheet":"ok.anim.json.txt"}`,
		"ok.anim.json.txt":   `{"frames":[]}`,
		"pivot.anim.json":    `{"version":1,"sheet":"mixed.json","pivot":{"x":1,"y":1}}`,
		"nopivot.anim.json":  `{"version":1,"sheet":"mixed.json"}`,
		"plain.json":         `{"version":1,"sheet":"sheet.json"}`,
		"wide.json":          strings.Replace(sheetJSON, `"meta":{`, `"meta":{"size":{"w":15808,"h":65},`, 1),
		"wide.anim.json":     `{"version":1,"sheet":"wide.json"}`,
	})
	p := func(name string) string { return filepath.Join(dir, name) }

	tests := []struct {
		name  string
		file  string
		code  int
		want  []string // substrings of stdout
		avoid []string
	}{
		{"omitted tags are info", "ok.anim.json", exitOK, []string{
			`: info: tag "attack" is not listed in clips: it keeps the sheet's loop mode and timing`,
			": ok (2 tags, 1 listed, 1 omitted)",
		}, []string{"error:", "warn:"}},
		{"sheet relative to the file", "sub/up.anim.json", exitOK, []string{": ok (2 tags, 2 listed, 0 omitted)"}, []string{"info:"}},
		{"unknown tag and bad fps", "bad.anim.json", exitError, []string{
			`: error: clip "jump": no such tag in sheet "sheet.json"`,
			`: error: clip "idle": "fps" is 0, want > 0`,
			": 2 errors",
		}, []string{": ok"}},
		{"syntax error", "syntax.anim.json", exitError, []string{": error: unexpected EOF", ": 1 error"}, nil},
		{"unknown field", "unknown.anim.json", exitError, []string{`: error: json: unknown field "pivto"`}, nil},
		{"missing sheet", "nosheet.anim.json", exitError, []string{`: error: "sheet": open `}, nil},
		{"invalid sheet", "badsheet.anim.json", exitError, []string{`: error: sheet "ok.anim.json.txt": no "frames"`}, nil},
		{"uneven trim under a pivot warns", "pivot.anim.json", exitOK, []string{
			`: warn: clip "idle": frames are trimmed differently (offsets 0,3 and 2,1)`,
			": ok (2 tags, 0 listed, 2 omitted)",
		}, []string{`clip "attack"`}},
		{"uneven trim without a pivot is fine", "nopivot.anim.json", exitOK, nil, []string{"warn:"}},
		{"name without .anim.json warns", "plain.json", exitOK, []string{": warn: the name does not end in .anim.json"}, nil},
		{"sheet over 8192 warns", "wide.anim.json", exitOK, []string{`: warn: sheet "wide.json" is 15808x65; many laptops`}, nil},
		{"unreadable file", "nope.anim.json", exitError, []string{": error: open "}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, out, errOut := runValidate(p(tt.file))
			if code != tt.code {
				t.Fatalf("exit %d, want %d:\n%s%s", code, tt.code, out, errOut)
			}
			for _, w := range tt.want {
				mustContain(t, out, p(tt.file)+w)
			}
			for _, a := range tt.avoid {
				if strings.Contains(out, a) {
					t.Errorf("output contains %q:\n%s", a, out)
				}
			}
		})
	}
}

func TestValidateSeveralFiles(t *testing.T) {
	dir := writeFiles(t, map[string]string{
		"sheet.json":    sheetJSON,
		"ok.anim.json":  `{"version":1,"sheet":"sheet.json"}`,
		"bad.anim.json": `{"version":1,"sheet":"sheet.json","defaultClip":"jump"}`,
	})
	code, out, _ := runValidate(filepath.Join(dir, "ok.anim.json"), filepath.Join(dir, "bad.anim.json"))
	if code != exitError {
		t.Fatalf("exit %d, want %d", code, exitError)
	}
	mustContain(t, out, "ok.anim.json: ok")
	mustContain(t, out, `bad.anim.json: error: "defaultClip" "jump"`)
}

// wantStarter is what create writes for sheetJSON.
const wantStarter = `{
  "version": 1,
  "sheet": "sheet.json",
  "defaultClip": "idle",
  "clips": {
    "idle": {
      "loop": true
    },
    "attack": {
      "loop": true
    }
  }
}
`

func TestCreate(t *testing.T) {
	dir := writeFiles(t, map[string]string{"sheet.json": sheetJSON})
	sheet := filepath.Join(dir, "sheet.json")
	out := filepath.Join(dir, "sheet.anim.json")

	code, stdout, stderr := runCreate(CreateOptions{File: sheet})
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	mustContain(t, stdout, "wrote "+out+" (2 clips)")
	got, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != wantStarter {
		t.Fatalf("created file:\n%s\nwant:\n%s", got, wantStarter)
	}

	// The starter is valid and lists every tag.
	code, vout, _ := runValidate(out)
	if code != exitOK {
		t.Fatalf("validate exit %d:\n%s", code, vout)
	}
	mustContain(t, vout, ": ok (2 tags, 2 listed, 0 omitted)")

	// An existing file is kept unless --force.
	if code, _, stderr := runCreate(CreateOptions{File: sheet}); code != exitError || !strings.Contains(stderr, "exists; use --force") {
		t.Fatalf("second create: exit %d, stderr %q", code, stderr)
	}
	if code, _, stderr := runCreate(CreateOptions{File: sheet, Force: true}); code != exitOK {
		t.Fatalf("create --force: exit %d: %s", code, stderr)
	}
}

func TestCreateOutputPaths(t *testing.T) {
	dir := writeFiles(t, map[string]string{"art/sheet.json": sheetJSON})
	sheet := filepath.Join(dir, "art", "sheet.json")

	// Stdout: the definition is assumed to sit next to the sheet.
	code, stdout, stderr := runCreate(CreateOptions{File: sheet, Out: "-"})
	if code != exitOK || stdout != wantStarter {
		t.Fatalf("exit %d, stdout:\n%s\nstderr: %s", code, stdout, stderr)
	}

	// Elsewhere: "sheet" is relative to the output's directory.
	out := filepath.Join(dir, "defs", "hero.anim.json")
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		t.Fatal(err)
	}
	if code, _, stderr := runCreate(CreateOptions{File: sheet, Out: out}); code != exitOK {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	got, _ := os.ReadFile(out)
	mustContain(t, string(got), `"sheet": "../art/sheet.json"`)
	if code, vout, _ := runValidate(out); code != exitOK {
		t.Fatalf("validate: %s", vout)
	}

	// A name the loader would not route to the definition loader warns.
	if _, _, stderr := runCreate(CreateOptions{File: sheet, Out: filepath.Join(dir, "x.json")}); !strings.Contains(stderr, "warning: ") {
		t.Fatalf("no warning for a .json output: %q", stderr)
	}
}

func TestCreateTaglessSheet(t *testing.T) {
	dir := writeFiles(t, map[string]string{"s.json": `{"frames":[{"filename":"a","frame":{"x":0,"y":0,"w":4,"h":4},"duration":50}],"meta":{}}`})
	code, stdout, stderr := runCreate(CreateOptions{File: filepath.Join(dir, "s.json"), Out: "-"})
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	mustContain(t, stdout, `"defaultClip": "all"`)
	mustContain(t, stdout, `"all": {`)
}

func TestCreateBadSheet(t *testing.T) {
	dir := writeFiles(t, map[string]string{"s.json": `{"frames":{}}`})
	if code, _, stderr := runCreate(CreateOptions{File: filepath.Join(dir, "s.json")}); code != exitError || !strings.Contains(stderr, `no "frames"`) {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
	if code, _, _ := runCreate(CreateOptions{File: filepath.Join(dir, "missing.json")}); code != exitError {
		t.Fatalf("missing sheet: exit %d", code)
	}
}

// writeHero writes the test library's hero folder (sheet, image and a full
// definition with pivot, body and shapes) into a temp dir and returns it.
func writeHero(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for name, f := range testassets.Lib() {
		if path.Dir(name) != "global/hero" {
			continue
		}
		if err := os.WriteFile(filepath.Join(dir, path.Base(name)), f.Data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// TestHeroRoundTrip: a full definition validates clean; a starter created
// for the same sheet validates and loads with the real anim loader.
func TestHeroRoundTrip(t *testing.T) {
	dir := writeHero(t)
	full := filepath.Join(dir, "hero.anim.json")
	code, out, _ := runValidate(full)
	if code != exitOK || strings.Contains(out, "warn:") || strings.Contains(out, "error:") {
		t.Fatalf("exit %d:\n%s", code, out)
	}
	mustContain(t, out, full+": ok (2 tags, 2 listed, 0 omitted)")

	starter := filepath.Join(dir, "starter.anim.json")
	if code, _, stderr := runCreate(CreateOptions{File: filepath.Join(dir, "hero.aseprite.json"), Out: starter}); code != exitOK {
		t.Fatalf("create: %s", stderr)
	}
	if code, vout, _ := runValidate(starter); code != exitOK {
		t.Fatalf("validate:\n%s", vout)
	}

	srv := asset.NewServer(asset.Options{FS: os.DirFS(dir)})
	defer srv.Close()
	render.RegisterLoaders(srv, render.LoaderOptions{})
	anim.RegisterLoaders(srv)
	h, err := anim.LoadClipSetSync(srv.Global(), "starter.anim.json")
	if err != nil {
		t.Fatal(err)
	}
	set := anim.ClipSetStoreOf(srv).Get(h)
	if len(set.Clips) != 2 || set.Default != "idle" {
		t.Fatalf("loaded %d clips, Default %q", len(set.Clips), set.Default)
	}
	// No pivot: the clips leave Sprite.Origin to the sheet's default.
	c := anim.ClipStoreOf(srv).Get(set.Clips["idle"])
	if c.Loop != anim.Loop || len(c.Frames) != 2 || c.Frames[0].Origin.X != 0 || len(c.Frames[0].Boxes) != 0 {
		t.Fatalf("idle = loop %v, frames %+v", c.Loop, c.Frames)
	}
}
