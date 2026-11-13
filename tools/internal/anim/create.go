package anim

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/weakpixel/engine/anim/animfile"
)

// CreateOptions are the arguments of `weakpixel anim create`.
type CreateOptions struct {
	File  string // the Aseprite sheet JSON
	Out   string // output file, "-" for stdout; empty means File with .json replaced by .anim.json
	Force bool   // overwrite an existing output file
}

// Create writes the starter definition of opts.File. Warnings go to stderr.
func Create(opts CreateOptions, stdout, stderr io.Writer) error {
	out := opts.Out
	if out == "" {
		out = strings.TrimSuffix(opts.File, ".json") + animfile.Suffix
	}
	return create(opts.File, out, opts.Force, stdout, stderr)
}

// create writes the starter definition of the sheet at sheetPath to outPath
// ("-" is stdout).
func create(sheetPath, outPath string, force bool, stdout, stderr io.Writer) error {
	data, err := os.ReadFile(sheetPath)
	if err != nil {
		return err
	}
	sheet, err := animfile.DecodeSheet(data)
	if err != nil {
		return fmt.Errorf("%s: %w", sheetPath, err)
	}
	ref, err := sheetRef(sheetPath, outPath)
	if err != nil {
		return err
	}
	out, err := animfile.Encode(animfile.Starter(sheet, ref))
	if err != nil {
		return err
	}

	if outPath == "-" {
		_, err := stdout.Write(out)
		return err
	}
	if !strings.HasSuffix(outPath, animfile.Suffix) {
		fmt.Fprintf(stderr, "weakpixel anim create: warning: %s does not end in %s, so the anim loader reads it as an Aseprite export\n", outPath, animfile.Suffix)
	}
	if !force {
		if _, err := os.Stat(outPath); err == nil {
			return fmt.Errorf("%s exists; use --force to overwrite it", outPath)
		} else if !errors.Is(err, fs.ErrNotExist) {
			return err
		}
	}
	if err := os.WriteFile(outPath, out, 0o644); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "wrote %s (%d clips)\n", outPath, len(sheet.Tags))
	return nil
}

// sheetRef is the "sheet" value: the sheet's path relative to the output's
// directory, with forward slashes. For stdout the definition is assumed to
// go next to the sheet.
func sheetRef(sheetPath, outPath string) (string, error) {
	if outPath == "-" {
		return filepath.Base(sheetPath), nil
	}
	absSheet, err := filepath.Abs(sheetPath)
	if err != nil {
		return "", err
	}
	absOut, err := filepath.Abs(outPath)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(filepath.Dir(absOut), absSheet)
	if err != nil {
		return "", err
	}
	return filepath.ToSlash(rel), nil
}
