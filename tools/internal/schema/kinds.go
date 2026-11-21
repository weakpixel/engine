package schema

import (
	"slices"

	"github.com/weakpixel/engine/god"
	"github.com/weakpixel/engine/input"
)

// kind is one JSON file kind with a fixed shape.
type kind struct {
	// name is the output file's stem: <name>.schema.json.
	name  string
	title string
	desc  string
	// build returns the shape as a GOD TypeSchema.
	build func() (*god.TypeSchema, error)
	// raw is a ready JSON Schema for a shape TypeSchema cannot express; used
	// instead of build.
	raw map[string]any
}

// Small constructors keep the descriptions below readable.
func obj(desc string, required []string, props map[string]*god.TypeSchema) *god.TypeSchema {
	slices.Sort(required)
	return &god.TypeSchema{Type: "object", Description: desc, Properties: props, Required: required}
}

func str(desc string) *god.TypeSchema { return &god.TypeSchema{Type: "string", Description: desc} }
func boolean(desc string) *god.TypeSchema {
	return &god.TypeSchema{Type: "boolean", Description: desc}
}

func integer(desc string, min float64) *god.TypeSchema {
	return &god.TypeSchema{Type: "integer", Description: desc, Minimum: &min}
}

func number(desc string) *god.TypeSchema {
	return &god.TypeSchema{Type: "number", Description: desc}
}

func list(item *god.TypeSchema) *god.TypeSchema {
	return &god.TypeSchema{Type: "array", Items: item}
}

func point(desc string) *god.TypeSchema {
	return obj(desc, []string{"x", "y"}, map[string]*god.TypeSchema{"x": {Type: "integer"}, "y": {Type: "integer"}})
}

func rect(desc string) *god.TypeSchema {
	i := &god.TypeSchema{Type: "integer"}
	return obj(desc, []string{"x", "y", "w", "h"}, map[string]*god.TypeSchema{"x": i, "y": i, "w": i, "h": i})
}

// kinds lists the fixed-shape kinds. The Go types that load them are named
// in each description; tests check the shipped sample files against them.
func kinds() []kind {
	return []kind{
		{name: "anim", title: "weakpixel animation definition (*.anim.json)", build: animShape,
			desc: "Gameplay data layered over an Aseprite export (animfile.File)."},
		{name: "sheet", title: "weakpixel grid sheet (*.sheet.json)", build: sheetShape,
			desc: "A texture cut into a grid of equal frames (render grid sheet)."},
		{name: "atlas", title: "weakpixel atlas (*.atlas.json, TexturePacker)", raw: atlasShape(),
			desc: "TexturePacker JSON, hash or array form. Only the fields the engine reads are described."},
		{name: "bindings", title: "weakpixel input bindings (bindings.json)", build: bindingsShape,
			desc: "Default input bindings (input.BindingsFile)."},
		{name: "library", title: "weakpixel asset library config (library.json)", build: libraryShape,
			desc: "Optional overrides at the library root (library.Config)."},
		{name: "weakpixel", title: "weakpixel project settings (.weakpixel.json)", build: projectShape,
			desc: "Where the tools and the editor find the project's files (package project)."},
	}
}

func animShape() (*god.TypeSchema, error) {
	shape := obj("A hit or hurt box over a range of frames.", []string{"kind", "rect"}, map[string]*god.TypeSchema{
		"kind": str(`"hit" or "hurt".`),
		"id":   str("Names the shape for game code."),
		"frames": obj("Frame range in play order; absent means every frame.", []string{"from"}, map[string]*god.TypeSchema{
			"from": integer("First frame.", 0), "to": integer("Last frame; absent means the last one.", 0),
		}),
		"rect": rect("Rectangle relative to the pivot, y down."),
	})
	clip := obj("Overrides of one clip.", nil, map[string]*god.TypeSchema{
		"loop":      boolean("Loop the clip."),
		"speed":     number("Playback speed factor."),
		"fps":       integer("Frames per second; replaces the sheet's durations.", 1),
		"durations": list(integer("Milliseconds.", 1)),
		"pivot":     point("Pivot in the trimmed sprite image."),
		"shapes":    list(shape),
	})
	return obj("", []string{"version", "sheet"}, map[string]*god.TypeSchema{
		"version":     {Type: "integer", Description: "Format version, 1.", Minimum: god.Ptr(1.0), Maximum: god.Ptr(1.0)},
		"sheet":       str("Aseprite export, relative to this file."),
		"pivot":       point("Default pivot in the trimmed sprite image."),
		"defaultClip": str("Clip played when none is asked for."),
		"body":        list(rect("Body rectangle relative to the pivot.")),
		"clips":       {Type: "object", Description: "Clip overrides by tag name, in play order.", AdditionalProperties: clip},
	}), nil
}

