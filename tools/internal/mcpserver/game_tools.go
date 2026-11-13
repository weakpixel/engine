package mcpserver

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/weakpixel/engine/tools/internal/mcp"
)

// control are the annotations of a tool that acts on the running game
// without changing a file.
var control = mcp.Annotations{}

type startArgs struct {
	Path     string `json:"path"`
	Headless bool   `json:"headless"`
}

// sessionInfo describes a running game.
func sessionInfo(s *gameSession) map[string]any {
	st := s.cl.state()
	out := map[string]any{
		"running": true, "pid": s.pid, "path": st.Path, "stage": s.stage, "headless": s.headless,
		"playing": st.Play, "paused": st.Paused, "started": s.started.Format(time.RFC3339), "build_ms": s.buildDur.Milliseconds(),
		"objects": len(st.Objects),
	}
	if st.ApplyErr != "" {
		out["last_applied_error"] = st.ApplyErr
	}
	if st.LastErr != "" {
		out["last_bridge_error"] = st.LastErr
	}
	return out
}

func (t *tools) gameStart(ctx context.Context, a startArgs) (any, error) {
	if a.Path == "" {
		return nil, errors.New(`"path" is required: the level or prefab to open (list_documents lists them)`)
	}
	s, err := t.game.start(ctx, startOptions{Path: a.Path, Headless: a.Headless})
	if err != nil {
		return nil, err
	}
	out := sessionInfo(s)
	if w := t.game.buildIgnored(); w != "" {
		out["warnings"] = []string{w}
	}
	if !a.Headless {
		out["note"] = "the game has a window; screenshot works, and edits in the window are written to the document"
	} else {
		out["note"] = "headless: nothing is drawn, so screenshot is refused; step and play work"
	}
	return out, nil
}

func (t *tools) gameStop(_ context.Context, _ struct{}) (any, error) {
	exit, ok := t.game.stop()
	if !ok {
		return map[string]any{"stopped": false, "note": "no game was running"}, nil
	}
	return map[string]any{"stopped": true, "pid": exit.PID, "status": exit.Status}, nil
}

func (t *tools) gameStatus(ctx context.Context, _ struct{}) (any, error) {
	if s := t.game.current(); s != nil {
		out := sessionInfo(s)
		if st, err := s.cl.queryStatus(ctx); err == nil {
			out["tick"], out["scenes"], out["playing"], out["paused"] = st.Tick, st.Scenes, st.Play, st.Paused
		} else {
			out["status_error"] = err.Error()
		}
		return out, nil
	}
	out := map[string]any{"running": false}
	t.game.mu.Lock()
	last := t.game.last
	t.game.mu.Unlock()
	if last != nil {
		out["last_exit"] = last
	}
	if t.p.Game == "" {
		out["note"] = "the project has no editor package (editor.game): game_start cannot build a game"
	}
	return out, nil
}

func (t *tools) gameRebuild(ctx context.Context, _ struct{}) (any, error) {
	s, err := t.game.rebuild(ctx)
	if err != nil {
		return nil, err
	}
	return sessionInfo(s), nil
}

type logsArgs struct {
	Since uint64 `json:"since"`
	Limit int    `json:"limit"`
}

func (t *tools) readLogs(_ context.Context, a logsArgs) (any, error) {
	limit := a.Limit
	if limit <= 0 {
		limit = 200
	}
	lines, next, dropped := t.game.logs.read(a.Since, limit)
	if lines == nil {
		lines = []logLine{}
	}
	out := map[string]any{"lines": lines, "next": next}
	if dropped {
		out["dropped"] = true
		out["note"] = "lines after `since` were pushed out of the log before they were read"
	}
	return out, nil
}

func (t *tools) listLiveObjects(_ context.Context, _ struct{}) (any, error) {
	s, err := t.game.need()
	if err != nil {
		return nil, err
	}
	st := s.cl.state()
	objs := st.Objects
	if objs == nil {
		objs = []objectInfo{}
	}
	sel := st.Selection
	if sel == nil {
		sel = []string{}
	}
	return map[string]any{"path": st.Path, "objects": objs, "selection": sel}, nil
}

type selectArgs struct {
	IDs    []string `json:"ids"`
	Reveal bool     `json:"reveal"`
}

