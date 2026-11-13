package gen

import (
	"flag"
	"os"
	"path/filepath"
	"testing"
)

var update = flag.Bool("update", false, "update golden files")

// TestGolden renders each testdata/golden/<case> package and compares it
// against its committed zz__gen.go.golden/zz__gen.god.json.golden. Run with
// -update to rewrite them after a deliberate change; review the diff like
// code.
func TestGolden(t *testing.T) {
	cases := []string{"simple", "zero", "many", "full", "docs", "entityref", "enum", "defaults", "editor"}
	for _, name := range cases {
		t.Run(name, func(t *testing.T) {
			dir := filepath.Join("testdata", "golden", name)
			outputs, err := generate(dir, []string{"."})
			if err != nil {
				t.Fatal(err)
			}
			if len(outputs) != 1 {
				t.Fatalf("generate() returned %d outputs, want 1", len(outputs))
			}
			out := outputs[0]
			checkGolden(t, filepath.Join(dir, "zz__gen.go.golden"), out.Content)
			checkGolden(t, filepath.Join(dir, "zz__gen.god.json.golden"), out.JSON)
		})
	}
}

func checkGolden(t *testing.T, path string, got []byte) {
	t.Helper()
	if *update {
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden file: %v (run with -update to create it)", err)
	}
	if string(want) != string(got) {
		t.Errorf("%s differs from golden output:\n--- got ---\n%s\n--- want ---\n%s", path, got, want)
	}
}

// TestIdempotent checks that generating a package twice produces identical
// bytes, and that write() doesn't rewrite a file whose content is unchanged.
func TestIdempotent(t *testing.T) {
	dir := t.TempDir()
	src := `// +game:object
type Thing struct {
	// +game:component
	Pos *Pos
}

// +game:component:group=idem
type Pos struct{ X, Y float64 }
`
	writeModule(t, dir, "idempotent", src)

	first, err := generate(dir, []string{"."})
	if err != nil {
		t.Fatal(err)
	}
	second, err := generate(dir, []string{"."})
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != 1 || len(second) != 1 {
		t.Fatalf("got %d/%d outputs, want 1/1", len(first), len(second))
	}
	if string(first[0].Content) != string(second[0].Content) {
		t.Fatal("generate() is not idempotent: zz__gen.go differs between runs")
	}
	if string(first[0].JSON) != string(second[0].JSON) {
		t.Fatal("generate() is not idempotent: zz__gen.god.json differs between runs")
	}

	if err := write(first[0], ""); err != nil {
		t.Fatal(err)
	}
	genPath := filepath.Join(dir, outputName)
	info1, err := os.Stat(genPath)
	if err != nil {
		t.Fatal(err)
	}

	if err := write(second[0], ""); err != nil {
		t.Fatal(err)
	}
	info2, err := os.Stat(genPath)
	if err != nil {
		t.Fatal(err)
	}
	if info1.ModTime() != info2.ModTime() {
		t.Error("write() rewrote a file whose content was unchanged")
	}
}

// writeModule writes a minimal package to dir: a go.mod rooted there (so
// packages.Load can resolve it independently of the enclosing module) and
// one source file.
func writeModule(t *testing.T, dir, pkgName, src string) {
	t.Helper()
	mod := "module " + pkgName + "\n\ngo 1.21\n"
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(mod), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, pkgName+".go"), []byte("package "+pkgName+"\n\n"+src), 0o644); err != nil {
		t.Fatal(err)
	}
}
