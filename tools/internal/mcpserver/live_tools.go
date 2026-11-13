package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/weakpixel/engine/tools/internal/mcp"
)

type screenshotArgs struct {
	Target    string `json:"target"`
	Scale     int    `json:"scale"`
	KeepAlpha bool   `json:"keep_alpha"`
	Last      bool   `json:"last"`
}

// screenshot asks the game for a picture over the bridge's screenshot
// message and returns it as an image (plan 3.7).
func (t *tools) screenshot(ctx context.Context, raw json.RawMessage) (*mcp.Result, error) {
	var a screenshotArgs
	if err := json.Unmarshal(raw, &a); err != nil {
		return nil, err
	}
	s, err := t.game.need()
	if err != nil {
		return nil, err
	}
	var ans shotAnswer
	switch {
	case a.Last:
		last := s.cl.latestShot()
		if last == nil {
			return nil, errors.New("the game has not taken a capture of its own yet (the screenshot key, F12 by default, takes one in the window); call screenshot without last")
		}
		ans = *last
	case s.headless:
		return nil, errors.New("this game runs without a window and draws nothing, so it cannot take a screenshot: game_stop, then game_start without headless")
	default:
		if ans, err = s.cl.screenshot(ctx, a.Target, a.Scale, a.KeepAlpha); err != nil {
			return nil, err
		}
	}
	info, _ := json.Marshal(map[string]any{"width": ans.Width, "height": ans.Height, "tick": ans.Tick, "id": ans.ID})
	res := mcp.TextResult(string(info))
	img := mcp.ImageResult(ans.PNG, "image/png")
	res.Content = append(res.Content, img.Content...)
	res.StructuredContent = map[string]any{"width": ans.Width, "height": ans.Height, "tick": ans.Tick}
	return res, nil
}

type inspectArgs struct {
	IDs        []string `json:"ids"`
	Components []string `json:"components"`
	All        bool     `json:"all"`
	Limit      int      `json:"limit"`
}

