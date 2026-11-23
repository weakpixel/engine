// Package bridge connects an editor.Session to a host over JSON messages:
// the VSCode extension (through the webview, see transport_js.go) or, in
// tests, a fake. The protocol is transport-agnostic; doc/design/editor.md
// lists every message.
//
// Game → host: ready, applied, objects, selection, edit, mode, create,
// choose, camera, mapLayers, screenshot, entities, status, error. Host →
// game: document, open, assetsChanged, select, drop, create, assetField,
// play, stop, step, pause, frame, camera, settings, visible,
// mapLayerHidden, screenshot, inspect, status, input.
package bridge

import (
	"encoding/json"
	"path"
	"strings"

	"github.com/weakpixel/engine/editor/hostlink"
	"github.com/weakpixel/engine/god"
)

// Version is the protocol version, sent in "ready". A host refuses a game
// with a version it does not know. It is [hostlink.Version], which the tool
// shell sends too.
const Version = hostlink.Version

// AssetKinds maps file suffixes to the asset kind (x-asset) a dropped file
// of that type fills; longer suffixes are checked first. It is sent in
// "ready" so the host's inspector uses the same mapping.
var AssetKinds = map[string]string{
	".anim.json":     "clip",
	".fx.json":       "effect",
	".prefab.json":   "prefab",
	".voice.json":    "voice",
	".dialogue.json": "dialogue",
	".png":           "sprite",
	".jpg":           "sprite",
	".jpeg":          "sprite",
	".kage":          "shader",
	".wav":           "sound",
	".ogg":           "sound",
	".mp3":           "sound",
}

// kindOf returns the asset kind of a file path, or "".
func kindOf(p string) string {
	name := strings.ToLower(path.Base(p))
	best, kind := 0, ""
	for suffix, k := range AssetKinds {
		if strings.HasSuffix(name, suffix) && len(suffix) > best {
			best, kind = len(suffix), k
		}
	}
	return kind
}

// inbound is any message from the host; Type selects which fields are set.
type inbound struct {
	Type string `json:"type"`
	// document, open
	Text string `json:"text"`
	// assetsChanged
	Paths []string `json:"paths"`
	// select: IDs, and Reveal to pan the camera to them when they are off
	// screen.
	IDs    []string `json:"ids"`
	Reveal bool     `json:"reveal"`
	// mapLayerHidden: Map (the map's asset key), Path (the layer's Tiled
	// path) and Hidden.
	Map    string `json:"map"`
	Hidden bool   `json:"hidden"`
	// open: Path is the file's asset key (relative to the asset root).
	// drop: the file (relative to the asset root), where it landed in
	// window (device) pixels on the canvas, and the chosen "group:class"
	// after a "choose".
	Path  string  `json:"path"`
	X     float64 `json:"x"`
	Y     float64 `json:"y"`
	Class string  `json:"class"`
	// create: Class, or Prefab (a prefab key) for an instance of it.
	// assetField: the object, the field (component key
	// first, then property names) and Path, the dropped file.
	Prefab string   `json:"prefab"`
	ID     string   `json:"id"`
	Field  []string `json:"field"`
	// camera: the editor camera's world position (X, Y, shared with drop's
	// window position) and Zoom.
	Zoom float32 `json:"zoom"`
	// settings: the drag snap grid in world units, 0 for off, the collider
	// outlines of edit mode, the level's background in edit mode and the
	// prefab stage's look. Each part is
	// applied only when present.
	Snap      *float32      `json:"snap"`
	Colliders *bool         `json:"colliders"`
	Stage     *stageSetting `json:"stage"`
	// Background is the level stage's background in edit mode ("#rrggbb",
	// "" for the game's own).
	Background *string `json:"background"`
	// Rate is the tick rates of pacing (editor.Rates); missing parts keep
	// their value.
	Rate *rateSetting `json:"rate"`
	// visible: whether the host shows the view.
	Visible *bool `json:"visible"`
	// pause: Paused.
	Paused bool `json:"paused"`
	// inspect: IDs (none: every object of the level), Components (keys;
	// none: all), All for the entities without a document id too, and
	// Limit (0: 100).
	Components []string `json:"components"`
	All        bool     `json:"all"`
	Limit      int      `json:"limit"`
	// input: Keys held (input.Key names), Mouse, Pad and Ticks (0: 1).
	Keys  []string    `json:"keys"`
	Mouse *inputMouse `json:"mouse"`
	Pad   *inputPad   `json:"pad"`
	Ticks int         `json:"ticks"`
}

