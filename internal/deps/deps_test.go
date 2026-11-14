// Package deps holds the test that enforces the package dependency rules of
// doc/design/architecture.md. The module also contains Ebiten, so the module
// boundary does not keep it out of the headless packages.
package deps

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"testing"
)

const root = "github.com/weakpixel/engine/"

// ecsModule is the module path of the ECS core (package ecs, ecs/system and
// its internal packages), a dependency of the engine module.
const ecsModule = "github.com/weakpixel/ecs"

// isECS reports whether dep is a package of the ECS module.
func isECS(dep string) bool {
	return dep == ecsModule || strings.HasPrefix(dep, ecsModule+"/")
}

// assetModule is the module path of the asset system (package asset and
// httpfs), a dependency of the engine module.
const assetModule = "github.com/weakpixel/asset"

// isAsset reports whether dep is a package of the asset module.
func isAsset(dep string) bool {
	return dep == assetModule || strings.HasPrefix(dep, assetModule+"/")
}

// listDeps returns the import paths pkg depends on, transitively, that are
// not part of the standard library.
func listDeps(t *testing.T, pkg string) []string {
	t.Helper()
	return listDepsEnv(t, pkg)
}

// listDepsEnv is listDeps with extra environment variables (GOOS=js and
// GOARCH=wasm list what the browser build links).
func listDepsEnv(t *testing.T, pkg string, env ...string) []string {
	t.Helper()
	cmd := exec.Command("go", "list", "-deps", "-f", "{{if not .Standard}}{{.ImportPath}}{{end}}", pkg)
	cmd.Env = append(cmd.Environ(), env...)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("go list -deps %s: %v", pkg, err)
	}
	var deps []string
	for _, line := range strings.Fields(string(out)) {
		if line != pkg {
			deps = append(deps, line)
		}
	}
	return deps
}

