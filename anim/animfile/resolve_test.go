package animfile

import (
	"slices"
	"strings"
	"testing"
	"time"
)

// heroPlay is the hero sheet of sheet_test.go as Resolve input.
func heroPlay(t *testing.T) []Tag {
	t.Helper()
	s, err := DecodeSheet([]byte(sheetHash(6, heroDurs, heroTags)))
	if err != nil {
		t.Fatal(err)
	}
	return s.PlayOrder()
}

// resolve decodes a definition of the hero sheet built from the given
// top-level members (without braces) and resolves it.
func resolve(t *testing.T, members string) (*Resolved, error) {
	t.Helper()
	src := `{"version":1,"sheet":"hero.json"`
	if members != "" {
		src += "," + members
	}
	f, err := Decode([]byte(src + "}"))
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	return Resolve(f, heroPlay(t))
}

func mustResolve(t *testing.T, members string) *Resolved {
	t.Helper()
	r, err := resolve(t, members)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	return r
}

func (r *Resolved) clip(t *testing.T, name string) *ResolvedClip {
	t.Helper()
	for i := range r.Clips {
		if r.Clips[i].Name == name {
			return &r.Clips[i]
		}
	}
	t.Fatalf("no clip %q", name)
	return nil
}

func TestResolveOrderAndOmitted(t *testing.T) {
	r := mustResolve(t, `"defaultClip":"run","clips":{"bounce":{},"idle":{"loop":false}}`)
	var names []string
	for _, c := range r.Clips {
		names = append(names, c.Name)
	}
	if want := []string{"idle", "run", "bounce", "bounce_rev"}; !slices.Equal(names, want) {
		t.Fatalf("clips = %v, want tag order %v", names, want)
	}
	if want := []string{"run", "bounce_rev"}; !slices.Equal(r.Omitted, want) {
		t.Fatalf("Omitted = %v, want %v", r.Omitted, want)
	}
	if r.Default != "run" || !r.clip(t, "idle").Listed || r.clip(t, "run").Listed {
		t.Fatalf("Default %q, listed flags wrong", r.Default)
	}
	// Unlisted clips keep the sheet's loop mode and play-order timing.
	if run := r.clip(t, "run"); run.Loop != Loop || !slices.Equal(run.Durations, durs(80, 70, 60, 50)) {
		t.Fatalf("run = %+v", run)
	}
}

