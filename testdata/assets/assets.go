// Package testassets builds a small, artificial asset library for tests of
// the engine and the tools. Nothing in it comes from a demo: the images are
// drawn here and the data files are written by hand, so the library stays a
// few kilobytes and changes only when a test needs it to.
//
// The library (asset keys, every top folder a package):
//
//	global/hero/hero.png            4 frames of 4x4 on a 16x4 sheet
//	global/hero/hero.aseprite.json  array-form sheet, tags idle (0-1) and attack (2-3, ping-pong)
//	global/hero/hero.anim.json      definition with pivot, body, both clips and a hit shape
//	global/input/hero.bindings.json two mappings (play, menu)
//	temple/temple.png, temple.tsj   8 tiles of 16x16 on 4 columns
//	temple/colliders.png, .tsj      3 tiles: 0 a full wall (static), 1 the right half
//	                                with Tiled's rounding noise (y -0.0037), 2 the top half
//	world-1/bg/sky.png              128x64 background
//	world-1/level-1.tmj             6x4 cells, see Level1Layers
//	world-1/level-2.tmj             4x4 cells: main and collider-tiles (see Level2Colliders)
//	world-1/world-1.world           level-1 at (-96, 0), level-2 at (0, 0)
//	world-1/level-1.level.json      an empty level beside level-1.tmj
//
// Tilesets in both maps: temple at firstgid 1, colliders at firstgid 9.
package testassets

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"strings"
	"testing/fstest"
)

// Level1Layers are the layers of world-1/level-1.tmj in draw order, as
// "path:kind". bg is an invisible image layer (offset -16, 0), front is
// hidden, collider-tiles has opacity 0.2, class "colliders" and the string
// property renderPass "debug". The objects layer holds a point "player"
// (class player, at 24,40) and two points of class "chick" (property mass 1).
var Level1Layers = []string{
	"bg:image", "main:tiles", "entity-group:group", "entity-group/entities:tiles",
	"entity-group/entities:objects", "front:tiles", "colliders:group", "colliders/collider-tiles:tiles",
}

// Level1MainTiles is the number of non-empty cells of level-1's main layer.
const Level1MainTiles = 14

// Level2Colliders is level-2's collider-tiles layer, row by row, as tile ids
// of colliders.tsj (-1 is empty).
var Level2Colliders = [4][4]int{
	{-1, -1, -1, -1},
	{-1, 1, 0, -1},
	{-1, -1, 2, -1},
	{0, 0, 0, 0},
}

// Lib returns a new copy of the library; callers may add and delete files.
func Lib() fstest.MapFS {
	text := func(s string) *fstest.MapFile { return &fstest.MapFile{Data: []byte(s)} }
	return fstest.MapFS{
		"global/hero/hero.png":            {Data: sheetPNG()},
		"global/hero/hero.aseprite.json":  text(heroSheet),
		"global/hero/hero.anim.json":      text(heroAnim),
		"global/input/hero.bindings.json": text(bindings),
		"temple/temple.png":               {Data: tilesPNG(4, 2)},
		"temple/temple.tsj":               text(templeTSJ),
		"temple/colliders.png":            {Data: tilesPNG(3, 1)},
		"temple/colliders.tsj":            text(collidersTSJ),
		"world-1/bg/sky.png":              {Data: solidPNG(128, 64, color.RGBA{B: 160, A: 255})},
		"world-1/level-1.tmj":             text(level1()),
		"world-1/level-2.tmj":             text(level2()),
		"world-1/world-1.world":           text(world),
		"world-1/level-1.level.json":      text(`{"name": "Level 1", "objects": []}` + "\n"),
	}
}

// encode returns img as PNG bytes.
func encode(img image.Image) []byte {
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		panic("testassets: " + err.Error())
	}
	return buf.Bytes()
}

func solidPNG(w, h int, c color.RGBA) []byte {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.SetRGBA(x, y, c)
		}
	}
	return encode(img)
}

// tilesPNG draws cols x rows tiles of 16x16, each a different solid colour.
func tilesPNG(cols, rows int) []byte {
	img := image.NewRGBA(image.Rect(0, 0, cols*16, rows*16))
	for y := range rows * 16 {
		for x := range cols * 16 {
			i := uint8((y/16)*cols + x/16)
			img.SetRGBA(x, y, color.RGBA{R: 40 * i, G: 200 - 20*i, B: 90, A: 255})
		}
	}
	return encode(img)
}

