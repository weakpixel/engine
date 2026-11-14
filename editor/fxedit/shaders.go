package fxedit

import (
	"embed"
	"path"
	"strings"
)

// builtinShaders holds the sources of the built-in shaders. Like the
// sprites they are served under [BuiltinPrefix] while editing, and Save as
// copies the one in use next to the effect.
//
//go:embed shaders/*.kage
var builtinShaders embed.FS

// Keys of the built-in shaders.
const (
	// Burn burns particles away along a glowing edge; see its source.
	Burn = BuiltinPrefix + "burn.kage"
)

// BuiltinShaderKeys returns the asset keys of the built-in shaders.
func BuiltinShaderKeys() []string {
	entries, _ := builtinShaders.ReadDir("shaders")
	keys := make([]string, len(entries))
	for i, e := range entries {
		keys[i] = BuiltinPrefix + e.Name()
	}
	return keys
}

// BuiltinShader returns the Kage source of the built-in shader key.
func BuiltinShader(key string) ([]byte, bool) {
	if !IsBuiltin(key) || path.Ext(key) != ".kage" {
		return nil, false
	}
	data, err := builtinShaders.ReadFile("shaders/" + strings.TrimPrefix(key, BuiltinPrefix))
	return data, err == nil
}
