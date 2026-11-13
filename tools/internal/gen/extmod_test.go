package gen

import (
	"os/exec"
	"testing"
)

// TestExtmod drives testdata/extmod, a separate module with its own go.mod
// and filesystem replacements to this checkout. It declares an object using
// a component from its own package and one from the engine's
// github.com/weakpixel/engine/gfx. It proves the generator
// and its generated code work from outside the weakpixel module, using only
// exported ecs API. Skipped under -short or when go isn't on PATH.
func TestExtmod(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping module build/test in -short mode")
	}
	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Skip("go not found on PATH")
	}

	dir := "testdata/extmod"
	for _, args := range [][]string{
		{"generate", "./..."},
		{"build", "./..."},
		{"test", "./..."},
	} {
		cmd := exec.Command(goBin, args...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("go %v: %v\n%s", args, err, out)
		}
	}
}
