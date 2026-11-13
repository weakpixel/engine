package render

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/weakpixel/engine/gfx"
)

// shaderGuide is the user guide whose Kage examples must stay valid.
const shaderGuide = "../doc/guide/engine/shaders.md"

// kageBlocks returns the bodies of the ```kage fenced blocks of a Markdown
// document.
func kageBlocks(doc string) []string {
	var blocks []string
	var cur []string
	in := false
	for line := range strings.Lines(doc) {
		switch {
		case !in && strings.TrimSpace(line) == "```kage":
			in, cur = true, nil
		case in && strings.TrimSpace(line) == "```":
			in = false
			blocks = append(blocks, strings.Join(cur, ""))
		case in:
			cur = append(cur, line)
		}
	}
	return blocks
}

// TestShaderGuideExamplesLoad loads every Kage example of the shaders guide
// through the shader loader: each compiles and passes the uniform check.
func TestShaderGuideExamplesLoad(t *testing.T) {
	data, err := os.ReadFile(shaderGuide)
	if err != nil {
		t.Fatal(err)
	}
	blocks := kageBlocks(string(data))
	if len(blocks) < 7 {
		t.Fatalf("found %d kage blocks in %s, want the quick start and six examples", len(blocks), shaderGuide)
	}
	files := fstest.MapFS{}
	for i, b := range blocks {
		files[fmt.Sprintf("ex%d.kage", i)] = textFile(b)
	}
	e := newAssetEnv(t, files, 0)
	sc := e.srv.NewScope("guide")
	handles := make([]gfx.ShaderHandle, len(blocks))
	for i := range blocks {
		handles[i] = LoadShader(sc, fmt.Sprintf("ex%d.kage", i))
	}
	e.settle(sc)
	st := ShaderStoreOf(e.srv)
	for i, h := range handles {
		if !st.Ready(h) {
			t.Errorf("kage block %d of %s does not load: %v", i+1, shaderGuide, st.Err(h))
		}
	}
}