func sheetShape() (*god.TypeSchema, error) {
	return obj("", []string{"image", "frameW", "frameH"}, map[string]*god.TypeSchema{
		"image":   str("Texture, relative to this file."),
		"frameW":  integer("Frame width in pixels.", 1),
		"frameH":  integer("Frame height in pixels.", 1),
		"margin":  integer("Border around the grid in pixels.", 0),
		"spacing": integer("Gap between frames in pixels.", 0),
		"pivot": obj("Pivot as a fraction of the frame.", []string{"x", "y"},
			map[string]*god.TypeSchema{"x": {Type: "number"}, "y": {Type: "number"}}),
	}), nil
}

// atlasShape is TexturePacker's JSON: "frames" is an object keyed by frame
// name or an array with a "filename" each, which TypeSchema cannot express.
func atlasShape() map[string]any {
	num := map[string]any{"type": "number"}
	rectS := map[string]any{
		"type": "object", "required": []string{"x", "y", "w", "h"},
		"properties": map[string]any{"x": num, "y": num, "w": num, "h": num},
	}
	frame := map[string]any{
		"type": "object", "required": []string{"frame"},
		"properties": map[string]any{
			"filename":         map[string]any{"type": "string", "description": "Frame name (array form)."},
			"frame":            rectS,
			"rotated":          map[string]any{"type": "boolean"},
			"trimmed":          map[string]any{"type": "boolean"},
			"spriteSourceSize": rectS,
			"sourceSize":       map[string]any{"type": "object", "properties": map[string]any{"w": num, "h": num}},
			"pivot":            map[string]any{"type": "object", "properties": map[string]any{"x": num, "y": num}},
		},
	}
	return map[string]any{
		"type": "object", "required": []string{"frames", "meta"},
		"properties": map[string]any{
			"frames": map[string]any{"oneOf": []any{
				map[string]any{"type": "object", "additionalProperties": frame},
				map[string]any{"type": "array", "items": frame},
			}},
			"meta": map[string]any{
				"type": "object", "required": []string{"image"},
				"properties": map[string]any{"image": map[string]any{"type": "string", "description": "Texture, relative to this file."}},
			},
		},
	}
}

func bindingsShape() (*god.TypeSchema, error) { return parseTypeSchema(input.BindingsSchema()) }

func libraryShape() (*god.TypeSchema, error) {
	return obj("", nil, map[string]*god.TypeSchema{
		"entries": {
			Type:                 "object",
			Description:          "Extra entry keys by group name: a font size to preload, or a file that should load on its own.",
			AdditionalProperties: list(str("Asset key, for example global/fonts/ui.ttf#16.")),
		},
	}), nil
}

// projectShape describes .weakpixel.json. Tests check it covers
// project.Keys() exactly.
func projectShape() (*god.TypeSchema, error) {
	section := func(desc string, props map[string]*god.TypeSchema) *god.TypeSchema {
		return obj(desc, nil, props)
	}
	min0 := god.Ptr(0.0)
	return obj("", nil, map[string]*god.TypeSchema{
		"god": section("Game Object definition files.", map[string]*god.TypeSchema{
			"specs": str("Folder of the per-package GOD files."),
			"out":   str("The merged project GOD."),
		}),
		"godDoc": section("Rendered GOD documentation.", map[string]*god.TypeSchema{"out": str("Output folder.")}),
		"schema": section("JSON schemas for editors.", map[string]*god.TypeSchema{"out": str("Folder wpix schema writes into.")}),
		"assets": section("The asset library.", map[string]*god.TypeSchema{
			"root":    str("Library root; asset keys are relative to it."),
			"prefabs": str("Folder of new prefabs under the root."),
		}),
		"editor": section("The VSCode level editor.", map[string]*god.TypeSchema{
			"game":        str("Directory of the package the editor builds to WebAssembly."),
			"tags":        list(str("Build tag.")),
			"snap":        {Type: "number", Description: "Drag grid in world units; 0 is off.", Minimum: min0},
			"screenshots": str("Folder screenshots are saved to, relative to this file."),
			"prefab": section("The prefab stage.", map[string]*god.TypeSchema{
				"background": str("Background colour as #rrggbb."),
				"grid":       {Type: "number", Description: "Grid step in world units; 0 draws no grid.", Minimum: min0},
			}),
		}),
	}), nil
}
