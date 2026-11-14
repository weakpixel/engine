// Package schema implements `weakpixel schema`. It writes JSON Schemas
// (draft-07) for every JSON file kind of a game project, so an editor can
// complete and check them:
//
//	weakpixel schema [-o dir] [god inputs...]
//
// The fixed kinds (anim, sheet, atlas, bindings, library, weakpixel) are
// described by the tool. The level and prefab schemas come from the project's
// GOD files: the inputs are files, directories or "..." patterns as for
// `weakpixel doc`, by default god.specs of .weakpixel.json, else "./...".
// Without GOD files, level and prefab are skipped.
//
// The output folder is -o, else schema.out of .weakpixel.json, else
// build/schema. Each file is <kind>.schema.json. Files are generated; do not
// edit them. Exit codes: 0 ok, 1 failure, 2 usage error.
package schema

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"

	"github.com/weakpixel/engine/internal/project"
	"github.com/weakpixel/engine/tools/internal/cli"
	"github.com/weakpixel/engine/tools/internal/goddoc"
)

// DefaultOut is the output folder without -o and schema.out.
const DefaultOut = "build/schema"

// Options are the arguments of `weakpixel schema`.
type Options struct {
	Out    string   // output folder; empty means schema.out of .weakpixel.json, else DefaultOut
	Inputs []string // GOD files, directories or patterns; empty means god.specs, else ./...
}

// Run writes the schemas to the output folder and lists the files to stdout.
// A .weakpixel.json that cannot be read is a usage error (exit 2), as before.
func Run(opts Options, stdout io.Writer) error {
	settings, _, err := project.Load(".")
	if err != nil {
		return cli.Usagef("%v", err)
	}
	if settings == nil {
		settings = &project.Settings{}
	}
	inputs := opts.Inputs
	switch {
	case len(inputs) > 0:
	case settings.GODSpecs != "":
		inputs = []string{settings.GODSpecs}
	case isDir(project.DefaultSpecs(".")):
		inputs = []string{project.DefaultSpecs(".")}
	default:
		inputs = []string{"./..."}
	}
	dir := opts.Out
	if dir == "" {
		dir = settings.SchemaOut
	}
	if dir == "" {
		dir = DefaultOut
	}
	return generate(dir, inputs, stdout)
}

// generate builds every schema and writes them to dir; nothing is written if
// a fixed kind fails to build.
func generate(dir string, godInputs []string, stdout io.Writer) error {
	files := map[string]any{}
	for _, k := range kinds() {
		if k.raw != nil {
			d := map[string]any{"$schema": jsonSchemaURL, "title": k.title, "description": k.desc}
			for key, v := range k.raw {
				d[key] = v
			}
			files[k.name] = d
			continue
		}
		s, err := k.build()
		if err != nil {
			return fmt.Errorf("%s: %w", k.name, err)
		}
		files[k.name] = document(k.title, k.desc, s)
	}
	comps, objects, err := goddoc.Load(godInputs)
	switch {
	case err != nil:
		fmt.Fprintf(stdout, "level, prefab: skipped (no usable GOD files: %v)\n", err)
	default:
		files["level"] = documentSchema(false, comps, objects)
		files["prefab"] = documentSchema(true, comps, objects)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	for _, name := range sortedKeys(files) {
		data, err := json.MarshalIndent(files[name], "", "  ")
		if err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		path := filepath.Join(dir, name+".schema.json")
		if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
			return err
		}
		fmt.Fprintln(stdout, path)
	}
	return nil
}

func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}

// isDir reports whether path is an existing folder.
func isDir(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && fi.IsDir()
}
