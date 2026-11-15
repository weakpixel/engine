package library

import (
	"archive/zip"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
)

func TestLooseLibraryRefusesIgnoredKeys(t *testing.T) {
	lib, srv := open(t, ignoreFixture())
	_, err := fs.ReadFile(lib.FS(), "forest/raw/sketch.png")
	if !errors.Is(err, fs.ErrNotExist) || !strings.Contains(err.Error(), "library: forest/raw/sketch.png is ignored by forest/.assetignore:1") {
		t.Fatalf("read of an ignored key: %v", err)
	}
	if _, err := fs.Stat(lib.FS(), "global/art.psd"); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("stat of an ignored key: %v", err)
	}
	sc := srv.NewScope("t")
	if got := textOf(srv, sc, "forest/keep.psd"); got != "kept" {
		t.Fatalf("re-included key = %q", got)
	}
	if got := textOf(srv, sc, "global/art.psd"); got != "" {
		t.Fatalf("an ignored key loaded: %q", got)
	}
}

func TestOverlayServesIgnoredKey(t *testing.T) {
	lib, err := Open(ignoreFixture())
	if err != nil {
		t.Fatal(err)
	}
	lib.Mount("", fstest.MapFS{"global/art.psd": {Data: []byte("mod")}})
	if data, err := fs.ReadFile(lib.FS(), "global/art.psd"); err != nil || string(data) != "mod" {
		t.Fatalf("overlay: %q %v", data, err)
	}
}

func TestCatalogLibraryIsNotFiltered(t *testing.T) {
	// With a catalog the catalog is the file list; the base is read as is.
	fsys := ignoreFixture()
	cat, _, err := Scan(fsys)
	if err != nil {
		t.Fatal(err)
	}
	fsys[CatalogFile] = &fstest.MapFile{Data: cat.Encode()}
	lib, err := Open(fsys)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fs.ReadFile(lib.FS(), "global/art.psd"); err != nil {
		t.Fatalf("catalog library: %v", err)
	}
}

func TestWritePacksSkipsIgnoredFiles(t *testing.T) {
	src := ignoreFixture()
	src["notes.md"] = &fstest.MapFile{Data: []byte("root notes")}
	src["readme.txt"] = &fstest.MapFile{Data: []byte("root file")}
	src[".assetignore"] = &fstest.MapFile{Data: append(src[".assetignore"].Data, "notes.md\n"...)}
	out := t.TempDir()
	cat, _, err := WritePacks(src, out)
	if err != nil {
		t.Fatal(err)
	}
	var written []string
	err = filepath.WalkDir(out, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(out, p)
		written = append(written, filepath.ToSlash(rel))
		if strings.HasSuffix(p, ".zip") {
			zr, err := zip.OpenReader(p)
			if err != nil {
				return err
			}
			defer zr.Close()
			for _, f := range zr.File {
				written = append(written, rel+"!"+f.Name)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, w := range written {
		for _, bad := range []string{".assetignore", "raw/", ".psd", "notes.md", "drafts"} {
			if strings.Contains(w, bad) && !strings.HasSuffix(w, "keep.psd") {
				t.Errorf("packed build has %s", w)
			}
		}
	}
	if _, err := os.Stat(filepath.Join(out, "readme.txt")); err != nil {
		t.Errorf("root file not copied: %v", err)
	}

	// The same tree without the ignored files gives the same packs.
	clean := fstest.MapFS{}
	for p, f := range src {
		if ok, _ := mustIgnore(t, src).Match(p, false); !ok && filepath.Base(p) != IgnoreFile {
			clean[p] = f
		}
	}
	cat2, _, err := WritePacks(clean, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for i := range cat.Packages {
		if cat.Packages[i].Pack != cat2.Packages[i].Pack {
			t.Errorf("pack %s differs from the build without ignored files", cat.Packages[i].Name)
		}
	}
}

func mustIgnore(t *testing.T, fsys fs.FS) *Ignore {
	t.Helper()
	ig, err := ReadIgnore(fsys)
	if err != nil {
		t.Fatal(err)
	}
	return ig
}
