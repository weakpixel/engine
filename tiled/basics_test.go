package tiled

import (
	"encoding/json"
	"testing"
)

func TestSplitGID(t *testing.T) {
	for _, tc := range []struct {
		raw  uint32
		gid  uint32
		flip Flip
	}{
		{0, 0, 0},
		{92, 92, 0},
		{0x80000000 | 92, 92, FlipH},
		{0x40000000 | 92, 92, FlipV},
		{0x20000000 | 92, 92, FlipD},
		{0xc0000000 | 92, 92, FlipH | FlipV},
		{0xa0000000 | 92, 92, FlipH | FlipD},
		{0x60000000 | 92, 92, FlipV | FlipD},
		{0xe0000000 | 92, 92, FlipH | FlipV | FlipD},
		{0x10000000 | 7, 7, FlipHex},
		{0xf0000000 | 0x0fffffff, 0x0fffffff, FlipH | FlipV | FlipD | FlipHex},
	} {
		gid, flip := SplitGID(tc.raw)
		if gid != tc.gid || flip != tc.flip {
			t.Errorf("SplitGID(%#x) = %d, %b; want %d, %b", tc.raw, gid, flip, tc.gid, tc.flip)
		}
		if back := JoinGID(gid, flip); back != tc.raw {
			t.Errorf("JoinGID(%d, %b) = %#x, want %#x", gid, flip, back, tc.raw)
		}
	}
}

func TestParseColor(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want Color
		bad  bool
	}{
		{"", 0, false},
		{"#ff8000", 0xff8000ff, false},
		{"ff8000", 0xff8000ff, false},
		{"#80ff8000", 0xff800080, false},
		{"#00000000", 0, false},
		{"#fff", 0, true},
		{"#gg0000", 0, true},
	} {
		c, err := ParseColor(tc.in)
		if (err != nil) != tc.bad || c != tc.want {
			t.Errorf("ParseColor(%q) = %#x, %v; want %#x, bad=%v", tc.in, uint32(c), err, uint32(tc.want), tc.bad)
		}
	}
	if s := Color(0xff800080).String(); s != "#80ff8000" {
		t.Errorf("String = %q", s)
	}
}

