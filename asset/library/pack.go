package library

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/weakpixel/asset"
)

// Pack is a mounted pack file: the files of one package in a zip archive,
// served at the package's folder while the pack is loaded. Packs are loaded
// by [Library.LoadPackage] and [Library.HoldPackage]; game code rarely
// sees them.
type Pack struct {
	// Package is the package the pack serves; empty for the placeholder.
	Package string
	// Files is the number of files in the pack.
	Files int
	fsys  fs.FS
}

// packData is a decoded pack: the zip directory, read on a worker.
type packData struct {
	pkg string
	zr  *zip.Reader
}

type packLoader struct{ lib *Library }

// Decode reads the zip directory. The archive is kept in memory: over HTTP
// a pack is downloaded whole, one request per package.
func (pl packLoader) Decode(ctx *asset.DecodeContext, data []byte) (packData, error) {
	pkg, ok := pl.lib.packOf[ctx.Key.Path]
	if !ok {
		return packData{}, fmt.Errorf("library: %s is not a pack of the catalog", ctx.Key.Path)
	}
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return packData{}, fmt.Errorf("library: pack %s: %w", ctx.Key.Path, err)
	}
	return packData{pkg: pkg, zr: zr}, nil
}

// Finish mounts the pack at its package's folder. On a hot reload the new
// version is mounted above the old one, which Unload then removes.
func (pl packLoader) Finish(_ *asset.FinishContext, d packData) (Pack, error) {
	pl.lib.fs.add(layerPack, d.pkg, d.zr)
	return Pack{Package: d.pkg, Files: len(d.zr.File), fsys: d.zr}, nil
}

// Unload unmounts the pack.
func (pl packLoader) Unload(p *Pack) {
	if p.fsys != nil {
		pl.lib.fs.remove(layerPack, p.Package, p.fsys)
	}
}

// precompressed lists extensions stored without compression in a pack:
// deflating them again costs time and saves nothing.
var precompressed = []string{".png", ".jpg", ".jpeg", ".ogg", ".mp3", ".zip"}

// packEpoch is the modification time of every pack member, so the same
// files always make the same pack (and the same content hash).
var packEpoch = time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)

// WritePacks builds a packed library from the loose library src into the
// directory out: one zip per package under packs/ (named
// "<package>.<sha256 prefix>.zip", members relative to the package folder),
// files outside the packages copied loose, and an assets.catalog.json that
// names the packs. Keys are the same as in src. The packs are
// deterministic: the same files give the same bytes and names.
//
// It returns the catalog and Scan's findings; it writes even when there
// are findings (weakpixel assets pack refuses errors before calling it).
func WritePacks(src fs.FS, out string) (*Catalog, []Finding, error) {
	cat, findings, err := Scan(src)
	if err != nil {
		return nil, nil, err
	}
	grouped := map[string]bool{}
	for i := range cat.Packages {
		g := &cat.Packages[i]
		data, err := zipPackage(src, g)
		if err != nil {
			return nil, nil, err
		}
		sum := sha256.Sum256(data)
		g.PackSHA256 = hex.EncodeToString(sum[:])
		g.Pack = packKey(g.Name, g.PackSHA256)
		g.PackSize = int64(len(data))
		if err := writeFile(out, g.Pack, data); err != nil {
			return nil, nil, err
		}
		for _, f := range g.Files {
			grouped[f] = true
		}
	}
	files, err := listFiles(src)
	if err != nil {
		return nil, nil, err
	}
	for _, f := range files {
		if grouped[f] || f == CatalogFile || f == ConfigFile {
			continue
		}
		data, err := fs.ReadFile(src, f)
		if err != nil {
			return nil, nil, err
		}
		if err := writeFile(out, f, data); err != nil {
			return nil, nil, err
		}
	}
	if err := writeFile(out, CatalogFile, cat.Encode()); err != nil {
		return nil, nil, err
	}
	return cat, findings, nil
}

// zipPackage returns the zip archive of g's files.
func zipPackage(src fs.FS, g *Package) ([]byte, error) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, f := range g.Files {
		data, err := fs.ReadFile(src, f)
		if err != nil {
			return nil, err
		}
		method := zip.Deflate
		lower := strings.ToLower(f)
		for _, ext := range precompressed {
			if strings.HasSuffix(lower, ext) {
				method = zip.Store
			}
		}
		w, err := zw.CreateHeader(&zip.FileHeader{
			Name:     strings.TrimPrefix(f, g.Name+"/"),
			Method:   method,
			Modified: packEpoch,
		})
		if err != nil {
			return nil, err
		}
		if _, err := w.Write(data); err != nil {
			return nil, err
		}
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// writeFile writes data to the key p below dir, creating folders.
func writeFile(dir, p string, data []byte) error {
	dst := filepath.Join(dir, filepath.FromSlash(p))
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0o644)
}
