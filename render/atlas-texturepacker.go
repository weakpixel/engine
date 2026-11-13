package render

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"image"

	"github.com/weakpixel/asset"
	"github.com/weakpixel/engine/gfx"
)

// TexturePacker JSON ("JSON (Hash)" and "JSON (Array)" export formats). The
// Aseprite sprite sheet export uses the same shape, so those sheets load
// here too; their animation tags and durations are read by package anim.
//
//	{
//	  "frames": { "idle_0": { "frame": {"x":0,"y":0,"w":16,"h":16},
//	                          "spriteSourceSize": {"x":2,"y":1,"w":12,"h":14},
//	                          "sourceSize": {"w":16,"h":16},
//	                          "pivot": {"x":0.5,"y":0.5} } },
//	  "meta": { "image": "hero.png" }
//	}
//
// The array form has "frames": [ {"filename": "idle_0", ... } ]. Only "frame"
// is required. spriteSourceSize and sourceSize describe trimming (they set
// Sprite.Offset and Sprite.Size); pivot sets Sprite.Origin. Rotated frames
// are rejected.
type tpFile struct {
	Frames json.RawMessage `json:"frames"`
	Meta   struct {
		Image string `json:"image"`
	} `json:"meta"`
}

type tpRect struct{ X, Y, W, H int }

type tpFrame struct {
	Filename         string  `json:"filename"` // array form only
	Frame            tpRect  `json:"frame"`
	Rotated          bool    `json:"rotated"`
	SpriteSourceSize *tpRect `json:"spriteSourceSize"`
	SourceSize       *struct {
		W, H int
	} `json:"sourceSize"`
	Pivot *struct {
		X, Y float32
	} `json:"pivot"`
}

// decodeTexturePacker parses the JSON into frame specs, in file order.
func decodeTexturePacker(ctx *asset.DecodeContext, data []byte) (atlasSpec, error) {
	var f tpFile
	if err := json.Unmarshal(data, &f); err != nil {
		return atlasSpec{}, fmt.Errorf("texture packer json: %w", err)
	}
	if f.Meta.Image == "" {
		return atlasSpec{}, errors.New(`texture packer json: "meta.image" is required`)
	}
	frames, err := tpFrames(f.Frames)
	if err != nil {
		return atlasSpec{}, fmt.Errorf("texture packer json: %w", err)
	}
	if len(frames) == 0 {
		return atlasSpec{}, errors.New(`texture packer json: no "frames"`)
	}

	spec := atlasSpec{texture: asset.Dep[Texture](ctx, f.Meta.Image)}
	seen := make(map[string]bool, len(frames))
	var errs []error
	for _, fr := range frames {
		switch {
		case fr.Filename == "":
			errs = append(errs, errors.New("a frame has no name"))
			continue
		case seen[fr.Filename]:
			errs = append(errs, fmt.Errorf("duplicate frame name %q", fr.Filename))
			continue
		case fr.Rotated:
			errs = append(errs, fmt.Errorf("frame %q is rotated; export without rotation", fr.Filename))
			continue
		}
		seen[fr.Filename] = true
		spec.frames = append(spec.frames, fr.spec())
	}
	if err := errors.Join(errs...); err != nil {
		return atlasSpec{}, fmt.Errorf("texture packer json: %w", err)
	}
	return spec, nil
}

// tpFrames reads the "frames" value, an array or an object. The object form
// is read token by token because a Go map would lose the file order, which
// Atlas.Order promises.
func tpFrames(raw json.RawMessage) ([]tpFrame, error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return nil, errors.New(`"frames" is required`)
	}
	switch raw[0] {
	case '[':
		var frames []tpFrame
		err := json.Unmarshal(raw, &frames)
		return frames, err
	case '{':
		dec := json.NewDecoder(bytes.NewReader(raw))
		if _, err := dec.Token(); err != nil { // the opening '{'
			return nil, err
		}
		var frames []tpFrame
		for dec.More() {
			key, err := dec.Token()
			if err != nil {
				return nil, err
			}
			var fr tpFrame
			if err := dec.Decode(&fr); err != nil {
				return nil, err
			}
			fr.Filename = key.(string)
			frames = append(frames, fr)
		}
		return frames, nil
	}
	return nil, errors.New(`"frames" must be an object or an array`)
}

// spec converts a parsed frame. Rectangles are checked against the texture in
// Finish, when its size is known.
func (fr tpFrame) spec() frameSpec {
	r := fr.Frame
	s := frameSpec{
		name:   fr.Filename,
		rect:   image.Rect(r.X, r.Y, r.X+r.W, r.Y+r.H),
		origin: defaultOrigin,
		size:   gfx.Vec2{X: float32(r.W), Y: float32(r.H)},
	}
	if fr.SourceSize != nil {
		s.size = gfx.Vec2{X: float32(fr.SourceSize.W), Y: float32(fr.SourceSize.H)}
	}
	if fr.SpriteSourceSize != nil {
		s.offset = gfx.Vec2{X: float32(fr.SpriteSourceSize.X), Y: float32(fr.SpriteSourceSize.Y)}
	}
	if fr.Pivot != nil {
		s.origin = gfx.Vec2{X: fr.Pivot.X, Y: fr.Pivot.Y}
	}
	return s
}
