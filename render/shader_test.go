package render

import (
	"strings"
	"testing"
	"testing/fstest"

	"github.com/weakpixel/asset"
)

const kageOK = `//kage:unit pixels

package main

func Fragment(dst vec4, src vec2, color vec4) vec4 {
	return color
}
`

func TestShaderLoads(t *testing.T) {
	e := newAssetEnv(t, fstest.MapFS{"fx/tint.kage": textFile(kageOK)}, 2)
	sc := e.srv.NewScope("s")
	h := asset.Load[Shader](sc, "fx/tint.kage")
	e.settle(sc)

	shaders := asset.StoreOf[Shader](e.srv)
	if !shaders.Ready(h) || shaders.Get(h).Shader == nil {
		t.Fatalf("shader not ready: %v", shaders.Err(h))
	}
	sc.Release()
	e.srv.Update()
	if shaders.Ready(h) {
		t.Fatal("shader still loaded after release")
	}
}

func TestShaderCompileErrorFailsAsset(t *testing.T) {
	e := newAssetEnv(t, fstest.MapFS{"bad.kage": textFile("package main\nfunc Fragment( {")}, 0)
	sc := e.srv.NewScope("s")
	h := asset.Load[Shader](sc, "bad.kage")
	e.settle(sc)

	shaders := asset.StoreOf[Shader](e.srv)
	if shaders.Err(h) == nil {
		t.Fatal("a shader that does not compile must fail")
	}
	if shaders.Get(h).Shader != nil {
		t.Fatal("failed shader did not resolve to the no-shader placeholder")
	}
	if len(e.errs) != 1 {
		t.Fatalf("OnError calls = %v", e.errs)
	}
}

func TestShaderZeroHandleIsNoShader(t *testing.T) {
	e := newAssetEnv(t, nil, 0)
	if asset.StoreOf[Shader](e.srv).Get(ShaderID{}).Shader != nil {
		t.Fatal("zero ShaderID must resolve to no shader")
	}
}

func TestShaderUniformTypeMismatchFailsAsset(t *testing.T) {
	const src = `//kage:unit pixels

package main

var Params float

func Fragment(dst vec4, src vec2, color vec4) vec4 {
	return color * Params
}
`
	e := newAssetEnv(t, fstest.MapFS{"fx/bad.kage": textFile(src)}, 0)
	sc := e.srv.NewScope("s")
	h := LoadShader(sc, "fx/bad.kage")
	e.settle(sc)

	shaders := ShaderStoreOf(e.srv)
	err := shaders.Err(h)
	if err == nil || !strings.Contains(err.Error(), "var Params vec4") {
		t.Fatalf("Err = %v, want a uniform type error naming Params vec4", err)
	}
	if shaders.Get(h).Shader != nil {
		t.Fatal("shader with a bad uniform did not resolve to the no-shader placeholder")
	}
}

func TestShaderStoreFacade(t *testing.T) {
	e := newAssetEnv(t, fstest.MapFS{"fx/tint.kage": textFile(kageOK)}, 0)
	h, err := LoadShaderSync(e.srv.NewScope("s"), "fx/tint.kage")
	if err != nil {
		t.Fatal(err)
	}
	st := StoresOf(e.srv).Shaders
	if !st.Ready(h) || st.Get(h).Shader == nil || st.Err(h) != nil {
		t.Fatalf("shader not ready: %v", st.Err(h))
	}
	if got := st.Key(h).String(); got != "fx/tint.kage" {
		t.Fatalf("Key = %q", got)
	}
	if st.Generic() != asset.StoreOf[Shader](e.srv) {
		t.Fatal("Generic is not the server's shader store")
	}
}

func TestShaderTimeUniform(t *testing.T) {
	const declared = `//kage:unit pixels

package main

var Time float

func Fragment(dst vec4, src vec2, color vec4) vec4 {
	return color * fract(Time)
}
`
	const mismatch = `//kage:unit pixels

package main

var Time vec2

func Fragment(dst vec4, src vec2, color vec4) vec4 {
	return color * Time.x
}
`
	e := newAssetEnv(t, fstest.MapFS{"ok.kage": textFile(declared), "bad.kage": textFile(mismatch)}, 0)
	sc := e.srv.NewScope("s")
	ok, bad := LoadShader(sc, "ok.kage"), LoadShader(sc, "bad.kage")
	e.settle(sc)

	shaders := ShaderStoreOf(e.srv)
	if !shaders.Ready(ok) {
		t.Fatalf("shader declaring var Time float does not load: %v", shaders.Err(ok))
	}
	if err := shaders.Err(bad); err == nil || !strings.Contains(err.Error(), "var Time float") {
		t.Fatalf("Err = %v, want a uniform type error naming Time float", err)
	}
}