func (t *tools) selectObjects(ctx context.Context, a selectArgs) (any, error) {
	s, err := t.game.need()
	if err != nil {
		return nil, err
	}
	sel, err := s.cl.selectIDs(ctx, a.IDs, a.Reveal)
	if err != nil {
		return nil, err
	}
	out := map[string]any{"selection": sel}
	if len(sel) < len(a.IDs) {
		out["note"] = "ids that name no object of the open level were ignored"
	}
	return out, nil
}

func (t *tools) playGame(ctx context.Context, _ struct{}) (any, error) {
	s, err := t.game.need()
	if err != nil {
		return nil, err
	}
	on, err := s.cl.setPlay(ctx, true)
	if err != nil {
		return nil, err
	}
	if !on {
		if s.stage == "prefab" {
			return nil, errors.New("a prefab cannot be played; open a level (game_stop, then game_start with a *.level.json)")
		}
		return nil, errors.New("the game did not enter play mode (read_logs may say why)")
	}
	return map[string]any{"playing": true}, nil
}

func (t *tools) stopPlay(ctx context.Context, _ struct{}) (any, error) {
	s, err := t.game.need()
	if err != nil {
		return nil, err
	}
	was := s.cl.state().Play
	if _, err := s.cl.setPlay(ctx, false); err != nil {
		return nil, err
	}
	note := "the level is back to its document"
	if !was {
		note = "the game was not playing"
	}
	return map[string]any{"playing": false, "note": note}, nil
}

type stepArgs struct {
	Count int `json:"count"`
}

func (t *tools) stepGame(ctx context.Context, a stepArgs) (any, error) {
	s, err := t.game.need()
	if err != nil {
		return nil, err
	}
	n := a.Count
	if n == 0 {
		n = 1
	}
	if err := s.cl.step(ctx, n); err != nil {
		return nil, err
	}
	return map[string]any{"ticks": n}, nil
}

func (t *tools) frameView(ctx context.Context, _ struct{}) (any, error) {
	s, err := t.game.need()
	if err != nil {
		return nil, err
	}
	cam, err := s.cl.frame(ctx)
	if err != nil {
		return nil, err
	}
	return map[string]any{"camera": cam}, nil
}

func (t *tools) getCamera(_ context.Context, _ struct{}) (any, error) {
	s, err := t.game.need()
	if err != nil {
		return nil, err
	}
	return map[string]any{"camera": s.cl.state().Camera, "playing": s.cl.state().Play}, nil
}

type cameraArgs struct {
	X    *float64 `json:"x"`
	Y    *float64 `json:"y"`
	Zoom float64  `json:"zoom"`
}

func (t *tools) setCamera(ctx context.Context, a cameraArgs) (any, error) {
	s, err := t.game.need()
	if err != nil {
		return nil, err
	}
	cur := s.cl.state().Camera
	x, y, zoom := cur.X, cur.Y, cur.Zoom
	if a.X != nil {
		x = *a.X
	}
	if a.Y != nil {
		y = *a.Y
	}
	if a.Zoom > 0 {
		zoom = a.Zoom
	}
	cam, err := s.cl.setCamera(ctx, x, y, zoom)
	if err != nil {
		return nil, err
	}
	return map[string]any{"camera": cam}, nil
}

type openArgs struct {
	Path string `json:"path"`
}

func (t *tools) openDocument(ctx context.Context, a openArgs) (any, error) {
	s, err := t.game.need()
	if err != nil {
		return nil, err
	}
	abs, key, err := t.p.AssetPath(a.Path)
	if err != nil {
		return nil, err
	}
	if _, err := documentKind(key); err != nil {
		return nil, err
	}
	if stageOf(key) != s.stage {
		return nil, fmt.Errorf("the running game is in the %s stage and %s is a %s: game_stop, then game_start with this path", s.stage, key, stageOf(key))
	}
	t.docMu.Lock()
	defer t.docMu.Unlock()
	text, err := os.ReadFile(abs)
	if err != nil {
		return nil, err
	}
	if err := s.cl.apply(ctx, "open", key, text); err != nil {
		return nil, fmt.Errorf("the game did not open %s: %w", key, err)
	}
	return sessionInfo(s), nil
}

type reloadArgs struct {
	Paths []string `json:"paths"`
}

