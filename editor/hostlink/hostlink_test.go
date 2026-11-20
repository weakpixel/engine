package hostlink_test

import (
	"testing"

	"github.com/weakpixel/engine/editor/hostlink"
)

// TestNativeSettings checks the environment side: a native game learns its
// stage, first file and headless mode from the host's variables and has no
// host page.
func TestNativeSettings(t *testing.T) {
	t.Setenv(hostlink.EnvStage, "prefab")
	t.Setenv(hostlink.EnvPath, "a/one.prefab.json")
	t.Setenv(hostlink.EnvHeadless, "1")
	if got := hostlink.Stage(); got != "prefab" {
		t.Errorf("Stage = %q", got)
	}
	if got := hostlink.Path(); got != "a/one.prefab.json" {
		t.Errorf("Path = %q", got)
	}
	if !hostlink.Headless() {
		t.Error("Headless = false with WEAKPIXEL_HEADLESS=1")
	}
	t.Setenv(hostlink.EnvHeadless, "0")
	if hostlink.Headless() {
		t.Error("Headless = true with WEAKPIXEL_HEADLESS=0")
	}
	if _, ok := hostlink.ConnectPage(func([]byte) {}); ok {
		t.Error("ConnectPage found a host page outside a browser")
	}
	if base, ok := hostlink.AssetBase(); ok || base != "" {
		t.Errorf("AssetBase = %q, %v", base, ok)
	}
	if hostlink.Untitled() || hostlink.Files() != nil {
		t.Error("Untitled or Files set outside a browser")
	}
}
