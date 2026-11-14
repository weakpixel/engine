package newgame

import (
	"fmt"
	"io"
	"os/exec"
	"strings"
)

// postSteps makes the new project ready to open: git init (unless --no-git,
// and only when git is installed), go mod tidy for go.sum and the tool's
// dependencies, go generate ./... for zz__gen.go and the GOD file, and go
// build ./... to prove it compiles. It stops at the first failing step and
// returns its output.
func postSteps(o Options, stdout, stderr io.Writer) error {
	steps := [][]string{
		{"go", "mod", "tidy"},
		{"go", "generate", "./..."},
		{"go", "build", "./..."},
	}
	if !o.NoGit {
		if _, err := exec.LookPath("git"); err == nil {
			steps = append([][]string{{"git", "init", "-q"}}, steps...)
		} else {
			fmt.Fprintln(stderr, "wpix new: git not found, skipped git init")
		}
	}
	for _, s := range steps {
		fmt.Fprintf(stdout, "$ %s\n", strings.Join(s, " "))
		cmd := exec.Command(s[0], s[1:]...)
		cmd.Dir = o.Dir
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("%s: %v\n%s", strings.Join(s, " "), err, out)
		}
	}
	return nil
}
