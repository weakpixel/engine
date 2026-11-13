package tiled

import "testing"

func TestDecodeRealWorld(t *testing.T) {
	w, err := DecodeWorld(readTestdata(t, "world-1.world"))
	if err != nil {
		t.Fatal(err)
	}
	want := []WorldMap{
		{FileName: "level-1.tmj", X: -336, Y: 160, Width: 640, Height: 640},
		{FileName: "level-2.tmj", X: 304, Y: 160, Width: 640, Height: 640},
	}
	if len(w.Maps) != 2 || w.Maps[0] != want[0] || w.Maps[1] != want[1] {
		t.Fatalf("maps = %+v", w.Maps)
	}
}

func TestDecodeWorldErrors(t *testing.T) {
	for _, tc := range []struct {
		name string
		json string
		want []string
	}{
		{"json", `{`, []string{"world: invalid JSON"}},
		{"patterns", `{"patterns":[{"regexp":"p(\\d+)\\.tmj","multiplierX":640}]}`, []string{"unsupported Tiled feature: world patterns"}},
		{"empty", `{"maps":[]}`, []string{"world lists no maps"}},
		{"all at once", `{"maps":[{"fileName":""},{"fileName":"a.tmx"},{"fileName":"b.tmj"},{"fileName":"./b.tmj"}]}`,
			[]string{"map 0: fileName is empty", `map 1: "a.tmx" is not a .tmj map`, `map 3: "./b.tmj" is listed twice`}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := DecodeWorld([]byte(tc.json))
			mustContain(t, err, tc.want...)
		})
	}
}
