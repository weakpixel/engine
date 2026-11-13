package anim

import (
	"testing"

	"github.com/weakpixel/engine/gfx"
)

func TestAnimBodyUnionAndNone(t *testing.T) {
	e, set := mustLoadDef(t, defJSON(`"body":[{"x":-4,"y":-13,"w":8,"h":7},{"x":-2,"y":-6,"w":10,"h":6}]`))
	if got := e.clip(set, "run").Body; got != (gfx.IRect{X: -4, Y: -13, W: 12, H: 13}) {
		t.Errorf("union body = %+v", got)
	}
	e, set = mustLoadDef(t, defJSON(`"clips":{"idle":{"fps":8}}`))
	if got := e.clip(set, "idle").Body; got != (gfx.IRect{}) {
		t.Errorf("body without a definition body = %+v, want zero", got)
	}
}
