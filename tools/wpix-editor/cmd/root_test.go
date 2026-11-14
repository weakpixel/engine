//go:build !(js && wasm)

package cmd

import (
	"bytes"
	"strings"
	"testing"

	"github.com/weakpixel/engine/tools/internal/cli"
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
		{"no args lists the tools", nil, 2, nil, "", "anim"},
		{"view", []string{"view", "a.png"}, 0, &native.Options{Tool: "view", File: "a.png"}, "", ""},
		{"view without a file", []string{"view"}, 2, nil, "", "accepts 1 arg"},
		{"voice", []string{"voice", "talk/kip.voice.json"}, 0, &native.Options{Tool: "voice", File: "talk/kip.voice.json"}, "", ""},
		{"view with two files", []string{"view", "a", "b"}, 2, nil, "", "accepts 1 arg"},
		{"anim with sheet", []string{"anim", "--sheet", "s.json", "a.anim.json"}, 0, &native.Options{Tool: "anim", File: "a.anim.json", Sheet: "s.json"}, "", ""},
		{"sheet after the file", []string{"anim", "a.anim.json", "--sheet", "s.json"}, 0, &native.Options{Tool: "anim", File: "a.anim.json", Sheet: "s.json"}, "", ""},
		{"sheet without value", []string{"anim", "a.anim.json", "--sheet"}, 2, nil, "", "needs an argument"},
		{"sheet for view", []string{"view", "--sheet", "s.json", "a.png"}, 2, nil, "", "unknown flag"},
		{"sheet for fx", []string{"fx", "--sheet", "s.json"}, 2, nil, "", "unknown flag"},
		{"sheet with image", []string{"sheet", "--image", "a.png", "a.sheet.json"}, 0, &native.Options{Tool: "sheet", File: "a.sheet.json", Image: "a.png"}, "", ""},
		{"sheet without image", []string{"sheet", "a.sheet.json"}, 0, &native.Options{Tool: "sheet", File: "a.sheet.json"}, "", ""},
		{"image for anim", []string{"anim", "--image", "a.png", "a.anim.json"}, 2, nil, "", "unknown flag"},
		{"fx without a file", []string{"fx"}, 0, &native.Options{Tool: "fx"}, "", ""},
		{"fx with a file", []string{"fx", "a.fx.json"}, 0, &native.Options{Tool: "fx", File: "a.fx.json"}, "", ""},
		{"fx with two files", []string{"fx", "a", "b"}, 2, nil, "", "between 0 and 1 arg"},
		{"unknown tool", []string{"paint", "a.png"}, 2, nil, "", `unknown command "paint"`},
		{"help view", []string{"help", "view"}, 0, nil, "Usage:", ""},
		{"view help flag", []string{"view", "--help"}, 0, nil, "Usage:", ""},
		{"help lists the tools", []string{"help"}, 0, nil, "fx", ""},
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

func TestNoArgsListsEveryTool(t *testing.T) {
	_, _, _, errOut := execute()
	for _, tool := range []string{"anim", "fx", "view", "voice"} {
		if !strings.Contains(errOut, tool) {
			t.Errorf("help lacks %q: %s", tool, errOut)
		}
	}
}