// sheetPNG draws four 4x4 frames side by side.
func sheetPNG() []byte {
	img := image.NewRGBA(image.Rect(0, 0, 16, 4))
	for y := range 4 {
		for x := range 16 {
			img.SetRGBA(x, y, color.RGBA{R: uint8(60 * (x / 4)), G: 120, B: 200, A: 255})
		}
	}
	return encode(img)
}

// cells formats a tile layer's data, row by row.
func cells(rows ...[]int) string {
	var parts []string
	for _, r := range rows {
		for _, v := range r {
			parts = append(parts, fmt.Sprint(v))
		}
	}
	return strings.Join(parts, ",")
}

const mapHead = `"type":"map","orientation":"orthogonal","renderorder":"right-down","infinite":false,"tilewidth":16,"tileheight":16,
 "tilesets":[{"firstgid":1,"source":"../temple/temple.tsj"},{"firstgid":9,"source":"../temple/colliders.tsj"}],`

func level1() string {
	main := cells(
		[]int{1, 2, 0, 0, 2, 1},
		[]int{3, 0, 0, 0, 0, 4},
		[]int{5, 0, 0, 0, 0, 6},
		[]int{7, 8, 7, 8, 7, 8},
	)
	entities := cells([]int{0, 0, 0, 0, 0, 0}, []int{0, 0, 0, 0, 0, 0}, []int{0, 0, 3, 0, 0, 0}, []int{0, 0, 0, 0, 0, 0})
	front := cells([]int{0, 0, 0, 0, 0, 0}, []int{0, 0, 0, 0, 0, 0}, []int{0, 0, 0, 0, 0, 0}, []int{0, 4, 0, 0, 4, 0})
	colliders := cells([]int{0, 0, 0, 0, 0, 0}, []int{0, 0, 0, 0, 0, 0}, []int{0, 0, 0, 0, 0, 0}, []int{9, 9, 9, 9, 9, 9})
	return `{` + mapHead + `
 "width":6,"height":4,"layers":[
  {"id":1,"type":"imagelayer","name":"bg","image":"bg/sky.png","imagewidth":128,"imageheight":64,"offsetx":-16,"offsety":0,"opacity":1,"visible":false,"x":0,"y":0},
  {"id":2,"type":"tilelayer","name":"main","width":6,"height":4,"opacity":1,"visible":true,"x":0,"y":0,"data":[` + main + `]},
  {"id":3,"type":"group","name":"entity-group","opacity":1,"visible":true,"x":0,"y":0,"layers":[
   {"id":4,"type":"tilelayer","name":"entities","width":6,"height":4,"opacity":1,"visible":true,"x":0,"y":0,"data":[` + entities + `]},
   {"id":5,"type":"objectgroup","name":"entities","draworder":"topdown","opacity":1,"visible":true,"x":0,"y":0,"objects":[
    {"id":1,"name":"player","type":"player","point":true,"x":24,"y":40,"width":0,"height":0,"rotation":0,"visible":true},
    {"id":2,"name":"","type":"chick","point":true,"x":56,"y":40,"width":0,"height":0,"rotation":0,"visible":true,"properties":[{"name":"mass","type":"float","value":1}]},
    {"id":3,"name":"","type":"chick","point":true,"x":72,"y":24,"width":0,"height":0,"rotation":0,"visible":true,"properties":[{"name":"mass","type":"float","value":1}]}]}]},
  {"id":6,"type":"tilelayer","name":"front","width":6,"height":4,"opacity":1,"visible":false,"x":0,"y":0,"data":[` + front + `]},
  {"id":7,"type":"group","name":"colliders","opacity":1,"visible":true,"x":0,"y":0,"layers":[
   {"id":8,"type":"tilelayer","name":"collider-tiles","class":"colliders","width":6,"height":4,"opacity":0.2,"visible":true,"x":0,"y":0,
    "properties":[{"name":"renderPass","type":"string","value":"debug"}],"data":[` + colliders + `]}]}]}
`
}

func level2() string {
	main := cells([]int{1, 1, 1, 1}, []int{2, 0, 0, 2}, []int{2, 0, 0, 2}, []int{3, 3, 3, 3})
	var rows [][]int
	for _, r := range Level2Colliders {
		var row []int
		for _, id := range r {
			gid := 0
			if id >= 0 {
				gid = 9 + id
			}
			row = append(row, gid)
		}
		rows = append(rows, row)
	}
	return `{` + mapHead + `
 "width":4,"height":4,"layers":[
  {"id":1,"type":"tilelayer","name":"main","width":4,"height":4,"opacity":1,"visible":true,"x":0,"y":0,"data":[` + main + `]},
  {"id":2,"type":"tilelayer","name":"collider-tiles","class":"colliders","width":4,"height":4,"opacity":0.25,"visible":true,"x":0,"y":0,"data":[` + cells(rows...) + `]}]}
`
}