func TestProperties(t *testing.T) {
	var raw []rawProperty
	if err := json.Unmarshal([]byte(`[
		{"name":"s","type":"string","value":"hi"},
		{"name":"untyped","value":"plain"},
		{"name":"f","type":"file","value":"../x.png"},
		{"name":"c","type":"color","value":"#80ff0000"},
		{"name":"i","type":"int","value":3},
		{"name":"fl","type":"float","value":1.5},
		{"name":"fi","type":"float","value":1},
		{"name":"b","type":"bool","value":true},
		{"name":"o","type":"object","value":14},
		{"name":"e","type":"string","propertytype":"Kind","value":"wall"},
		{"name":"cls","type":"class","propertytype":"Stats","value":{"hp":10,"speed":2.5,"name":"bug","tint":"#ff0000","alive":true,"inner":{"x":1}}}
	]`), &raw); err != nil {
		t.Fatal(err)
	}
	p, err := decodeProperties(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(p) != 11 || p[0].Name != "s" || p[10].Name != "cls" {
		t.Fatalf("order: %+v", p)
	}
	check := func(name string, got, want any, ok bool) {
		t.Helper()
		if !ok || got != want {
			t.Errorf("%s = %v (ok %v), want %v", name, got, ok, want)
		}
	}
	s, ok := p.String("s")
	check("String(s)", s, "hi", ok)
	s, ok = p.String("untyped")
	check("String(untyped)", s, "plain", ok)
	s, ok = p.String("f")
	check("String(f)", s, "../x.png", ok)
	s, ok = p.File("f")
	check("File(f)", s, "../x.png", ok)
	c, ok := p.Color("c")
	check("Color(c)", c, Color(0xff000080), ok)
	i, ok := p.Int("i")
	check("Int(i)", i, 3, ok)
	f, ok := p.Float("fl")
	check("Float(fl)", f, 1.5, ok)
	f, ok = p.Float("fi")
	check("Float(fi)", f, 1.0, ok)
	f, ok = p.Float("i")
	check("Float(i) widens", f, 3.0, ok)
	b, ok := p.Bool("b")
	check("Bool(b)", b, true, ok)
	o, ok := p.Object("o")
	check("Object(o)", o, 14, ok)
	if pr, _ := p.Get("e"); pr.PropertyType != "Kind" {
		t.Errorf("PropertyType = %q", pr.PropertyType)
	}

	cls, ok := p.Class("cls")
	if !ok || len(cls) != 6 || cls[0].Name != "alive" {
		t.Fatalf("Class(cls) = %+v, %v", cls, ok)
	}
	hp, ok := cls.Int("hp")
	check("member hp", hp, 10, ok)
	speed, ok := cls.Float("speed")
	check("member speed", speed, 2.5, ok)
	tint, ok := cls.Color("tint")
	check("member tint", tint, Color(0xff0000ff), ok)
	alive, ok := cls.Bool("alive")
	check("member alive", alive, true, ok)
	inner, ok := cls.Class("inner")
	if !ok {
		t.Fatal("member inner is not a class")
	}
	x, ok := inner.Int("x")
	check("inner x", x, 1, ok)

	// Wrong types are ok=false.
	for name, got := range map[string]bool{
		"Int(s)":     second(p.Int("s")),
		"Int(o)":     second(p.Int("o")),
		"Int(fl)":    second(p.Int("fl")),
		"Float(s)":   second(p.Float("s")),
		"Bool(i)":    second(p.Bool("i")),
		"String(i)":  second(p.String("i")),
		"File(s)":    second(p.File("s")),
		"Color(s)":   second(p.Color("s")),
		"Object(i)":  second(p.Object("i")),
		"Class(s)":   second(p.Class("s")),
		"Int(none)":  second(p.Int("none")),
		"String(cl)": second(p.String("cls")),
	} {
		if got {
			t.Errorf("%s: ok = true, want false", name)
		}
	}
}

func second[T any](_ T, ok bool) bool { return ok }

func TestPropertyErrors(t *testing.T) {
	var raw []rawProperty
	if err := json.Unmarshal([]byte(`[
		{"name":"a","type":"int","value":1.5},
		{"name":"b","type":"color","value":"red"},
		{"name":"c","type":"bool","value":"yes"},
		{"name":"d","type":"vector","value":1}
	]`), &raw); err != nil {
		t.Fatal(err)
	}
	_, err := decodeProperties(raw)
	mustContain(t, err, `property "a"`, "not an integer", `property "b"`, `property "c"`, `property "d"`, `unknown property type "vector"`)
}

func TestObjectShapesAndClass(t *testing.T) {
	var raw []rawObject
	if err := json.Unmarshal([]byte(`[
		{"id":1,"name":"r","type":"wall","x":1,"y":2,"width":3,"height":4,"rotation":90},
		{"id":2,"type":"old","class":"new","point":true,"x":5,"y":6},
		{"id":3,"ellipse":true,"width":2,"height":2,"visible":false},
		{"id":4,"polygon":[{"x":0,"y":0},{"x":4,"y":0},{"x":0,"y":4}]},
		{"id":5,"polyline":[{"x":0,"y":0},{"x":8,"y":8}]},
		{"id":6,"text":{"text":"hello","color":"#ff0000","pixelsize":12,"halign":"center"}},
		{"id":7,"gid":2147483740,"width":16,"height":16},
		{"id":8,"properties":[{"name":"mass","type":"float","value":1}]}
	]`), &raw); err != nil {
		t.Fatal(err)
	}
	objs, err := decodeObjects(raw)
	if err != nil {
		t.Fatal(err)
	}
	want := []Shape{ShapeRect, ShapePoint, ShapeEllipse, ShapePolygon, ShapePolyline, ShapeText, ShapeTile, ShapeRect}
	for i, o := range objs {
		if o.Shape != want[i] {
			t.Errorf("object %d: shape %v, want %v", o.ID, o.Shape, want[i])
		}
	}
	if o := objs[0]; o.Class != "wall" || o.X != 1 || o.Y != 2 || o.Width != 3 || o.Height != 4 || o.Rotation != 90 || !o.Visible {
		t.Errorf("rect = %+v", o)
	}
	if objs[1].Class != "new" {
		t.Errorf("class beats type: got %q", objs[1].Class)
	}
	if objs[2].Visible {
		t.Error("visible:false ignored")
	}
	if len(objs[3].Points) != 3 || objs[3].Points[1] != (Point{X: 4}) {
		t.Errorf("polygon points = %v", objs[3].Points)
	}
	if tx := objs[5].Text; tx == nil || tx.Text != "hello" || tx.Color != 0xff0000ff || tx.PixelSize != 12 || tx.HAlign != "center" || tx.VAlign != "top" || !tx.Kerning {
		t.Errorf("text = %+v", tx)
	}
	if o := objs[6]; o.GID != 92 || o.Flip != FlipH {
		t.Errorf("tile object gid %d flip %b", o.GID, o.Flip)
	}
	if m, ok := objs[7].Props.Float("mass"); !ok || m != 1 {
		t.Errorf("mass = %v, %v", m, ok)
	}
}
