package cli

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// newTestRoot is a tree with a plain command, a failing one, a usage-error
// one, one with a required flag and a group.
func newTestRoot() *cobra.Command {
	root := Group(&cobra.Command{Use: "tool"})
	AddVersion(root)
	root.AddCommand(&cobra.Command{
		Use: "ok", Args: Args(cobra.ExactArgs(1)),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.Println("ok " + args[0])
			return nil
		},
	})
	root.AddCommand(&cobra.Command{
		Use:  "fail",
		RunE: func(*cobra.Command, []string) error { return errors.New("it broke") },
	})
	root.AddCommand(&cobra.Command{
		Use:  "bad",
		RunE: func(*cobra.Command, []string) error { return Usagef("bad input") },
	})
	req := &cobra.Command{Use: "req", RunE: func(*cobra.Command, []string) error { return nil }}
	req.Flags().String("name", "", "name")
	_ = req.MarkFlagRequired("name")
	root.AddCommand(req)
	grp := Group(&cobra.Command{Use: "grp"})
	grp.AddCommand(&cobra.Command{Use: "sub", Run: func(*cobra.Command, []string) {}})
	root.AddCommand(grp)
	return root
}

func TestExecute(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		code       int
		stdout     string // substring
		stderr     string // substring
		noUsageErr bool   // stderr must not contain "Usage:"
	}{
		{"success", []string{"ok", "x"}, 0, "ok x", "", true},
		{"plain error", []string{"fail"}, 1, "", "tool fail: it broke", true},
		{"usage error", []string{"bad"}, 2, "", "tool bad: bad input", true},
		{"unknown flag", []string{"ok", "--nope", "x"}, 2, "", "unknown flag", true},
		{"wrong arg count", []string{"ok"}, 2, "", "accepts 1 arg", true},
		{"unknown subcommand", []string{"nope"}, 2, "", `unknown command "nope"`, true},
		{"missing required flag", []string{"req"}, 2, "", "required flag", true},
		{"no command", nil, 2, "", "tool needs a command", false},
		{"group without subcommand", []string{"grp"}, 2, "", "tool grp needs a command", false},
		{"unknown subcommand of a group", []string{"grp", "zz"}, 2, "", `unknown command "zz"`, true},
		{"help", []string{"--help"}, 0, "Usage:", "", true},
		{"help command", []string{"help", "ok"}, 0, "Usage:", "", true},
		{"version flag", []string{"--version"}, 0, Version(), "", true},
		{"version command", []string{"version"}, 0, Version(), "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			root := newTestRoot()
			root.SetOut(&stdout)
			root.SetErr(&stderr)
			root.SetArgs(tt.args)
			if code := Execute(root, &stderr); code != tt.code {
				t.Errorf("exit code = %d, want %d\nstdout: %s\nstderr: %s", code, tt.code, &stdout, &stderr)
			}
			if !strings.Contains(stdout.String(), tt.stdout) {
				t.Errorf("stdout = %q, want %q", &stdout, tt.stdout)
			}
			if !strings.Contains(stderr.String(), tt.stderr) {
				t.Errorf("stderr = %q, want %q", &stderr, tt.stderr)
			}
			if tt.noUsageErr && strings.Contains(stderr.String(), "Usage:") {
				t.Errorf("stderr has the usage text: %q", &stderr)
			}
		})
	}
}