func (t *tools) reloadAssets(ctx context.Context, a reloadArgs) (any, error) {
	s, err := t.game.need()
	if err != nil {
		return nil, err
	}
	keys := make([]string, 0, len(a.Paths))
	for _, p := range a.Paths {
		_, key, err := t.p.AssetPath(p)
		if err != nil {
			return nil, err
		}
		keys = append(keys, key)
	}
	if len(keys) == 0 {
		return nil, errors.New(`give "paths", the asset keys to reload; changed files are picked up on their own every half second`)
	}
	if _, err := s.cl.sendSync(ctx, map[string]any{"type": "assetsChanged", "paths": keys}); err != nil {
		return nil, err
	}
	return map[string]any{"reloaded": keys}, nil
}

func (t *tools) listMapLayers(_ context.Context, _ struct{}) (any, error) {
	s, err := t.game.need()
	if err != nil {
		return nil, err
	}
	layers := s.cl.state().Layers
	if layers == nil {
		layers = []mapLayer{}
	}
	return map[string]any{"layers": layers}, nil
}

type layerArgs struct {
	Map    string `json:"map"`
	Path   string `json:"path"`
	Hidden bool   `json:"hidden"`
}

func (t *tools) setMapLayerHidden(ctx context.Context, a layerArgs) (any, error) {
	s, err := t.game.need()
	if err != nil {
		return nil, err
	}
	errs, err := s.cl.sendSync(ctx, map[string]any{"type": "mapLayerHidden", "map": a.Map, "path": a.Path, "hidden": a.Hidden})
	if err != nil {
		return nil, err
	}
	if len(errs) > 0 {
		return nil, fmt.Errorf("%s (list_map_layers lists the layers)", strings.TrimPrefix(errs[0], "editor: "))
	}
	return map[string]any{"map": a.Map, "path": a.Path, "hidden": a.Hidden}, nil
}

type settingsArgs struct {
	Snap      *float64 `json:"snap"`
	Colliders *bool    `json:"colliders"`
}

func (t *tools) editorSettings(ctx context.Context, a settingsArgs) (any, error) {
	s, err := t.game.need()
	if err != nil {
		return nil, err
	}
	msg := map[string]any{"type": "settings"}
	if a.Snap != nil {
		msg["snap"] = *a.Snap
	}
	if a.Colliders != nil {
		msg["colliders"] = *a.Colliders
	}
	if len(msg) > 1 {
		if _, err := s.cl.sendSync(ctx, msg); err != nil {
			return nil, err
		}
		s.cl.mu.Lock()
		if a.Snap != nil {
			s.cl.snap = *a.Snap
		}
		if a.Colliders != nil {
			v := *a.Colliders
			s.cl.colliders = &v
		}
		s.cl.mu.Unlock()
	}
	st := s.cl.state()
	out := map[string]any{"snap": st.Snap}
	if st.Colliders != nil {
		out["colliders"] = *st.Colliders
	}
	return out, nil
}

