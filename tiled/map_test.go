package tiled

import (
	"bytes"
	"compress/gzip"
	"compress/zlib"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"slices"
	"strings"
	"testing"
)

// layerNames lists the layers depth-first as "path:kind".
func layerNames(ls []Layer, prefix string) []string {
	var out []string
	for _, l := range ls {
		p := prefix + l.Name
		out = append(out, p+":"+l.Kind.String())
		out = append(out, layerNames(l.Layers, p+"/")...)
	}
	return out
}

func TestDecodeMapRealLevels(t *testing.T) {
	want := []string{
		"gb-image:imagelayer", "main:tilelayer", "deco:tilelayer", "shadow:tilelayer",
		"entity-group:group", "entity-group/entities:tilelayer", "entity-group/entities:objectgroup",
		"forground:tilelayer", "colliders:group", "colliders/collider-tiles:tilelayer",
	}
	for _, tc := range []struct {
		file              string
		objects           int
		imgOffsetX        float64
		forground         bool
		groupOpacity      float64
		colliderOpacity   float64
		colliderLayerType string
	}{
		{"level-1.tmj", 3, -896, false, 1, 0.2, ""},
		{"level-2.tmj", 33, 0, true, 0.5, 0.5, "colliders"},
	} {
		t.Run(tc.file, func(t *testing.T) {
			m, err := DecodeMap(readTestdata(t, tc.file))
			if err != nil {
				t.Fatal(err)
			}
			if m.Width != 40 || m.Height != 40 || m.TileWidth != 16 || m.TileHeight != 16 || m.Infinite || m.RenderOrder != "right-down" {
				t.Errorf("map = %+v", m)
			}
			if len(m.Tilesets) != 2 || m.Tilesets[0] != (MapTileset{FirstGID: 1, Source: "tiles/temple.tsj"}) ||
				m.Tilesets[1] != (MapTileset{FirstGID: 321, Source: "tiles/colliders.tsj"}) {
				t.Errorf("tilesets = %+v", m.Tilesets)
			}
			if got := layerNames(m.Layers, ""); !slices.Equal(got, want) {
				t.Errorf("layers = %v", got)
			}
			img := m.Layers[0]
			if img.Image != "bg/black-tri.png" || img.ImageWidth != 1536 || img.ImageHeight != 1024 || img.Visible || img.Offset.X != tc.imgOffsetX {
				t.Errorf("image layer = %+v", img)
			}
			main := m.Layers[1]
			if len(main.Data) != 1600 || main.Opacity != 1 || !main.Visible || main.Parallax != (Point{1, 1}) {
				t.Errorf("main: %d cells, opacity %v, parallax %v", len(main.Data), main.Opacity, main.Parallax)
			}
			group := m.Layers[4]
			if mode, ok := group.Props.String("layer-render-mode"); !ok || mode != "merge" {
				t.Errorf("layer-render-mode = %q, %v", mode, ok)
			}
			objs := group.Layers[1]
			if len(objs.Objects) != tc.objects || objs.DrawOrder != "topdown" {
				t.Errorf("entities: %d objects, draworder %q", len(objs.Objects), objs.DrawOrder)
			}
			if m.Layers[5].Visible != tc.forground {
				t.Errorf("forground visible = %v", m.Layers[5].Visible)
			}
			colliders := m.Layers[6]
			tiles := colliders.Layers[0]
			if colliders.Opacity != tc.groupOpacity || tiles.Opacity != tc.colliderOpacity || tiles.Class != tc.colliderLayerType {
				t.Errorf("colliders opacity %v, tiles opacity %v class %q", colliders.Opacity, tiles.Opacity, tiles.Class)
			}
			if pass, _ := tiles.Props.String("renderPass"); pass != "debug" {
				t.Errorf("renderPass = %q", pass)
			}
		})
	}

	m, _ := DecodeMap(readTestdata(t, "level-2.tmj"))
	objs := m.Layers[4].Layers[1].Objects
	var player, start *Object
	chicks := 0
	for i := range objs {
		switch objs[i].Class {
		case "player":
			player = &objs[i]
		case "start":
			start = &objs[i]
		case "baby-chick":
			chicks++
			if mass, ok := objs[i].Props.Float("mass"); !ok || mass != 1 {
				t.Errorf("chick %d mass = %v, %v", objs[i].ID, mass, ok)
			}
		}
	}
	if player == nil || player.Name != "player" || player.Shape != ShapeRect || player.Width != 0 || player.X < 187 || player.X > 188 {
		t.Errorf("player = %+v", player)
	}
	if start == nil || start.Shape != ShapePoint {
		t.Errorf("start = %+v", start)
	}
	if chicks != 30 {
		t.Errorf("%d chicks, want 30", chicks)
	}
}