const templeTSJ = `{"type":"tileset","name":"temple","image":"temple.png","imagewidth":64,"imageheight":32,
 "tilewidth":16,"tileheight":16,"columns":4,"tilecount":8,"margin":0,"spacing":0}
`

const collidersTSJ = `{"type":"tileset","name":"colliders","image":"colliders.png","imagewidth":48,"imageheight":16,
 "tilewidth":16,"tileheight":16,"columns":3,"tilecount":3,"margin":0,"spacing":0,"tiles":[
  {"id":0,"type":"wall","properties":[{"name":"static","type":"bool","value":true}],
   "objectgroup":{"type":"objectgroup","draworder":"index","objects":[{"id":1,"type":"wall","x":0,"y":0,"width":16,"height":16,"rotation":0,"visible":true}]}},
  {"id":1,"type":"wall",
   "objectgroup":{"type":"objectgroup","draworder":"index","objects":[{"id":1,"type":"wall","x":8,"y":-0.00366132723112056,"width":8,"height":16,"rotation":0,"visible":true}]}},
  {"id":2,"type":"wall",
   "objectgroup":{"type":"objectgroup","draworder":"index","objects":[{"id":1,"type":"wall","x":0,"y":0,"width":16,"height":8,"rotation":0,"visible":true}]}}]}
`

const world = `{"type":"world","onlyShowAdjacentMaps":false,"maps":[
 {"fileName":"level-1.tmj","x":-96,"y":0,"width":96,"height":64},
 {"fileName":"level-2.tmj","x":0,"y":0,"width":64,"height":64}]}
`

// heroSheet is an Aseprite array export of the four frames of hero.png, each
// trimmed to (2,1) inside an 8x8 source frame.
const heroSheet = `{"frames":[
 {"filename":"f0","frame":{"x":0,"y":0,"w":4,"h":4},"trimmed":true,"spriteSourceSize":{"x":2,"y":1,"w":4,"h":4},"sourceSize":{"w":8,"h":8},"duration":100},
 {"filename":"f1","frame":{"x":4,"y":0,"w":4,"h":4},"trimmed":true,"spriteSourceSize":{"x":2,"y":1,"w":4,"h":4},"sourceSize":{"w":8,"h":8},"duration":100},
 {"filename":"f2","frame":{"x":8,"y":0,"w":4,"h":4},"trimmed":true,"spriteSourceSize":{"x":2,"y":1,"w":4,"h":4},"sourceSize":{"w":8,"h":8},"duration":80},
 {"filename":"f3","frame":{"x":12,"y":0,"w":4,"h":4},"trimmed":true,"spriteSourceSize":{"x":2,"y":1,"w":4,"h":4},"sourceSize":{"w":8,"h":8},"duration":80}],
 "meta":{"image":"hero.png","size":{"w":16,"h":4},"frameTags":[
  {"name":"idle","from":0,"to":1,"direction":"forward"},
  {"name":"attack","from":2,"to":3,"direction":"pingpong"}]}}
`

const heroAnim = `{
  "version": 1,
  "sheet": "hero.aseprite.json",
  "pivot": { "x": 4, "y": 8 },
  "defaultClip": "idle",
  "body": [ { "x": -2, "y": -6, "w": 4, "h": 6 } ],
  "clips": {
    "idle": { "loop": true, "fps": 6 },
    "attack": {
      "loop": false,
      "fps": 12,
      "shapes": [
        { "kind": "hit", "id": "swing", "frames": { "from": 1 }, "rect": { "x": 2, "y": -6, "w": 4, "h": 4 } }
      ]
    }
  }
}
`

const bindings = `{
  "version": 1,
  "groups": { "play": ["play"] },
  "mappings": {
    "play": {
      "bindings": {
        "move": [{ "composite": "dpad", "up": "key:W", "down": "key:S", "left": "key:A", "right": "key:D" },
                 { "control": "pad:leftstick", "processors": [{ "deadzone": 0.2 }] }],
        "jump": [{ "control": "key:Space" }, { "control": "pad:south" }],
        "pause": [{ "control": "key:Escape", "locked": true }]
      }
    },
    "menu": {
      "priority": 20,
      "block": true,
      "bindings": {
        "ui.confirm": [{ "control": "key:Enter" }, { "control": "pad:south" }]
      }
    }
  }
}
`