func (t *tools) gameTools() []*mcp.Tool {
	none := mcp.Object(nil)
	return []*mcp.Tool{
		tool("game_start", "Start the game",
			"Builds the project's editor package (go build -tags editor, output in build/weakpixel-mcp/) and starts it on the level or prefab at "+
				"path, as a child process. By default the game has a window, so the developer watches; edits they make there are written to "+
				"the document, and edits you make to it show in the window. headless: true runs without a window (CI, no display): nothing "+
				"is drawn and screenshot is refused. Only one game runs at a time.",
			mcp.Object(map[string]*mcp.Schema{
				"path":     mcp.String("asset key of the level (*.level.json) or prefab (*.prefab.json) to open"),
				"headless": mcp.Boolean("run without a window (default false)"),
			}, "path"),
			control, t.gameStart),
		tool("game_stop", "Stop the game",
			"Ends the running game: it is asked to quit, and killed after 3 seconds. Files are not changed.",
			none, control, t.gameStop),
		tool("game_status", "Show the game's status",
			"Says whether a game runs, its process id, the open document, play mode and pause, the game's tick count and scene stack "+
				"(names, bottom first), the last error the game reported for a document, and how the previous game ended. Changes nothing.",
			none, readOnly, t.gameStatus),
		tool("game_rebuild", "Rebuild and restart the game",
			"Builds the game again after a code change. Only if the build succeeds, the running game is replaced by the new one, with "+
				"the same document open and the same camera. On a build error the output is returned and the old game keeps running.",
			none, control, t.gameRebuild),
		tool("read_logs", "Read the game's output",
			"Returns lines the game printed (stdout, stderr) and errors it reported over the editor protocol (stream bridge), "+
				"with the server's own notes (stream server). Pass the returned next as since to read only newer lines. The last 5000 "+
				"lines are kept, also after the game ended. Changes nothing.",
			mcp.Object(map[string]*mcp.Schema{
				"since": mcp.Integer("return lines with a larger sequence number (default 0: from the oldest)", 0, 1e15),
				"limit": mcp.Integer("most lines to return (default 200)", 1, 5000),
			}),
			readOnly, t.readLogs),
		tool("list_live_objects", "List the objects of the open level in the game",
			"Lists the objects the running game has from the open document (id, group, class, prefab) and the selection. Changes nothing.",
			none, readOnly, t.listLiveObjects),
		tool("select_objects", "Select objects in the game",
			"Selects objects by id in the game (ids that name no object are ignored). reveal pans the camera to them when they are off screen. Edit mode only.",
			mcp.Object(map[string]*mcp.Schema{
				"ids":    {Type: "array", Items: mcp.String("object id"), Description: "ids to select; an empty list clears the selection"},
				"reveal": mcp.Boolean("pan the camera to the selection if it is off screen"),
			}, "ids"),
			control, t.selectObjects),
		tool("play", "Play the level",
			"Runs the game from the level's current state, like F5 in the editor. stop returns to the document's state. Levels only.",
			none, control, t.playGame),
		tool("stop", "Stop playing",
			"Ends play mode: objects created while playing are removed and the level is restored from its document. Does nothing in edit mode.",
			none, control, t.stopPlay),
		tool("step", "Step the paused game",
			"Runs count ticks (1 to 600) of the paused game, in edit mode or in a play mode held by pause, and returns when the last one "+
				"ran. Not while play mode runs by itself.",
			mcp.Object(map[string]*mcp.Schema{"count": mcp.Integer("ticks to run (default 1)", 1, 600)}),
			control, t.stepGame),
		tool("frame_view", "Frame the level",
			"Centers the editor camera on the level, like the Frame command, and returns the camera.",
			none, control, t.frameView),
		tool("get_camera", "Get the camera",
			"Returns the editor camera (world position and zoom) as the game last reported it. Changes nothing.",
			none, readOnly, t.getCamera),
		tool("set_camera", "Set the camera",
			"Moves the editor camera to a world position and zoom. Omitted values stay as they are.",
			mcp.Object(map[string]*mcp.Schema{
				"x": mcp.Number("world x of the view center"), "y": mcp.Number("world y of the view center"),
				"zoom": mcp.Number("zoom, 1 is one screen pixel per world unit"),
			}),
			control, t.setCamera),
		tool("open_document", "Open another document in the game",
			"Switches the running game to another level (or prefab) from the same stage, reading the file from disk. To change between a "+
				"level and a prefab, stop the game and start it again.",
			mcp.Object(map[string]*mcp.Schema{"path": mcp.String("asset key of the document")}, "path"),
			control, t.openDocument),
		tool("reload_assets", "Reload assets",
			"Tells the game that asset files changed so it reloads them (textures, shaders, sounds, sheets, effects). The server also "+
				"does this on its own within half a second of a file change; use this to force it.",
			mcp.Object(map[string]*mcp.Schema{"paths": {Type: "array", Items: mcp.String("asset key"), Description: "asset keys to reload"}}, "paths"),
			control, t.reloadAssets),
		tool("list_map_layers", "List the Tiled map layers",
			"Lists the tile and image layers of the Tiled map behind the open level, in draw order, with their hidden state.",
			none, readOnly, t.listMapLayers),
		tool("set_map_layer_hidden", "Hide or show a map layer",
			"Hides or shows a layer of the Tiled map in the editor view (not in the file). Edit mode only. map and path come from list_map_layers.",
			mcp.Object(map[string]*mcp.Schema{
				"map": mcp.String("asset key of the map (.tmj)"), "path": mcp.String("the layer's path"),
				"hidden": mcp.Boolean("true hides the layer"),
			}, "map", "path", "hidden"),
			control, t.setMapLayerHidden),
		tool("editor_settings", "Change the editor view settings",
			"Sets the drag snap grid in world units (0 for off) and whether collider outlines show in edit mode. Omitted values stay; "+
				"the result shows what this server last set (the game does not report them).",
			mcp.Object(map[string]*mcp.Schema{
				"snap": mcp.Number("snap grid in world units, 0 for off"), "colliders": mcp.Boolean("show collider outlines"),
			}),
			control, t.editorSettings),
	}
}