func TestPackageDependencyRules(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go tool not found")
	}
	tests := []struct {
		name string
		pkgs []string
		// allowed reports whether dep may appear in the dependencies of the
		// packages; the reason is quoted in the failure message.
		allowed func(dep string) bool
		reason  string
	}{
		{
			name:    "god uses the standard library and the ECS module only",
			pkgs:    []string{"god"},
			allowed: func(dep string) bool { return isECS(dep) || strings.HasPrefix(dep, root+"god") },
			reason:  "god may depend only on the standard library and the github.com/weakpixel/ecs module",
		},
		{
			name:    "the MCP layer uses the standard library only",
			pkgs:    []string{"tools/internal/mcp"},
			allowed: func(dep string) bool { return false },
			reason:  "tools/internal/mcp may depend on nothing but the standard library (no MCP SDK, no weakpixel package)",
		},
		{
			name:    "the marker parser and enum lister use the standard library only",
			pkgs:    []string{"internal/marker", "internal/marker/enum"},
			allowed: func(dep string) bool { return false },
			reason:  "internal/marker and internal/marker/enum are standalone-ready libraries and may depend on nothing but the standard library",
		},
		{
			name:    "the shader parameter markers need only the marker library",
			pkgs:    []string{"internal/shaderparam"},
			allowed: func(dep string) bool { return dep == root+"internal/marker" },
			reason:  "internal/shaderparam may import the standard library and internal/marker only (no Ebiten, no render, no ecs)",
		},
		{
			name:    "the JSON splice helper uses the standard library only",
			pkgs:    []string{"internal/jsonedit"},
			allowed: func(dep string) bool { return false },
			reason:  "internal/jsonedit may depend on nothing but the standard library (it is shared by tools and the engine)",
		},
		{
			name:    "xmath uses the standard library only",
			pkgs:    []string{"xmath"},
			allowed: func(dep string) bool { return false },
			reason:  "xmath may depend on nothing but the standard library",
		},
		{
			name: "headless engine packages and tools do not import Ebiten",
			pkgs: []string{
				"xmath", "asset/library", "assetkind", "input", "gfx", "anim/animfile", "sheetfile", "tiled",
				"particles/fxfile", "tween", "collision", "physics",
				// The command-line tools run without a display. Their tests may
				// still use render; listDeps ignores test imports.
				"tools/internal/anim", "tools/internal/assets", "tools/internal/framelog", "tools/internal/newgame", "tools/internal/goddoc", "tools/internal/gen", "tools/internal/mcpserver", "tools/weakpixel", "tools/weakpixel/cmd",
			},
			allowed: func(dep string) bool {
				return !strings.Contains(dep, "hajimehoshi/ebiten") && !strings.Contains(dep, "ebitengine")
			},
			reason: "these packages must stay free of Ebiten",
		},
		{
			name:    "input uses the standard library only",
			pkgs:    []string{"input"},
			allowed: func(dep string) bool { return false },
			reason:  "input may depend on nothing but the standard library: the bindings asset is in input/inputsys",
		},
		{
			name:    "the host link uses the standard library only",
			pkgs:    []string{"editor/hostlink"},
			allowed: func(dep string) bool { return false },
			reason:  "editor/hostlink may depend on nothing but the standard library: the bridge and the tool shell both build on it",
		},
		{
			name:    "the asset kind registry imports only the asset module",
			pkgs:    []string{"assetkind"},
			allowed: isAsset,
			reason:  "assetkind may import the standard library and github.com/weakpixel/asset only, so every package that owns an asset kind (render included) can import it",
		},
		{
			name: "the resolver knows no asset kind",
			pkgs: []string{"resolver"},
			allowed: func(dep string) bool {
				return isECS(dep) || isAsset(dep) || dep == root+"god" || dep == root+"assetkind"
			},
			reason: "resolver may import only ecs, god, the asset module and assetkind: the packages that own an asset kind register it there",
		},
		{
			name:    "the asset library imports only the asset module",
			pkgs:    []string{"asset/library"},
			allowed: isAsset,
			reason:  "asset/library may import the standard library and github.com/weakpixel/asset only (and the asset module itself only the standard library)",
		},
		{
			// gfx brings ecs, god and the asset module along through its
			// generated Game Object code; nothing else is allowed, so the
			// loader (render, asset loading) stays in particles.
			name: "the fx file format imports only gfx and xmath",
			pkgs: []string{"particles/fxfile"},
			allowed: func(dep string) bool {
				return isECS(dep) || isAsset(dep) || dep == root+"god" || dep == root+"gfx" || dep == root+"xmath"
			},
			reason: "particles/fxfile may import only gfx and xmath (and what gfx itself needs: ecs, god, the asset module); the loader stays in particles",
		},
		{
			name:    "rooms are opt-in",
			pkgs:    []string{"game"},
			allowed: func(dep string) bool { return dep != root+"room" },
			reason:  "game must not import room: games install rooms with room.Install",
		},
		{
			name: "collision is headless",
			pkgs: []string{"collision"},
			allowed: func(dep string) bool {
				return !strings.HasPrefix(dep, root) || strings.HasPrefix(dep, root+"god") ||
					strings.HasPrefix(dep, root+"gfx") ||
					dep == root+"xmath"
			},
			reason: "collision may import only ecs, ecs/system, god, gfx and xmath (and their stdlib-only dependencies)",
		},
		{
			name: "anim does not know collision",
			pkgs: []string{"anim", "extract"},
			allowed: func(dep string) bool {
				return dep != root+"collision" && dep != root+"anim/animbody"
			},
			reason: "anim and extract must not depend on collision: the anim-body sync lives in anim/animbody",
		},
		{
			name: "physics is headless",
			pkgs: []string{"physics"},
			allowed: func(dep string) bool {
				return !strings.HasPrefix(dep, root) || strings.HasPrefix(dep, root+"god") ||
					strings.HasPrefix(dep, root+"gfx") ||
					dep == root+"collision" || dep == root+"tween" || dep == root+"xmath"
			},
			reason: "physics may import only ecs, ecs/system, god, gfx, xmath, collision and tween (and their stdlib-only dependencies)",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var errs []error
			for _, pkg := range tc.pkgs {
				for _, dep := range listDeps(t, root+pkg) {
					if !tc.allowed(dep) {
						errs = append(errs, fmt.Errorf("%s imports %s", pkg, dep))
					}
				}
			}
			if err := errors.Join(errs...); err != nil {
				t.Errorf("%s:\n%v", tc.reason, err)
			}
		})
	}
}

