package animedit

import (
	"github.com/hajimehoshi/ebiten/v2"

	"github.com/weakpixel/engine/editor/toolshell"
)

// toolButtons are the toolbar's tools, left to right, with their icon file.
var toolButtons = []struct {
	Tool ToolKind
	Icon string
}{
	{ToolSelect, "select"},
	{ToolPivot, "pivot"},
	{ToolHit, "hit"},
	{ToolHurt, "hurt"},
	{ToolBody, "body"},
}

// newToolbar returns the shared toolbar with the anim tool's buttons.
func newToolbar() *toolshell.Toolbar {
	bar := &toolshell.Toolbar{}
	for _, b := range toolButtons {
		bar.Buttons = append(bar.Buttons, toolshell.ToolbarButton{Icon: b.Icon})
	}
	return bar
}

// toolButtonAt returns the tool whose toolbar button is under the window
// point (x, y), if any.
func (t *Tool) toolButtonAt(s *toolshell.Shell, x, y float32) (ToolKind, bool) {
	i, ok := t.bar.At(s, x, y)
	if !ok {
		return 0, false
	}
	return toolButtons[i].Tool, true
}

// drawToolbar draws the tool buttons at the top center of the free area,
// the active one highlighted.
func (t *Tool) drawToolbar(s *toolshell.Shell, screen ebiten.FinalScreen) {
	t.bar.Draw(s, screen, func(i int) bool { return toolButtons[i].Tool == t.st.Tool })
}
