package debugpanel

import "testing"

func TestShortKeys(t *testing.T) {
	got := shortKeys([]string{"(none)", "fx-builtin/soft-circle.png", "a/very/long/folder/names/spark.png", "b/very/long/folder/names/spark.png"})
	if got[0] != "(none)" {
		t.Errorf("short key changed: %q", got[0])
	}
	if got[1] != "…iltin/soft-circle.png" {
		t.Errorf("cut label = %q", got[1])
	}
	if got[2] != "a/very/long/folder/names/spark.png" || got[3] != "b/very/long/folder/names/spark.png" {
		t.Errorf("colliding labels must keep full keys: %q %q", got[2], got[3])
	}
}
