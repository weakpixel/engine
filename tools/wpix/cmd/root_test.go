package cmd

import (
	"bytes"
	"strings"
	"testing"
)

func TestExecute(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantCode   int
		wantStdout string
		wantStderr string
	}{
		{"no args", nil, 2, "", "wpix needs a command"},
		{"help", []string{"help"}, 0, "framelog", ""},
		{"--help", []string{"--help"}, 0, "assets", ""},
		{"help command", []string{"help", "anim"}, 0, "", ""},
		{"help unknown", []string{"help", "nope"}, 2, "", `unknown command "nope"`},
		{"unknown", []string{"nope"}, 2, "", `unknown command "nope"`},
		{"version", []string{"version"}, 0, "(devel)", ""},
		{"--version", []string{"--version"}, 0, "(devel)", ""},
		{"doc bad format", []string{"doc", "--format", "pdf"}, 2, "", `unknown format "pdf"`},
		{"doc single-dash long flag", []string{"doc", "-format", "md"}, 2, "", "unknown shorthand flag"},
		{"gen bad flag", []string{"gen", "--x"}, 2, "", "unknown flag"},
		{"anim unknown", []string{"anim", "nope"}, 2, "", `wpix anim: unknown command "nope"`},
		{"anim without command", []string{"anim"}, 2, "", "wpix anim needs a command"},
		{"anim validate without files", []string{"anim", "validate"}, 2, "", "requires at least 1 arg"},
		{"anim create without --file", []string{"anim", "create"}, 2, "", "required flag(s)"},
		{"anim create with extra args", []string{"anim", "create", "--file", "a.json", "b.json"}, 2, "", "unknown command"},
		{"anim create bad flag", []string{"anim", "create", "--nope"}, 2, "", "unknown flag"},
		{"anim validate missing file", []string{"anim", "validate", "nope.anim.json"}, 1, "nope.anim.json: error", "wpix anim validate"},
		{"anim help", []string{"anim", "--help"}, 0, "validate", ""},
		{"assets without args", []string{"assets"}, 2, "", "wpix assets needs a command"},
		{"assets unknown", []string{"assets", "nope"}, 2, "", `unknown command "nope"`},
		{"assets two roots", []string{"assets", "check", "a", "b"}, 2, "", "accepts at most 1 arg"},
		{"assets pack old out argument", []string{"assets", "pack", "a", "b"}, 2, "", "accepts at most 1 arg"},
		{"assets json single dash", []string{"assets", "check", "-json", "a"}, 2, "", "unknown shorthand flag"},
		{"assets missing root", []string{"assets", "check", "/does/not/exist"}, 1, "", "is not a directory"},
		{"assets catalog bad flag", []string{"assets", "catalog", "--nope"}, 2, "", "unknown flag"},
		{"serve without dir", []string{"serve"}, 2, "", "accepts 1 arg"},
		{"serve missing dir", []string{"serve", "/does/not/exist"}, 2, "", "is not a directory"},
		{"new without dir", []string{"new"}, 2, "", "accepts 1 arg"},
		{"new bad group", []string{"new", "--no-run", "--name", "Bad", "/tmp/never-created"}, 2, "", "group"},
		{"schema bad flag", []string{"schema", "--nope"}, 2, "", "unknown flag"},
		{"schema missing value", []string{"schema", "-o"}, 2, "", "needs an argument"},
		{"tileset without args", []string{"tileset"}, 2, "", "wpix tileset needs a command"},
		{"tileset from-atlas without atlas", []string{"tileset", "from-atlas"}, 2, "", "requires at least 1 arg"},
		{"tileset from-atlas bad align", []string{"tileset", "from-atlas", "--align", "up", "a.xml"}, 2, "", "unknown alignment"},
		{"tileset from-atlas missing xml", []string{"tileset", "from-atlas", "nope.xml"}, 1, "", "nope.xml"},
		{"framelog negative skip", []string{"framelog", "--skip", "-1", "x.csv"}, 2, "", "--skip"},
		{"framelog missing file", []string{"framelog", "nope.csv"}, 1, "", "wpix framelog: "},
		{"framelog without file", []string{"framelog"}, 2, "", "accepts 1 arg"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := Execute(tc.args, &stdout, &stderr)
			if code != tc.wantCode {
				t.Errorf("exit code = %d, want %d (stderr %q)", code, tc.wantCode, stderr.String())
			}
			if !strings.Contains(stdout.String(), tc.wantStdout) {
				t.Errorf("stdout = %q, want it to contain %q", stdout.String(), tc.wantStdout)
			}
			if !strings.Contains(stderr.String(), tc.wantStderr) {
				t.Errorf("stderr = %q, want it to contain %q", stderr.String(), tc.wantStderr)
			}
		})
	}
}

// TestEveryCommandListed checks that help lists every command.
func TestEveryCommandListed(t *testing.T) {
	var stdout, stderr bytes.Buffer
	Execute([]string{"help"}, &stdout, &stderr)
	for _, name := range []string{"gen", "doc", "anim", "assets", "tileset", "schema", "framelog", "serve", "new", "version"} {
		if !strings.Contains(stdout.String(), "\n  "+name+" ") {
			t.Errorf("help does not list %q:\n%s", name, &stdout)
		}
	}
}