// TestGameDoesNotWireCollision checks that game does not import
// collision itself: games add collision.NewSystem (and animbody.NewSystem
// for anim-body colliders). It still depends on it through tilemap.
func TestGameDoesNotWireCollision(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go tool not found")
	}
	out, err := exec.Command("go", "list", "-f", "{{join .Imports \"\\n\"}}", root+"game").Output()
	if err != nil {
		t.Fatalf("go list game: %v", err)
	}
	for _, imp := range strings.Fields(string(out)) {
		if imp == root+"collision" {
			t.Errorf("game imports collision: collision is opt-in")
		}
	}
}

// TestNoDirectEbitenImports checks the packages that sit on top of render and
// so depend on Ebiten transitively, but make no Ebiten calls of their own.
func TestNoDirectEbitenImports(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go tool not found")
	}
	for _, pkg := range []string{"particles", "speech"} {
		out, err := exec.Command("go", "list", "-f", "{{join .Imports \"\\n\"}}", root+pkg).Output()
		if err != nil {
			t.Fatalf("go list %s: %v", pkg, err)
		}
		for _, imp := range strings.Fields(string(out)) {
			if strings.Contains(imp, "hajimehoshi/ebiten") || strings.Contains(imp, "ebitengine") {
				t.Errorf("%s imports %s: it must not call Ebiten itself", pkg, imp)
			}
		}
	}
}

// TestVoiceLibraryOnlyInSpeech checks that github.com/weakpixel/voice is
// imported by speech alone, and that speech imports nothing
// but the standard library, the voice core and the engine packages it builds
// on.
func TestVoiceLibraryOnlyInSpeech(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go tool not found")
	}
	const voiceMod = "github.com/weakpixel/voice"
	out, err := exec.Command("go", "list", "-f", `{{.ImportPath}}{{range .Imports}} {{.}}{{end}}`, root+"...").Output()
	if err != nil {
		t.Fatalf("go list: %v", err)
	}
	speechAllowed := func(imp string) bool {
		if !strings.Contains(imp, ".") {
			return true // standard library
		}
		for _, p := range []string{"god", "asset/library", "gfx", "sound", "game", "assetkind", "xmath"} {
			if imp == root+p || strings.HasPrefix(imp, root+p+"/") {
				return true
			}
		}
		return isECS(imp) || isAsset(imp) || imp == voiceMod || strings.HasPrefix(imp, voiceMod+"/")
	}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		fields := strings.Fields(line)
		pkg, imps := fields[0], fields[1:]
		speech := pkg == root+"speech" || strings.HasPrefix(pkg, root+"speech/")
		for _, imp := range imps {
			if speech && !speechAllowed(imp) {
				t.Errorf("%s imports %s: speech may import the standard library, the voice core, ecs, god, the asset module, asset/library, gfx, sound, game, assetkind and xmath only", pkg, imp)
			}
			if !speech && (imp == voiceMod || strings.HasPrefix(imp, voiceMod+"/")) {
				t.Errorf("%s imports %s: only speech may import the voice library", pkg, imp)
			}
		}
	}
}

