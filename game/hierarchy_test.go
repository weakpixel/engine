package game

import (
	"strings"
	"testing"

	"github.com/weakpixel/ecs"
	"github.com/weakpixel/engine/gfx"
)

const (
	visualAt  = `"gfx:Transform":{"Pos":{"X":%s,"Y":0},"PrevPos":{"X":0,"Y":0},"Rot":0,"Scale":{"X":1,"Y":1}},"gfx:Sprite":{"ID":"","Layer":0,"Origin":{"X":0,"Y":0},"FlipX":false,"FlipY":false,"Tint":0,"Blend":0,"Hidden":false}`
	parentOf  = `,"hierarchy:Parent":{"Entity":"%s","Local":{"Pos":{"X":5,"Y":0},"Rot":0,"Scale":{"X":0,"Y":0},"LayerOffset":0,"LayerMode":0,"YSortSide":0}}`
	visualObj = `{"id":"%s","group":"gfx","class":"Visual","components":{%s%s}}`
)

func hierDoc(objs ...string) string { return `{"objects":[` + strings.Join(objs, ",") + `]}` }

func visual(id, x, parent string) string {
	extra := ""
	if parent != "" {
		extra = strings.Replace(parentOf, "%s", parent, 1)
	}
	return strings.Replace(strings.Replace(strings.Replace(visualObj, "%s", id, 1),
		"%s", strings.Replace(visualAt, "%s", x, 1), 1), "%s", extra, 1)
}

func TestHierarchyLevelChildPlacedInFirstTick(t *testing.T) {
	e := New(Config{NoAudio: true})
	defer e.Close()
	l, err := e.LoadLevelData("h.json", []byte(hierDoc(visual("kid", "0", "root"), visual("root", "100", ""))))
	if err != nil {
		t.Fatal(err)
	}
	update(t, e, 1)
	var kid *gfx.Transform
	for _, o := range l.Objects() {
		if o.ID == "kid" {
			kid = ecs.GetComp[gfx.Transform](e.World(), o.Entity)
		}
	}
	if kid.Pos.X != 105 || kid.PrevPos.X != 105 {
		t.Fatalf("child Pos/PrevPos = %v/%v, want 105/105", kid.Pos.X, kid.PrevPos.X)
	}
}

// TestHierarchyFollowsUnderPauseAndHold: the hierarchy stage is not gated by
// a Tool pause or Hold.
func TestHierarchyFollowsUnderPauseAndHold(t *testing.T) {
	tool := &testTool{}
	for range 10 {
		tool.script = append(tool.script, TickPause)
	}
	e := New(Config{NoAudio: true, Tool: tool})
	defer e.Close()
	l, err := e.LoadLevelData("h.json", []byte(hierDoc(visual("root", "100", ""), visual("kid", "0", "root"))))
	if err != nil {
		t.Fatal(err)
	}
	root, kid := l.Objects()[0].Entity, l.Objects()[1].Entity
	update(t, e, 1)
	release := e.Hold()
	defer release()
	ecs.GetComp[gfx.Transform](e.World(), root).Pos.X = 200
	update(t, e, 1)
	if x := ecs.GetComp[gfx.Transform](e.World(), kid).Pos.X; x != 205 {
		t.Fatalf("child x = %v under pause and hold, want 205", x)
	}
}

func TestHierarchyCycleFailsDocument(t *testing.T) {
	e := New(Config{NoAudio: true})
	defer e.Close()
	view := ecs.NewView1[gfx.Transform](e.World())
	_, err := e.LoadLevelData("c.json", []byte(hierDoc(visual("a", "0", "b"), visual("b", "0", "a"), visual("ok", "0", ""))))
	if err == nil || !strings.Contains(err.Error(), "hierarchy: parent cycle") ||
		!strings.Contains(err.Error(), `"a"`) || !strings.Contains(err.Error(), `"b"`) {
		t.Fatalf("error = %v, want a parent cycle naming a and b", err)
	}
	if n := count(view); n != 0 {
		t.Fatalf("%d entities left after a failed document", n)
	}
	update(t, e, 1) // must not panic in the hierarchy stage
}