func TestResolveLoop(t *testing.T) {
	tests := []struct {
		name string
		clip string // idle loops, bounce ping-pongs in the sheet
		loop string // JSON value, "" for absent
		want LoopMode
	}{
		{"absent keeps loop", "idle", "", Loop},
		{"absent keeps pingpong", "bounce", "", PingPong},
		{"false is once", "idle", "false", Once},
		{"false overrides pingpong", "bounce", "false", Once},
		{"true keeps loop", "idle", "true", Loop},
		{"true keeps pingpong", "bounce", "true", PingPong},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			def := "{}"
			if tt.loop != "" {
				def = `{"loop":` + tt.loop + `}`
			}
			r := mustResolve(t, `"clips":{"`+tt.clip+`":`+def+`}`)
			if got := r.clip(t, tt.clip).Loop; got != tt.want {
				t.Fatalf("Loop = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestResolveTiming(t *testing.T) {
	// idle has two frames of 100 and 200 ms in the sheet.
	tests := []struct {
		name string
		def  string
		want []time.Duration
	}{
		{"none keeps the sheet", `{}`, durs(100, 200)},
		{"durations", `{"durations":[30,40]}`, durs(30, 40)},
		{"fps", `{"fps":8}`, durs(125, 125)},
		{"fps rounds to whole ms", `{"fps":12}`, durs(83, 83)},
		{"fps is at least 1 ms", `{"fps":5000}`, durs(1, 1)},
		{"durations beat fps", `{"fps":8,"durations":[30,40]}`, durs(30, 40)},
		{"speed scales the sheet", `{"speed":2}`, durs(50, 100)},
		{"speed scales fps", `{"fps":10,"speed":0.5}`, durs(200, 200)},
		{"speed scales durations", `{"durations":[30,60],"speed":3}`, durs(10, 20)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := mustResolve(t, `"clips":{"idle":`+tt.def+`}`)
			if got := r.clip(t, "idle").Durations; !slices.Equal(got, tt.want) {
				t.Fatalf("durations = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestResolveDoesNotModifyTags(t *testing.T) {
	tags := heroPlay(t)
	f, err := Decode([]byte(`{"version":1,"sheet":"hero.json","clips":{"idle":{"fps":8}}}`))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Resolve(f, tags); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(tags[0].Durations, durs(100, 200)) {
		t.Fatalf("Resolve changed its input: %v", tags[0].Durations)
	}
}

func TestResolvePivot(t *testing.T) {
	r := mustResolve(t, `"pivot":{"x":2,"y":4},"clips":{"run":{"pivot":{"x":1,"y":1}}}`)
	if p := r.clip(t, "idle").Pivot; p == nil || *p != (Point{X: 2, Y: 4}) {
		t.Errorf("idle pivot = %v, want the top-level (2,4)", p)
	}
	if p := r.clip(t, "run").Pivot; p == nil || *p != (Point{X: 1, Y: 1}) {
		t.Errorf("run pivot = %v, want its own (1,1)", p)
	}
	if p := mustResolve(t, "").clip(t, "idle").Pivot; p != nil {
		t.Errorf("no pivot anywhere: got %v", p)
	}
}

func TestResolveBoxes(t *testing.T) {
	body := Box{Kind: BoxHurt, ID: BodyBoxID, Rect: Rect{X: -2, Y: -4, W: 4, H: 4}}
	duck := Box{Kind: BoxHurt, ID: "duck", Rect: Rect{X: -2, Y: -2, W: 4, H: 2}}
	slash := Box{Kind: BoxHit, ID: "slash", Rect: Rect{X: 1, Y: -3, W: 5, H: 2}}
	glow := Box{Kind: BoxHit, Rect: Rect{X: 0, Y: 0, W: 1, H: 1}}
	r := mustResolve(t, `
		"body":[{"x":-2,"y":-4,"w":4,"h":4}],
		"clips":{
			"run":{"shapes":[
				{"kind":"hit","id":"slash","frames":{"from":1,"to":2},"rect":{"x":1,"y":-3,"w":5,"h":2}},
				{"kind":"hurt","id":"duck","frames":{"from":2},"rect":{"x":-2,"y":-2,"w":4,"h":2}}
			]},
			"bounce":{"shapes":[{"kind":"hit","rect":{"x":0,"y":0,"w":1,"h":1}}]}
		}`)
	tests := []struct {
		clip  string
		boxes [][]Box
	}{
		{"idle", [][]Box{{body}, {body}}},
		{"run", [][]Box{{body}, {body, slash}, {slash, duck}, {body}}},
		{"bounce", [][]Box{{body, glow}, {body, glow}, {body, glow}}},
	}
	for _, tt := range tests {
		t.Run(tt.clip, func(t *testing.T) {
			got := r.clip(t, tt.clip).Boxes
			if len(got) != len(tt.boxes) {
				t.Fatalf("%d frames of boxes, want %d", len(got), len(tt.boxes))
			}
			for i, want := range tt.boxes {
				if !slices.Equal(got[i], want) {
					t.Errorf("frame %d: boxes = %v, want %v", i, got[i], want)
				}
				if cap(got[i]) != len(got[i]) {
					t.Errorf("frame %d: cap %d > len %d", i, cap(got[i]), len(got[i]))
				}
			}
		})
	}
	if b := mustResolve(t, "").clip(t, "idle").Boxes; b != nil {
		t.Fatalf("no body, no shapes: Boxes = %v, want nil", b)
	}
}

func TestResolveErrors(t *testing.T) {
	tests := []struct {
		name    string
		members string
		want    string
	}{
		{"unknown clip", `"clips":{"jump":{}}`, `clip "jump": no such tag in sheet "hero.json"`},
		{"duplicate clip", `"clips":{"idle":{},"idle":{}}`, `clip "idle": listed twice`},
		{"unknown defaultClip", `"defaultClip":"jump"`, `"defaultClip" "jump": no such tag`},
		{"frame out of range", `"clips":{"idle":{"shapes":[{"kind":"hit","frames":{"from":1,"to":2},"rect":{"x":0,"y":0,"w":1,"h":1}}]}}`, `clip "idle": shape 0: frames 1..2 are outside the clip's 2 frames`},
		{"negative frame", `"clips":{"idle":{"shapes":[{"kind":"hit","frames":{"from":-1},"rect":{"x":0,"y":0,"w":1,"h":1}}]}}`, "frames -1..-1"},
		{"empty shape rect", `"clips":{"idle":{"shapes":[{"kind":"hit","rect":{"x":0,"y":0,"w":0,"h":1}}]}}`, "w and h must be positive, got w=0 h=1"},
		{"empty body rect", `"body":[{"x":0,"y":0,"w":3,"h":-1}]`, "body 0: rect w and h must be positive"},
		{"bad kind", `"clips":{"idle":{"shapes":[{"kind":"hitt","rect":{"x":0,"y":0,"w":1,"h":1}}]}}`, `"kind" is "hitt"`},
		{"durations length", `"clips":{"idle":{"durations":[100]}}`, `clip "idle": "durations" has 1 values for 2 frames`},
		{"zero duration", `"clips":{"idle":{"durations":[100,0]}}`, `"durations"[1] is 0`},
		{"zero fps", `"clips":{"idle":{"fps":0}}`, `"fps" is 0, want > 0`},
		{"zero speed", `"clips":{"idle":{"speed":0}}`, `"speed" is 0, want > 0`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := resolve(t, tt.members)
			mustContain(t, err, tt.want)
		})
	}
}

func TestResolveReportsEveryProblem(t *testing.T) {
	_, err := resolve(t, `
		"defaultClip":"nope",
		"body":[{"x":0,"y":0,"w":0,"h":0}],
		"clips":{
			"jump":{},
			"idle":{"fps":-1,"shapes":[{"kind":"x","frames":{"from":9},"rect":{"x":0,"y":0,"w":1,"h":1}}]},
			"run":{"durations":[1,2]}
		}`)
	want := []string{
		`body 0:`,
		`clip "jump": no such tag`,
		`"defaultClip" "nope"`,
		`clip "idle": "fps" is -1`,
		`clip "idle": shape 0: "kind" is "x"`,
		`clip "idle": shape 0: frames 9..9 are outside`,
		`clip "run": "durations" has 2 values for 4 frames`,
	}
	lines := strings.Split(err.Error(), "\n")
	if len(lines) != len(want) {
		t.Fatalf("error has %d lines, want %d:\n%v", len(lines), len(want), err)
	}
	for i, w := range want {
		if !strings.HasPrefix(lines[i], w) {
			t.Errorf("line %d = %q, want prefix %q", i, lines[i], w)
		}
	}
}
