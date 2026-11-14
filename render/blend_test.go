package render

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

func TestEbitenBlend(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   Blend
		want ebiten.Blend
	}{
		{"normal", BlendNormal, ebiten.BlendSourceOver},
		{"additive", BlendAdditive, ebiten.BlendLighter},
		{"multiply", BlendMultiply, blendMultiply},
		{"screen", BlendScreen, blendScreen},
		{"subtract", BlendSubtract, blendSubtract},
		{"lighten", BlendLighten, blendLighten},
		{"erase", BlendErase, ebiten.BlendDestinationOut},
		{"unknown draws normal", Blend(200), ebiten.BlendSourceOver},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := ebitenBlend(tc.in); got != tc.want {
				t.Errorf("ebitenBlend(%d) = %+v, want %+v", tc.in, got, tc.want)
			}
		})
	}
	if blendScreen == blendMultiply || blendSubtract == blendLighten {
		t.Error("blend states must differ")
	}
}