// TestToolShellDependencies checks that debugui and Cobra reach only the
// native weakpixel-editor: the tool shell, the tools and the whole js/wasm
// build of the command stay free of them. A game module that builds the wasm
// editor for the VSCode extension then needs no go.sum entries for either.
func TestToolShellDependencies(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go tool not found")
	}
	const debugui = "github.com/ebitengine/debugui"
	var errs []error
	check := func(what string, deps []string, bad func(dep string) bool) {
		for _, dep := range deps {
			if bad(dep) {
				errs = append(errs, fmt.Errorf("%s imports %s", what, dep))
			}
		}
	}
	isDebugUI := func(dep string) bool {
		return strings.HasPrefix(dep, debugui) || strings.Contains(dep, "hajimehoshi/bitmapfont")
	}
	isCobra := func(dep string) bool {
		return strings.HasPrefix(dep, cobra) || strings.HasPrefix(dep, "github.com/spf13/pflag")
	}
	for _, pkg := range []string{"editor/toolshell", "editor/viewer", "editor/animedit", "editor/fxedit", "editor/sheetedit"} {
		check(pkg, listDeps(t, root+pkg), isDebugUI)
	}
	// A tool links neither the level editor nor its bridge: what they share
	// is in editor/hostlink.
	isLevelEditor := func(dep string) bool {
		return dep == root+"editor" || dep == root+"editor/bridge" || dep == root+"editor/host"
	}
	check("editor/toolshell", listDeps(t, root+"editor/toolshell"), isLevelEditor)
	cmd := root + "tools/weakpixel-editor"
	wasm := listDepsEnv(t, cmd, "GOOS=js", "GOARCH=wasm")
	check("js/wasm "+cmd, wasm, isDebugUI)
	check("js/wasm "+cmd, wasm, isCobra)
	check("js/wasm "+cmd, wasm, isLevelEditor)
	check(cmd, listDeps(t, cmd), isLevelEditor)
	if err := errors.Join(errs...); err != nil {
		t.Errorf("debugui may only be linked by editor/toolshell/debugpanel and the native editor, Cobra not by the wasm editor, and the tool shell and weakpixel-editor must not link the level editor (editor, its bridge or its host):\n%v", err)
	}
}

const cobra = "github.com/spf13/cobra"

// TestToolPackageImports checks the direct imports of the packages of the
// root module: Cobra only under tools/ (and not in the tool packages of
// tools/internal, which neither import it nor flag), debugui only in
// debugpanel and the native editor.
func TestToolPackageImports(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go tool not found")
	}
	out, err := exec.Command("go", "list", "-f", "{{.ImportPath}} {{join .Imports \" \"}}", "./...").Output()
	if err != nil {
		t.Fatalf("go list ./...: %v", err)
	}
	const debugui = "github.com/ebitengine/debugui"
	var errs []error
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		pkg, imports, _ := strings.Cut(line, " ")
		rel := strings.TrimPrefix(pkg, root)
		for _, imp := range strings.Fields(imports) {
			if strings.HasPrefix(imp, cobra) && !strings.HasPrefix(rel, "tools/") {
				errs = append(errs, fmt.Errorf("%s imports %s: Cobra may be imported only under tools/", rel, imp))
			}
			// The tool packages take options structs: no command line parsing.
			if strings.HasPrefix(rel, "tools/internal/") && rel != "tools/internal/cli" &&
				(strings.HasPrefix(imp, cobra) || imp == "flag") {
				errs = append(errs, fmt.Errorf("%s imports %s: tool packages take options and leave the command line to tools/<binary>/cmd", rel, imp))
			}
			if strings.HasPrefix(imp, debugui) &&
				rel != "editor/toolshell/debugpanel" && rel != "tools/weakpixel-editor/internal/native" {
				errs = append(errs, fmt.Errorf("%s imports %s: debugui may be imported only by debugpanel and the native editor", rel, imp))
			}
		}
	}
	if err := errors.Join(errs...); err != nil {
		t.Error(err)
	}
}
