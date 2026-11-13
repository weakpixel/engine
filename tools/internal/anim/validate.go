package anim

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/weakpixel/engine/anim/animfile"
)

// ValidateOptions are the arguments of `weakpixel anim validate`.
type ValidateOptions struct {
	Files []string // the *.anim.json definitions to check
}

// Validate checks each definition against its sheet and prints the findings
// to stdout. It returns an error when any file has errors; the findings are
// already printed.
func Validate(opts ValidateOptions, stdout io.Writer) error {
	failed := 0
	for _, file := range opts.Files {
		if !validateFile(file, stdout) {
			failed++
		}
	}
	if failed > 0 {
		return fmt.Errorf("%d of %d files have errors", failed, len(opts.Files))
	}
	return nil
}

// report prints the findings of one file.
type report struct {
	file   string
	out    io.Writer
	errors int
}

func (r *report) print(level, msg string) {
	fmt.Fprintf(r.out, "%s: %s: %s\n", r.file, level, msg)
}

// error prints err, one line per problem: animfile joins its problems with
// newlines and gives every line its own context.
func (r *report) error(prefix string, err error) {
	for line := range strings.SplitSeq(err.Error(), "\n") {
		r.print("error", prefix+line)
		r.errors++
	}
}

// failed prints the summary line of a file with errors and reports false.
func (r *report) failed() bool {
	unit := "errors"
	if r.errors == 1 {
		unit = "error"
	}
	fmt.Fprintf(r.out, "%s: %d %s\n", r.file, r.errors, unit)
	return false
}

// validateFile checks one definition against its sheet and prints the
// findings. It reports whether the file is free of errors.
func validateFile(file string, out io.Writer) bool {
	r := &report{file: file, out: out}
	data, err := os.ReadFile(file)
	if err != nil {
		r.error("", err)
		return r.failed()
	}
	if !strings.HasSuffix(file, animfile.Suffix) {
		r.print("warn", fmt.Sprintf("the name does not end in %s, so the anim loader reads it as an Aseprite export", animfile.Suffix))
	}
	f, err := animfile.Decode(data)
	if err != nil {
		r.error("", err)
		return r.failed()
	}
	// Like the loader's dependency: relative to the definition's directory.
	sheetPath := filepath.Join(filepath.Dir(file), filepath.FromSlash(f.Sheet))
	sheetData, err := os.ReadFile(sheetPath)
	if err != nil {
		r.error(`"sheet": `, err)
		return r.failed()
	}
	sheet, err := animfile.DecodeSheet(sheetData)
	if err != nil {
		r.error(fmt.Sprintf("sheet %q: ", f.Sheet), err)
		return r.failed()
	}
	res, findings := animfile.Diagnose(f, sheet)
	for _, fd := range findings {
		r.print(string(fd.Level), fd.Msg)
		if fd.Level == animfile.LevelError {
			r.errors++
		}
	}
	if res == nil {
		return r.failed()
	}
	fmt.Fprintf(out, "%s: ok (%d tags, %d listed, %d omitted)\n",
		file, len(res.Clips), len(res.Clips)-len(res.Omitted), len(res.Omitted))
	return true
}