// rateSetting is the "rate" part of a settings message, in ticks per
// second; nil keeps a rate.
type rateSetting struct {
	Idle   *int `json:"idle"`
	Active *int `json:"active"`
	Hidden *int `json:"hidden"`
}

// stageSetting is the "stage" part of a settings message: the prefab
// stage's background ("#rrggbb", "" keeps it) and grid step in world units
// (nil keeps it, 0 for no grid).
type stageSetting struct {
	Background string   `json:"background"`
	Grid       *float32 `json:"grid"`
}

// Outbound messages. Each has its own type so empty lists are sent as [].

type readyMsg struct {
	Type    string       `json:"type"` // "ready"
	Version int          `json:"version"`
	Schemas []god.Schema `json:"schemas"`
	// Components is every registered component type: an object may hold
	// any of them, not only its class's.
	Components []god.ComponentSchema `json:"components"`
	AssetKinds map[string]string     `json:"assetKinds"`
}

// appliedMsg answers every "document" and "open": Error is "" when it was applied. A
// document that waits for prefabs is answered once they loaded, and a
// prefab change that re-applied the document sends one unasked.
type appliedMsg struct {
	Type  string `json:"type"` // "applied"
	Error string `json:"error"`
}

// ObjectInfo is one object of the level, for the host's outline. Prefab is
// the prefab key of an instance.
type ObjectInfo struct {
	ID     string `json:"id"`
	Group  string `json:"group"`
	Class  string `json:"class"`
	Prefab string `json:"prefab,omitempty"`
	// Part is the id within the prefab of an object of a multi-object
	// instance that is not its root ("sword"); its ID is "<root id>/<part>".
	Part string `json:"part,omitempty"`
}

type objectsMsg struct {
	Type    string       `json:"type"` // "objects"
	Objects []ObjectInfo `json:"objects"`
}

type selectionMsg struct {
	Type string   `json:"type"` // "selection"
	IDs  []string `json:"ids"`
}

// editMsg is editor.Edit: with Override, JSON is the component's override
// of a prefab instance ({} for none).
type editMsg struct {
	Type      string          `json:"type"` // "edit"
	ID        string          `json:"id"`
	Part      string          `json:"part,omitempty"` // with ID of the instance root
	Component string          `json:"component"`
	JSON      json.RawMessage `json:"json"`
	Override  bool            `json:"override,omitempty"`
}

// modeMsg reports Play and edit mode; Paused is set while Play is paused
// ("pause").
type modeMsg struct {
	Type   string `json:"type"` // "mode"
	Play   bool   `json:"play"`
	Paused bool   `json:"paused,omitempty"`
}

// createMsg asks the host to append Object to the document (it already has
// a fresh id); the document change then creates it in the game.
type createMsg struct {
	Type   string          `json:"type"` // "create"
	Object json.RawMessage `json:"object"`
}

// chooseMsg asks the host which object type a drop creates; the host sends
// the drop again with Class set to one of Options.
type chooseMsg struct {
	Type    string   `json:"type"` // "choose"
	Path    string   `json:"path"`
	X       float64  `json:"x"`
	Y       float64  `json:"y"`
	Options []string `json:"options"`
}

// fieldMsg answers "assetField": the host writes Value at Field of object ID.
type fieldMsg struct {
	Type  string   `json:"type"` // "field"
	ID    string   `json:"id"`
	Field []string `json:"field"`
	Value string   `json:"value"`
}

// cameraMsg reports the editor camera once it stopped moving; the host
// sends the same message back after a rebuild to restore it.
type cameraMsg struct {
	Type string  `json:"type"` // "camera"
	X    float32 `json:"x"`
	Y    float32 `json:"y"`
	Zoom float32 `json:"zoom"`
}

// mapLayersMsg lists the tile and image layers of the Tiled map behind the
// level, in draw order. It is sent when the list changes (not every tick).
type mapLayersMsg struct {
	Type   string         `json:"type"` // "mapLayers"
	Layers []mapLayerInfo `json:"layers"`
}

// mapLayerInfo is editor.MapLayer: Map the .tmj key, Path the Tiled layer
// path, Kind "tiles" or "image", Layer the render layer.
type mapLayerInfo struct {
	Map    string `json:"map"`
	Name   string `json:"name"`
	Path   string `json:"path"`
	Kind   string `json:"kind"`
	Layer  int16  `json:"layer"`
	Hidden bool   `json:"hidden"`
}

type errorMsg struct {
	Type    string `json:"type"` // "error"
	Message string `json:"message"`
}
