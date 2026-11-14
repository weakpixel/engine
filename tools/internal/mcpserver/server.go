// Package mcpserver implements `weakpixel mcp`: an MCP server over stdio
// that lets an AI agent read and edit a weakpixel game project (components, object
// types, assets, levels and prefabs). The protocol layer is
// tools/internal/mcp; this package holds the tools. Design and decisions:
// doc/guide/tools.md#mcp.
//
// No tool runs code or writes outside the asset root: every path is an asset
// key or a path inside the asset root (see [Project.AssetPath]).
package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sync"

	"github.com/weakpixel/engine/tools/internal/mcp"
)

// Options are the arguments of `weakpixel mcp`.
type Options struct {
	ProjectOptions
	// Version is the server version reported to the client.
	Version string
}

// instructions tell the model how the tools fit together.
const instructions = "Tools for a weakpixel game project. Start with project_info. " +
	"list_components and list_object_types show the game's Game Object Definition (GOD); describe_component " +
	"and describe_object_type give one entry's schema. list_documents lists levels (*.level.json) and prefabs " +
	"(*.prefab.json); read_document returns a document with its problems and a sha, get_object one object. " +
	"Paths are asset keys, relative to the asset root (forest/forest.level.json). " +
	"Edit tools (set_component, set_field, add_component, remove_component, set_override, revert_override, place_object, " +
	"remove_object, set_asset_field, create_document, write_asset_document) change only the bytes of the edited value in " +
	"the file and refuse a result that would add a problem; pass the sha from read_document as expect to refuse stale edits."

// Run serves the MCP protocol on stdin and stdout until stdin closes or ctx
// is done. Diagnostics go to stderr; stdout carries protocol messages only.
func Run(ctx context.Context, opts Options, stdin io.Reader, stdout, stderr io.Writer) error {
	s, p, t, err := build(opts)
	if err != nil {
		return err
	}
	defer t.game.shutdown()
	fmt.Fprintf(stderr, "weakpixel mcp: project %s, asset root %q\n", p.Root, p.AssetRoot)
	for _, w := range p.Warnings {
		fmt.Fprintf(stderr, "weakpixel mcp: warning: %s\n", w)
	}
	return s.Serve(ctx, stdin, stdout)
}

// New opens the project and returns a server with every tool registered.
func New(opts Options) (*mcp.Server, *Project, error) {
	s, p, _, err := build(opts)
	return s, p, err
}

// build is New that also returns the tools, whose game must be shut down.
func build(opts Options) (*mcp.Server, *Project, *tools, error) {
	p, err := OpenProject(opts.ProjectOptions)
	if err != nil {
		return nil, nil, nil, err
	}
	version := opts.Version
	if version == "" {
		version = "(devel)"
	}
	s := mcp.NewServer(mcp.Info{Name: "weakpixel", Version: version}, instructions)
	t := &tools{p: p}
	t.game = newGameManager(t)
	s.Add(t.projectTools()...)
	s.Add(t.documentTools()...)
	s.Add(t.editTools()...)
	s.Add(t.objectEditTools()...)
	s.Add(t.assetEditTools()...)
	s.Add(t.gameTools()...)
	s.Add(t.liveTools()...)
	return s, p, t, nil
}

// tools holds what the tool handlers share.
type tools struct {
	p *Project
	// game runs the game the control tools drive.
	game *gameManager
	// docMu makes every read-modify-write of a document one step: tool calls
	// run concurrently, and so do the edits the game window sends.
	docMu sync.Mutex
}

// readOnly are the annotations of a tool that changes nothing.
var readOnly = mcp.Annotations{ReadOnly: true, Idempotent: true}

// tool builds an mcp.Tool whose handler decodes the arguments into A and
// returns fn's result as structured JSON.
func tool[A any](name, title, desc string, input *mcp.Schema, ann mcp.Annotations, fn func(ctx context.Context, args A) (any, error)) *mcp.Tool {
	return &mcp.Tool{
		Name: name, Title: title, Description: desc, Input: input, Annotations: ann,
		Handler: func(ctx context.Context, raw json.RawMessage) (*mcp.Result, error) {
			var args A
			if err := json.Unmarshal(raw, &args); err != nil {
				return nil, fmt.Errorf("arguments: %v", err)
			}
			out, err := fn(ctx, args)
			if err != nil {
				return nil, err
			}
			return mcp.JSONResult(out)
		},
	}
}