// encodeGIDs returns the data of gids in the given encoding and compression,
// as a JSON value.
func encodeGIDs(t *testing.T, gids []uint32, compression string) string {
	t.Helper()
	var raw bytes.Buffer
	for _, g := range gids {
		binary.Write(&raw, binary.LittleEndian, g)
	}
	b := raw.Bytes()
	var out bytes.Buffer
	switch compression {
	case "":
		out.Write(b)
	case "zlib":
		w := zlib.NewWriter(&out)
		w.Write(b)
		w.Close()
	case "gzip":
		w := gzip.NewWriter(&out)
		w.Write(b)
		w.Close()
	}
	return `"` + base64.StdEncoding.EncodeToString(out.Bytes()) + `"`
}

// smallMap is a 3x2 orthogonal map with one tile layer whose "data",
// "encoding" and "compression" fields are given.
func smallMap(layerFields string) string {
	return `{"version":"1.10","orientation":"orthogonal","width":3,"height":2,"tilewidth":8,"tileheight":8,
		"tilesets":[{"firstgid":1,"source":"t.tsj"}],
		"layers":[{"type":"tilelayer","name":"l","width":3,"height":2,` + layerFields + `}]}`
}

func TestDecodeMapEncodings(t *testing.T) {
	gids := []uint32{1, 0, 3, 0x80000002, 5, 0}
	csv, err := DecodeMap([]byte(smallMap(`"data":[1,0,3,2147483650,5,0]`)))
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(csv.Layers[0].Data, gids) {
		t.Fatalf("csv = %v", csv.Layers[0].Data)
	}
	for _, comp := range []string{"", "zlib", "gzip"} {
		t.Run("base64-"+comp, func(t *testing.T) {
			fields := fmt.Sprintf(`"encoding":"base64","compression":%q,"data":%s`, comp, encodeGIDs(t, gids, comp))
			m, err := DecodeMap([]byte(smallMap(fields)))
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(m.Layers[0].Data, gids) {
				t.Fatalf("data = %v, want %v", m.Layers[0].Data, gids)
			}
		})
	}
}

func TestDecodeMapChunks(t *testing.T) {
	m, err := DecodeMap([]byte(`{"orientation":"orthogonal","infinite":true,"width":4,"height":4,"tilewidth":8,"tileheight":8,
		"layers":[{"type":"tilelayer","name":"l","startx":-2,"starty":-1,"width":4,"height":3,
			"chunks":[{"x":-2,"y":-1,"width":2,"height":2,"data":[1,2,3,4]},
			          {"x":0,"y":0,"width":2,"height":2,"data":[5,0,0,6]}]}]}`))
	if err != nil {
		t.Fatal(err)
	}
	l := m.Layers[0]
	if !m.Infinite || l.Data != nil || len(l.Chunks) != 2 || l.StartX != -2 || l.StartY != -1 {
		t.Fatalf("layer = %+v", l)
	}
	if c := l.Chunks[0]; c.X != -2 || c.Y != -1 || !slices.Equal(c.Data, []uint32{1, 2, 3, 4}) {
		t.Errorf("chunk 0 = %+v", c)
	}
}

