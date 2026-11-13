package collision

import (
	"strings"
	"testing"
)

func mustPanic(t *testing.T, want string, f func()) {
	t.Helper()
	defer func() {
		r := recover()
		if r == nil {
			t.Fatalf("expected panic containing %q", want)
		}
		if msg, _ := r.(string); !strings.Contains(msg, want) {
			t.Fatalf("panic %v does not contain %q", r, want)
		}
	}()
	f()
}

func TestLayers(t *testing.T) {
	player, enemy, wall := Layer(1), Layer(2), Layer(4)
	both := LayersOf(enemy, wall)
	tests := []struct {
		name string
		got  any
		want any
	}{
		{"Layer(0) is the default", Layer(0), DefaultLayer},
		{"Layer(31)", Layer(31), Layers(1 << 31)},
		{"LayersOf", both, Layers(0b10100)},
		{"LayersOf none", LayersOf(), NoLayers},
		{"Has all", both.Has(enemy | wall), true},
		{"Has partial", both.Has(enemy | player), false},
		{"Any partial", both.Any(enemy | player), true},
		{"Any none", both.Any(player), false},
		{"With", both.With(player), Layers(0b10110)},
		{"Without", both.Without(wall), enemy},
		{"IsEmpty", NoLayers.IsEmpty(), true},
		{"not IsEmpty", player.IsEmpty(), false},
		{"String numbers", LayersOf(Layer(1), Layer(3), Layer(4)).String(), "1|3|4"},
		{"String none", NoLayers.String(), "none"},
		{"AllLayers", AllLayers.Has(Layer(31) | DefaultLayer), true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.got != tc.want {
				t.Errorf("got %v, want %v", tc.got, tc.want)
			}
		})
	}
	mustPanic(t, "collision: layer 32 out of range 0..31", func() { Layer(32) })
	mustPanic(t, "collision: layer -1 out of range 0..31", func() { Layer(-1) })
}

func TestNameLayers(t *testing.T) {
	t.Cleanup(func() { layerNames.Store(nil) })
	NameLayers(map[Layers]string{Layer(1): "player", Layer(4): "wall"})
	if got := LayersOf(Layer(1), Layer(2), Layer(4)).String(); got != "player|2|wall" {
		t.Errorf("String = %q", got)
	}
	mustPanic(t, "collision: NameLayers: 6 is not a single layer", func() {
		NameLayers(map[Layers]string{Layer(1) | Layer(2): "x"})
	})
}

func TestColliderDetects(t *testing.T) {
	player, pickup := Layer(1), Layer(3)
	hero := Collider{Layer: player, Mask: LayersOf(pickup)}
	coin := Collider{Layer: pickup}
	plain := Collider{} // on the default layer, reacts to nothing
	if !hero.Detects(&coin) || coin.Detects(&hero) {
		t.Error("hero should detect the coin, the coin nothing")
	}
	if !coin.DetectedBy(&hero) {
		t.Error("coin should be detected by the hero")
	}
	if plain.LayerOrDefault() != DefaultLayer {
		t.Error("zero Layer should be the default layer")
	}
	all := Collider{Mask: AllLayers}
	if !all.Detects(&plain) {
		t.Error("AllLayers should detect a zero-Layer collider")
	}
}
