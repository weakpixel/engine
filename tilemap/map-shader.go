package tilemap

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/weakpixel/engine/tiled"
)

// Layer properties that put a tile or image layer through a shader (see
// [Layer.Shader]).
const (
	// PropShader is the .kage key of the layer's shader, relative to the map
	// file like an image layer's image.
	PropShader = "shader"
	// PropShaderParams is up to four comma-separated numbers for the
	// shader's Params uniform; missing ones are 0.
	PropShaderParams = "shaderParams"
)

// layerShaderKey returns the shader key of a tile or image layer.
func layerShaderKey(l *tiled.Layer) string {
	if l.Kind != tiled.TileLayer && l.Kind != tiled.ImageLayer {
		return ""
	}
	key, _ := l.Props.String(PropShader)
	return key
}

// parseShaderParams parses the shaderParams property of layer path, if it
// has one.
func parseShaderParams(props tiled.Properties, path string) (p [4]float32, err error) {
	s, ok := props.String(PropShaderParams)
	if !ok || strings.TrimSpace(s) == "" {
		return p, nil
	}
	parts := strings.Split(s, ",")
	if len(parts) > len(p) {
		return p, fmt.Errorf("layer %q: shaderParams %q has %d values, want at most %d", path, s, len(parts), len(p))
	}
	var errs []error
	for i, part := range parts {
		v, perr := strconv.ParseFloat(strings.TrimSpace(part), 32)
		if perr != nil {
			errs = append(errs, fmt.Errorf("layer %q: shaderParams %q: value %d is not a number", path, s, i+1))
			continue
		}
		p[i] = float32(v)
	}
	return p, errors.Join(errs...)
}