func TestDecodeMapGroupsAndDefaults(t *testing.T) {
	m, err := DecodeMap([]byte(`{"orientation":"orthogonal","width":1,"height":1,"tilewidth":8,"tileheight":8,
		"backgroundcolor":"#336699","parallaxoriginx":10,"parallaxoriginy":20,"class":"cave",
		"properties":[{"name":"music","type":"file","value":"cave.ogg"}],
		"tilesets":[{"firstgid":1,"name":"props","tilewidth":8,"tileheight":8,"tilecount":1,
			"tiles":[{"id":0,"image":"rock.png","imagewidth":8,"imageheight":8}]}],
		"layers":[{"type":"group","name":"g","offsetx":4,"parallaxx":0.5,"tintcolor":"#ff0000","layers":[
			{"type":"imagelayer","name":"sky","image":"sky.png","repeatx":true},
			{"type":"objectgroup","name":"o","draworder":"index","objects":[]}]}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if m.Background != 0x336699ff || m.ParallaxOrigin != (Point{10, 20}) || m.Class != "cave" {
		t.Errorf("map = %+v", m)
	}
	if f, _ := m.Props.File("music"); f != "cave.ogg" {
		t.Errorf("music = %q", f)
	}
	if ts := m.Tilesets[0]; ts.Embedded == nil || !ts.Embedded.IsCollection() || ts.Embedded.Tiles[0].Image != "rock.png" {
		t.Errorf("embedded = %+v", ts)
	}
	g := m.Layers[0]
	if g.Kind != GroupLayer || g.Offset.X != 4 || g.Parallax != (Point{0.5, 1}) || g.Tint != 0xff0000ff || len(g.Layers) != 2 {
		t.Errorf("group = %+v", g)
	}
	if sky := g.Layers[0]; !sky.RepeatX || sky.RepeatY || sky.Image != "sky.png" || !sky.Visible || sky.Opacity != 1 {
		t.Errorf("sky = %+v", sky)
	}
	if o := g.Layers[1]; o.DrawOrder != "index" {
		t.Errorf("draworder = %q", o.DrawOrder)
	}
}

func TestDecodeMapErrors(t *testing.T) {
	for _, tc := range []struct {
		name string
		json string
		want []string
	}{
		{"json", `[`, []string{"map: invalid JSON"}},
		{"orientation", strings.Replace(smallMap(`"data":[0,0,0,0,0,0]`), `"orthogonal"`, `"isometric"`, 1),
			[]string{`unsupported Tiled feature: orientation: "isometric"`}},
		{"version", strings.Replace(smallMap(`"data":[0,0,0,0,0,0]`), `"1.10"`, `"2.0"`, 1),
			[]string{"unsupported Tiled feature: format version"}},
		{"zstd", smallMap(`"encoding":"base64","compression":"zstd","data":"AAAA"`),
			[]string{`layer "l"`, "unsupported Tiled feature: compression", "zstd"}},
		{"cell count", smallMap(`"data":[1,2]`), []string{`layer "l"`, "data has 2 cells, want 6"}},
		{"template", `{"orientation":"orthogonal","width":1,"height":1,"tilewidth":8,"tileheight":8,
			"layers":[{"type":"group","name":"g","layers":[{"type":"objectgroup","name":"o","objects":[{"id":3,"template":"door.tx"}]}]}]}`,
			[]string{`layer "g/o": object 3: unsupported Tiled feature: object templates`, "door.tx"}},
		{"embedded single image", `{"orientation":"orthogonal","width":1,"height":1,"tilewidth":8,"tileheight":8,
			"tilesets":[{"firstgid":1,"name":"t","image":"t.png","tilewidth":8,"tileheight":8,"columns":1,"tilecount":1}]}`,
			[]string{"unsupported Tiled feature: embedded single-image tilesets", "export the tileset to a .tsj file"}},
		{"all at once", `{"orientation":"staggered","width":1,"height":1,"tilewidth":8,"tileheight":8,
			"tilesets":[{"firstgid":5,"source":"a.tsj"},{"firstgid":2,"source":"b.tsj"}],
			"layers":[
				{"type":"tilelayer","name":"a","width":1,"height":1,"data":[1,2]},
				{"type":"tilelayer","name":"b","width":1,"height":1,"encoding":"base64","compression":"zstd","data":"AAAA"},
				{"type":"objectgroup","name":"c","objects":[{"id":1,"template":"x.tx"},{"id":2,"text":{"color":"bad"}}]},
				{"type":"weird","name":"d"}]}`,
			[]string{"orientation", "firstgid must be positive and increasing", `layer "a": data has 2 cells`,
				`layer "b": unsupported Tiled feature: compression`, `layer "c": object 1: unsupported`,
				`layer "c": object 2: text: color "bad"`, `layer "d": unknown layer type "weird"`}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := DecodeMap([]byte(tc.json))
			mustContain(t, err, tc.want...)
		})
	}
}
