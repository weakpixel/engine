package gen

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestStaleFileRemoval checks that a package with no markers has its
// previously-generated files removed, while a hand-written file that
// happens to be named zz__gen.go (and so lacks the generated header) is
// left alone.
func TestStaleFileRemoval(t *testing.T) {
	t.Run("removes files the generator wrote", func(t *testing.T) {
		dir := t.TempDir()
		writeModule(t, dir, "stale", "")
		old := generatedHeader + "\n\npackage stale\n"
		if err := os.WriteFile(filepath.Join(dir, outputName), []byte(old), 0o644); err != nil {
			t.Fatal(err)
		}
		oldJSON := `{"version":3,"generator":"weakpixel gen","objects":[]}`
		if err := os.WriteFile(filepath.Join(dir, jsonOutputName), []byte(oldJSON), 0o644); err != nil {
			t.Fatal(err)
		}

		if err := run(dir, []string{"."}); err != nil {
			t.Fatal(err)
		}

		if _, err := os.Stat(filepath.Join(dir, outputName)); !os.IsNotExist(err) {
			t.Fatalf("zz__gen.go was not removed (stat err = %v)", err)
		}
		if _, err := os.Stat(filepath.Join(dir, jsonOutputName)); !os.IsNotExist(err) {
			t.Fatalf("zz__gen.god.json was not removed (stat err = %v)", err)
		}
	})

	t.Run("keeps a hand-written file with the same name", func(t *testing.T) {
		dir := t.TempDir()
		writeModule(t, dir, "stale2", "")
		handWritten := "package stale2\n\n// hand-written, not by the generator.\n"
		if err := os.WriteFile(filepath.Join(dir, outputName), []byte(handWritten), 0o644); err != nil {
			t.Fatal(err)
		}

		if err := run(dir, []string{"."}); err != nil {
			t.Fatal(err)
		}

		got, err := os.ReadFile(filepath.Join(dir, outputName))
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != handWritten {
			t.Fatalf("hand-written zz__gen.go was modified:\n%s", got)
		}
	})
}

// TestWriteSpecs checks the specs folder rules of D13: a package's GOD goes
// to <specs>/<group>.god.json, files it wrote under an earlier group and a
// stale zz__gen.god.json are removed, and a second package with the same
// group is an error instead of an overwrite.
func TestWriteSpecs(t *testing.T) {
	spec := func(pkg string) []byte {
		return []byte(`{"version":0,"generator":"weakpixel gen","package":"` + pkg + `"}`)
	}
	read := func(t *testing.T, path string) string {
		t.Helper()
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}

	dir, specs := t.TempDir(), filepath.Join(t.TempDir(), "god")
	if err := os.WriteFile(filepath.Join(dir, jsonOutputName), spec("example.com/a"), 0o644); err != nil {
		t.Fatal(err)
	}
	a := output{Dir: dir, PkgPath: "example.com/a", Group: "old", Content: []byte("package a\n"), JSON: spec("example.com/a")}
	if err := write(a, specs); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, jsonOutputName)); !os.IsNotExist(err) {
		t.Fatalf("zz__gen.god.json next to the sources was not removed (stat err = %v)", err)
	}
	if got := read(t, filepath.Join(specs, "old.god.json")); got != string(a.JSON) {
		t.Fatalf("old.god.json = %s", got)
	}

	// The package changes its group: the file under the old name goes.
	a.Group = "new"
	if err := write(a, specs); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(specs, "old.god.json")); !os.IsNotExist(err) {
		t.Fatalf("old.god.json was not removed (stat err = %v)", err)
	}
	read(t, filepath.Join(specs, "new.god.json"))

	// A hand-written file in the specs folder is never removed.
	hand := filepath.Join(specs, "hand.god.json")
	if err := os.WriteFile(hand, []byte(`{"objects":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}

	b := output{Dir: t.TempDir(), PkgPath: "example.com/b", Group: "new", Content: []byte("package b\n"), JSON: spec("example.com/b")}
	err := write(b, specs)
	want := `is generated from package example.com/a; package example.com/b has the same group "new"; set +game:group:name`
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("collision error = %v, want %q", err, want)
	}
	if got := read(t, filepath.Join(specs, "new.god.json")); got != string(a.JSON) {
		t.Fatalf("new.god.json was overwritten: %s", got)
	}
	read(t, hand)
}

// TestRunWritesGODToBuildGod checks the default of a project without a
// .weakpixel.json: the GOD is build output in <module root>/build/god, and
// nothing is written next to the sources.
func TestRunWritesGODToBuildGod(t *testing.T) {
	dir := t.TempDir()
	writeModule(t, dir, "plain", "// +game:component\ntype Pos struct{ X, Y float32 }\n")
	if err := run(dir, []string{"."}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, jsonOutputName)); !os.IsNotExist(err) {
		t.Errorf("zz__gen.god.json was written next to the sources (stat err = %v)", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "build", "god", "plain.god.json")); err != nil {
		t.Errorf("build/god/plain.god.json was not written: %v", err)
	}
}
