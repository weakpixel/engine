package camera

import (
	"image"
	"testing"

	"github.com/weakpixel/engine/gfx"
	"github.com/weakpixel/engine/render"
)

func TestBounds(t *testing.T) {
	screen := image.Pt(320, 180)
	level := gfx.Rect{Min: vec(0, 0), Max: vec(1000, 500)}
	tests := []struct {
		name   string
		cam    render.Camera
		bounds gfx.Rect
		p      gfx.Vec2
		want   gfx.Vec2
	}{
		{"unbounded", render.Camera{}, gfx.Rect{}, vec(-5000, 9000), vec(-5000, 9000)},
		{"inside", render.Camera{}, level, vec(500, 250), vec(500, 250)},
		{"top-left corner", render.Camera{}, level, vec(-50, -50), vec(160, 90)},
		{"bottom-right corner", render.Camera{}, level, vec(2000, 2000), vec(840, 410)},
		{"zoom 2 shows less", render.Camera{Zoom: 2}, level, vec(0, 0), vec(80, 45)},
		{"zoom 0.5 shows more", render.Camera{Zoom: 0.5}, level, vec(0, 0), vec(320, 180)},
		{"custom viewport", render.Camera{Viewport: image.Rect(0, 0, 100, 60)}, level, vec(0, 0), vec(50, 30)},
		{"narrower than the view centers X", render.Camera{},
			gfx.Rect{Min: vec(0, 0), Max: vec(200, 500)}, vec(-80, 0), vec(100, 90)},
		{"shorter than the view centers Y", render.Camera{},
			gfx.Rect{Min: vec(0, 100), Max: vec(1000, 200)}, vec(500, 999), vec(500, 150)},
		{"smaller on both axes", render.Camera{Zoom: 0.1}, level, vec(3, 3), vec(500, 250)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			half := viewHalfSize(&tt.cam, screen)
			if got := clampToBounds(tt.p, half, tt.bounds); !near(got, tt.want, 1e-3) {
				t.Errorf("clamp(%v) = %v, want %v", tt.p, got, tt.want)
			}
		})
	}
}
