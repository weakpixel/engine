package fxedit

import (
	"strings"
	"testing"
)

// A new effect names a copy of its built-in sprite next to it, never the
// built-in one, and returns the copy to write.
func TestNewEffect(t *testing.T) {
	text, files := NewEffect("fx/spark.fx.json")
	if len(files) != 1 || files[0].Path != "fx/spark.png" || len(files[0].Data) == 0 {
		t.Fatalf("files = %+v", files)
	}
	if !strings.Contains(string(text), `"fx/spark.png"`) || strings.Contains(string(text), BuiltinPrefix) {
		t.Errorf("text = %s", text)
	}
}