// inspectEntities reads live component values from the game (plan 4.1).
func (t *tools) inspectEntities(ctx context.Context, a inspectArgs) (any, error) {
	s, err := t.game.need()
	if err != nil {
		return nil, err
	}
	req := map[string]any{"ids": a.IDs, "components": a.Components, "all": a.All, "limit": a.Limit}
	raw, err := s.cl.inspect(ctx, req)
	if err != nil {
		return nil, err
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	delete(out, "type")
	st := s.cl.state()
	out["playing"], out["paused"] = st.Play, st.Paused
	return out, nil
}

type pauseArgs struct {
	Paused *bool `json:"paused"`
}

// pauseGame pauses or resumes Play (plan 4.3).
func (t *tools) pauseGame(ctx context.Context, a pauseArgs) (any, error) {
	s, err := t.game.need()
	if err != nil {
		return nil, err
	}
	on := a.Paused == nil || *a.Paused
	play, paused, err := s.cl.pause(ctx, on)
	if err != nil {
		return nil, err
	}
	if !play {
		return nil, errors.New("the game is not playing: pause holds a running play mode still (edit mode is always paused); call play first")
	}
	return map[string]any{"playing": play, "paused": paused}, nil
}

type inputMouseArgs struct {
	X       float64  `json:"x"`
	Y       float64  `json:"y"`
	Buttons []string `json:"buttons"`
	WheelX  float64  `json:"wheel_x"`
	WheelY  float64  `json:"wheel_y"`
}

type inputPadArgs struct {
	Buttons  []string  `json:"buttons"`
	Axes     []float64 `json:"axes"`
	Triggers []float64 `json:"triggers"`
}

type inputArgs struct {
	Keys  []string        `json:"keys"`
	Mouse *inputMouseArgs `json:"mouse"`
	Pad   *inputPadArgs   `json:"pad"`
	Ticks int             `json:"ticks"`
}

// sendInput holds input in the playing game for some ticks (plan 4.4).
func (t *tools) sendInput(ctx context.Context, a inputArgs) (any, error) {
	s, err := t.game.need()
	if err != nil {
		return nil, err
	}
	if len(a.Keys) == 0 && a.Mouse == nil && a.Pad == nil {
		return nil, errors.New(`give "keys", "mouse" or "pad"`)
	}
	ticks := max(a.Ticks, 1)
	req := map[string]any{"keys": a.Keys, "ticks": ticks}
	if m := a.Mouse; m != nil {
		req["mouse"] = map[string]any{"x": m.X, "y": m.Y, "buttons": m.Buttons, "wheelX": m.WheelX, "wheelY": m.WheelY}
	}
	if p := a.Pad; p != nil {
		var axes [4]float64
		var triggers [2]float64
		if len(p.Axes) > 4 || len(p.Triggers) > 2 {
			return nil, fmt.Errorf("pad: axes has at most 4 values (left x, left y, right x, right y) and triggers at most 2 (left, right)")
		}
		copy(axes[:], p.Axes)
		copy(triggers[:], p.Triggers)
		req["pad"] = map[string]any{"buttons": p.Buttons, "axes": axes, "triggers": triggers}
	}
	if !s.cl.state().Play {
		return nil, errors.New("the game is not playing: input reaches the game in play mode only (edit mode reads the mouse for editing); call play first")
	}
	st, waited, err := s.cl.sendInput(ctx, req, ticks)
	if err != nil {
		return nil, err
	}
	out := map[string]any{"ticks": ticks, "tick": st.Tick, "playing": st.Play, "paused": st.Paused}
	if waited {
		out["note"] = "the input was held for the ticks given and is released now"
	} else {
		out["note"] = "the game is paused: the input is held for the next ticks that run (step runs one)"
	}
	return out, nil
}

func (t *tools) liveTools() []*mcp.Tool {
	names := func(desc string) *mcp.Schema {
		return &mcp.Schema{Type: "array", Items: mcp.String("name"), Description: desc}
	}
	numbers := func(desc string) *mcp.Schema {
		return &mcp.Schema{Type: "array", Items: mcp.Number("value"), Description: desc}
	}
	return []*mcp.Tool{
		tool("inspect_entities", "Read live component values",
			"Returns the live values of components in the running game, encoded as a level document holds them (asset keys, object ids; "+
				"a reference to an entity that is no object of the level is written #<index>.<generation>). By default every object of the "+
				"open level; ids limits it to those objects, components to those component keys, all adds the entities without a document "+
				"id (spawned by systems). Each entity has entity (index.generation), id and components; truncated says the limit cut the "+
				"list, missing lists ids that name no live object. Works in edit and play mode. Changes nothing.",
			mcp.Object(map[string]*mcp.Schema{
				"ids":        names("object ids to read (default: every object of the level)"),
				"components": names(`component keys to include, "group:Name" (default: all)`),
				"all":        mcp.Boolean("also list the entities that are no object of the level"),
				"limit":      mcp.Integer("most entities to return (default 100)", 1, 10000),
			}),
			readOnly, t.inspectEntities),
		tool("pause", "Pause or resume play",
			"Pauses the playing game (paused true, the default) or resumes it (paused false). A paused game keeps drawing, its systems do "+
				"not run, and step advances it one tick at a time; the level is not reset (stop does that). Play mode only.",
			mcp.Object(map[string]*mcp.Schema{"paused": mcp.Boolean("true pauses (default), false resumes")}),
			control, t.pauseGame),
		tool("send_input", "Send input to the game",
			"Holds keys, mouse buttons and position, and a virtual gamepad in the playing game for ticks ticks (default 1), on top of the "+
				"real devices, then releases them; the game's input actions see it as a player would press it. Returns after those ticks "+
				"ran; while paused the input waits for the ticks step runs. Key names as the engine writes them (Space, A, "+
				"ArrowLeft, Enter); mouse buttons left, middle, right, back, forward; pad buttons south, east, west, north, lb, rb, "+
				"lt, rt, select, start, ls, rs, up, down, left, right, home (the pad is assigned like a connected gamepad). Play mode only.",
			mcp.Object(map[string]*mcp.Schema{
				"keys": names("keys to hold"),
				"mouse": mcp.Object(map[string]*mcp.Schema{
					"x": mcp.Number("cursor x in logical screen pixels"), "y": mcp.Number("cursor y in logical screen pixels"),
					"buttons": names("mouse buttons to hold"),
					"wheel_x": mcp.Number("wheel movement per tick, x"), "wheel_y": mcp.Number("wheel movement per tick, y"),
				}),
				"pad": mcp.Object(map[string]*mcp.Schema{
					"buttons":  names("pad buttons to hold"),
					"axes":     numbers("left x, left y, right x, right y; -1 to 1, y down"),
					"triggers": numbers("left and right trigger, 0 to 1"),
				}),
				"ticks": mcp.Integer("game ticks to hold the input (default 1)", 1, 600),
			}),
			control, t.sendInput),
		{
			Name: "screenshot", Title: "Take a screenshot",
			Description: "Returns a PNG of the running game. target game (default) is the game's own picture at its logical size, scale 1 to 8 " +
				"multiplies it; target window is the whole window with the letterbox. Needs a game with a window (not headless). last: true " +
				"returns the capture the developer took with the screenshot key in the window instead of taking a new one. Nothing is written to disk.",
			Input: mcp.Object(map[string]*mcp.Schema{
				"target":     mcp.Enum("game (default) or window", "game", "window"),
				"scale":      mcp.Integer("whole-number scale of a game capture, 1 to 8 (default 1)", 1, 8),
				"keep_alpha": mcp.Boolean("keep transparency instead of the opaque picture"),
				"last":       mcp.Boolean("return the game's latest own capture (hotkey) instead of taking one"),
			}),
			Annotations: readOnly,
			Handler:     t.screenshot,
		}}
}
