package tilemap

import (
	"testing"
)

const testKage = `//kage:unit pixels

package main

var Params vec4

func Fragment(dst vec4, src vec2, color vec4) vec4 {
	return imageSrc0At(src) * color
}
`

// shaderMap is a map with a tile layer and an image layer carrying the
// given shader properties, next to a plain layer.
func shaderMap(t *testing.T, props string) *Map {
	t.Helper()
	files := fixtureFiles(t, `{"orientation":"orthogonal","width":1,"height":1,"tilewidth":8,"tileheight":8,
		"tilesets":[{"firstgid":1,"source":"t.tsj"}],
		"layers":[{"type":"tilelayer","name":"plain","width":1,"height":1,"data":[1]},
			{"type":"tilelayer","name":"water","width":1,"height":1,"data":[1],"properties":[`+props+`]}]}`)
	files["fx/water.kage"] = textFile(testKage)
	srv := newServer(t, files)
	h, err := LoadMapSync(srv.Global(), "m.tmj")
	if err != nil {
		t.Fatal(err)
	}
	return MapStoreOf(srv).Get(h)
}

func TestLayerShaderProperties(t *testing.T) {
	m := shaderMap(t, `{"name":"shader","type":"file","value":"fx/water.kage"},
		{"name":"shaderParams","type":"string","value":"1, 0.5"}`)
	if l := m.Layers[0]; !l.Shader.IsZero() || l.Params != ([4]float32{}) {
		t.Errorf("plain layer = %+v, want no shader", l)
	}
	l := m.Layers[1]
	if l.Shader.IsZero() || l.Params != ([4]float32{1, 0.5}) {
		t.Fatalf("water layer shader %v params %v", l.Shader, l.Params)
	}
}

func TestLayerShaderErrors(t *testing.T) {
	tests := []struct {
		name  string
		props string
		want  []string
	}{
		{"missing kage", `{"name":"shader","type":"string","value":"fx/nope.kage"}`, []string{"fx/nope.kage"}},
		{"bad number", `{"name":"shaderParams","type":"string","value":"1, x"}`, []string{`layer "water"`, "value 2 is not a number"}},
		{"five values", `{"name":"shaderParams","type":"string","value":"1,2,3,4,5"}`, []string{`layer "water"`, "5 values"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			files := fixtureFiles(t, `{"orientation":"orthogonal","width":1,"height":1,"tilewidth":8,"tileheight":8,
				"tilesets":[{"firstgid":1,"source":"t.tsj"}],
				"layers":[{"type":"tilelayer","name":"water","width":1,"height":1,"data":[1],"properties":[`+tc.props+`]}]}`)
			srv := newServer(t, files)
			_, err := LoadMapSync(srv.Global(), "m.tmj")
			mustContain(t, err, tc.want...)
		})
	}
}
