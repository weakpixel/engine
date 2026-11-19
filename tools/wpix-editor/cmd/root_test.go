//go:build !(js && wasm)

package cmd

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/weakpixel/engine/tools/internal/cli"
	"github.com/weakpixel/engine/tools/wpix-editor/internal/editorapp"
	"github.com/weakpixel/engine/tools/wpix-editor/internal/native"
)

// execute runs the command tree with a stub that records what would open.
func execute(args ...string) (code int, opened *native.Options, stdout, stderr string) {
	var out, errOut bytes.Buffer
	root := newRootCmd(func(o native.Options) error {
		opened = &o
		return nil
	})
	root.SetArgs(args)
	root.SetOut(&out)
	root.SetErr(&errOut)
	code = cli.Execute(root, &errOut)
	return code, opened, out.String(), errOut.String()
}

func TestExecute(t *testing.T) {
	tests := []struct {
		name   string
		args   []string
		code   int
		opened *native.Options
		out    string // substring of stdout
		err    string // substring of stderr
	}{
		{"no file lists the kinds", nil, 2, nil, "", ".sheet.json"},
		{"a texture", []string{"a.png"}, 0, &native.Options{File: "a.png"}, "", ""},
		{"a sheet", []string{"a.sheet.json"}, 0, &native.Options{File: "a.sheet.json"}, "", ""},
		{"a new sheet", []string{"--image", "a.png", "new.sheet.json"}, 0, &native.Options{File: "new.sheet.json", Image: "a.png"}, "", ""},
		{"a new definition", []string{"--sheet", "a.json", "new.anim.json"}, 0, &native.Options{File: "new.anim.json", Sheet: "a.json"}, "", ""},
		{"flag after the file", []string{"new.anim.json", "--sheet", "a.json"}, 0, &native.Options{File: "new.anim.json", Sheet: "a.json"}, "", ""},
		{"an effect", []string{"new.fx.json"}, 0, &native.Options{File: "new.fx.json"}, "", ""},
		{"a voice", []string{"talk/kip.voice.json"}, 0, &native.Options{File: "talk/kip.voice.json"}, "", ""},
		{"an Aseprite export", []string{"hero.aseprite.json"}, 0, &native.Options{File: "hero.aseprite.json"}, "", ""},
		{"two files", []string{"a.png", "b.png"}, 2, nil, "", "accepts 1 file, received 2"},
		{"sheet for a texture", []string{"--sheet", "s.json", "a.png"}, 2, nil, "", "--sheet applies to .anim.json files only, not to a.png"},
		{"image for a definition", []string{"--image", "a.png", "a.anim.json"}, 2, nil, "", "--image applies to .sheet.json files only"},
		{"sheet without value", []string{"a.anim.json", "--sheet"}, 2, nil, "", "needs an argument"},
		{"unknown suffix", []string{"notes.txt"}, 2, nil, "", "notes.txt is not a file wpix-editor opens (supported: .sheet.json"},
		{"the old tool command", []string{"view", "a.png"}, 2, nil, "", "view is not a file wpix-editor opens"},
		{"help", []string{"help"}, 0, nil, ".dialogue.json", ""},
		{"help flag", []string{"--help"}, 0, nil, "--image", ""},
		{"version", []string{"version"}, 0, nil, cli.Version(), ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, opened, out, errOut := execute(tt.args...)
			if code != tt.code {
				t.Fatalf("exit code = %d, want %d\nstdout: %s\nstderr: %s", code, tt.code, out, errOut)
			}
			if (opened == nil) != (tt.opened == nil) || (opened != nil && *opened != *tt.opened) {
				t.Errorf("opened = %+v, want %+v", opened, tt.opened)
			}
			if !strings.Contains(out, tt.out) {
				t.Errorf("stdout = %q, want %q", out, tt.out)
			}
			if !strings.Contains(errOut, tt.err) {
				t.Errorf("stderr = %q, want %q", errOut, tt.err)
			}
		})
	}
}

// TestHelpListsEveryKind: the help names every suffix of the kind table.
func TestHelpListsEveryKind(t *testing.T) {
	_, _, out, _ := execute("--help")
	for _, k := range editorapp.Kinds {
		for _, s := range k.Suffixes {
			if !strings.Contains(out, s) {
				t.Errorf("help lacks %q", s)
			}
		}
	}
}

// TestMissingFile runs the real native.Run up to the window: a missing
// definition without --sheet fails before any window opens, naming the flag.
func TestMissingFile(t *testing.T) {
	dir := t.TempDir()
	tests := []struct{ file, want string }{
		{"new.anim.json", "new.anim.json does not exist: give --sheet <export> to start a new definition"},
		{"new.sheet.json", "new.sheet.json does not exist: give --image <texture> to start a new sheet"},
		{"missing.png", "no such file"},
	}
	for _, tt := range tests {
		t.Run(tt.file, func(t *testing.T) {
			err := native.Run(native.Options{File: filepath.Join(dir, tt.file)})
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("Run = %v, want %q", err, tt.want)
			}
		})
	}
}
