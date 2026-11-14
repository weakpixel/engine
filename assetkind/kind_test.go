package assetkind_test

import (
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/weakpixel/asset"
	"github.com/weakpixel/engine/assetkind"
)

// handle and other stand in for two handle types of different kinds.
type (
	handle struct{ asset.Ref }
	other  struct{ asset.Ref }
)

var wantKey = asset.Key{Path: "a/b.png", Sub: "x"}

// The kinds are registered once per process, so -count=2 does not register
// a name twice.
func init() {
	assetkind.Register("test-a", assetkind.Of(
		func(*asset.Scope, string) handle { return handle{} },
		func(*asset.Server, handle) asset.Key { return wantKey },
	))
}

func mustPanic(t *testing.T, want string, f func()) {
	t.Helper()
	defer func() {
		t.Helper()
		r := recover()
		if s, ok := r.(string); !ok || !strings.Contains(s, want) {
			t.Errorf("panic %v, want %q", r, want)
		}
	}()
	f()
}

func TestOfAndLookup(t *testing.T) {
	k, ok := assetkind.Lookup("test-a")
	if !ok {
		t.Fatal("test-a is not registered")
	}
	if h, ok := k.Load(nil, "a/b.png").(handle); !ok {
		t.Errorf("Load returned %T, want handle", h)
	}
	if key, ok := k.Key(nil, handle{}); !ok || key != wantKey {
		t.Errorf("Key = %v, %v, want %v, true", key, ok, wantKey)
	}
	if _, ok := k.Key(nil, other{}); ok {
		t.Error("Key accepted a handle of another type")
	}
	if _, ok := assetkind.Lookup("nope"); ok {
		t.Error("an unregistered kind was found")
	}
	if !slices.Contains(assetkind.Names(), "test-a") || !slices.IsSorted(assetkind.Names()) {
		t.Errorf("Names = %v", assetkind.Names())
	}
}

func TestRegisterPanics(t *testing.T) {
	full := assetkind.Of(
		func(*asset.Scope, string) handle { return handle{} },
		func(*asset.Server, handle) asset.Key { return asset.Key{} },
	)
	for _, tc := range []struct {
		name string
		kind assetkind.Kind
		want string
	}{
		{"test-a", full, `assetkind: kind "test-a" is already registered`},
		{"test-nil", assetkind.Kind{}, `assetkind: kind "test-nil" needs Load and Key`},
		{"test-nokey", assetkind.Kind{Load: full.Load}, "needs Load and Key"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mustPanic(t, tc.want, func() { assetkind.Register(tc.name, tc.kind) })
		})
	}
}

// registerOnce guards TestConcurrentRegister against -count=2.
var registerOnce sync.Once

func TestConcurrentRegister(t *testing.T) {
	registerOnce.Do(func() {
		k, _ := assetkind.Lookup("test-a")
		var wg sync.WaitGroup
		for i := range 16 {
			wg.Go(func() {
				assetkind.Register(fmt.Sprintf("test-c%d", i), k)
				assetkind.Lookup("test-a")
				assetkind.Names()
			})
		}
		wg.Wait()
	})
	for i := range 16 {
		if _, ok := assetkind.Lookup(fmt.Sprintf("test-c%d", i)); !ok {
			t.Errorf("test-c%d is missing", i)
		}
	}
}
