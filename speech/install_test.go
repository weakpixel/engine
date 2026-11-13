package speech

import (
	"testing"
	"testing/fstest"
	"time"

	"github.com/weakpixel/voice"
	"github.com/weakpixel/voice/voicetest"

	"github.com/weakpixel/ecs"
	"github.com/weakpixel/engine/assetkind"
	"github.com/weakpixel/engine/game"
	"github.com/weakpixel/engine/gfx"
	"github.com/weakpixel/engine/resolver"
)

// talkerLevel is a GOD level with one speech:Talker of voice "chirpy" and
// one of the bard voice file.
const talkerLevel = `{"objects":[
 {"group":"speech","class":"Talker","components":{
  "gfx:Transform":{"Pos":{"X":0,"Y":0},"PrevPos":{"X":0,"Y":0},"Rot":0,"Scale":{"X":1,"Y":1}},
  "gfx:Label":{"Text":"","Layer":0,"Color":0},
  "speech:Speaker":{"Voice":"chirpy","Speed":1,"Pan":0}}},
 {"group":"speech","class":"Talker","components":{
  "gfx:Transform":{"Pos":{"X":0,"Y":0},"PrevPos":{"X":0,"Y":0},"Rot":0,"Scale":{"X":1,"Y":1}},
  "gfx:Label":{"Text":"","Layer":0,"Color":0},
  "speech:Speaker":{"Voice":"talk/bard.voice.json","Speed":1,"Pan":0.5}}}]}`

func newEngine(t *testing.T, cfg Config) (*game.Engine, *Speech) {
	t.Helper()
	fsys := speechFiles()
	fsys["lvl.level.json"] = &fstest.MapFile{Data: []byte(talkerLevel)}
	fsys["box.level.json"] = &fstest.MapFile{Data: []byte(boxLevel)}
	e := game.New(game.Config{NoAudio: true, Width: 32, Height: 32, FS: fsys, Input: game.InputConfig{HideTouchControls: true}})
	t.Cleanup(e.Close)
	return e, Install(e, cfg)
}

func update(t *testing.T, e *game.Engine, n int) {
	t.Helper()
	for range n {
		if err := e.Update(); err != nil {
			t.Fatal(err)
		}
	}
}

// talkers returns the level's Talker entities in level order.
func talkers(t *testing.T, e *game.Engine) []ecs.Entity {
	t.Helper()
	if _, err := e.LoadLevel("lvl.level.json"); err != nil {
		t.Fatal(err)
	}
	update(t, e, 5)
	var out []ecs.Entity
	q := ecs.NewView2[Speaker, gfx.Label](e.World())
	for en := range q.Each() {
		out = append(out, en)
	}
	if len(out) != 2 {
		t.Fatalf("%d talkers, want 2", len(out))
	}
	voices := VoiceStoreOf(e.Assets())
	for _, en := range out {
		h := ecs.GetComp[Speaker](e.World(), en).Voice
		if h.IsZero() {
			t.Fatal("speech:Speaker.Voice not resolved")
		}
		for i := 0; !voices.Ready(h); i++ {
			if i == 1000 {
				t.Fatalf("voice %v never ready: %v", voices.Key(h), voices.Err(h))
			}
			time.Sleep(time.Millisecond)
			update(t, e, 1)
		}
	}
	return out
}

// TestAssetKinds checks that linking speech registers its two asset kinds.
func TestAssetKinds(t *testing.T) {
	for _, kind := range []string{AssetKindVoice, AssetKindDialogue} {
		if _, ok := assetkind.Lookup(kind); !ok {
			t.Errorf("kind %q is not registered", kind)
		}
	}
}

func TestLevelTalker(t *testing.T) {
	e, s := newEngine(t, Config{})
	es := talkers(t, e)
	w := e.World()
	if ecs.GetResource[Speech](w) != s {
		t.Error("Speech is not the World resource")
	}
	res := resolver.New(e.Assets())
	for i, want := range []string{"chirpy", "talk/bard.voice.json"} {
		key, err := res.AssetKey(AssetKindVoice, ecs.GetComp[Speaker](w, es[i]).Voice)
		if err != nil || key != want {
			t.Errorf("AssetKey = %q, %v, want %q", key, err, want)
		}
		if v, st := VoiceStoreOf(e.Assets()).Lookup(ecs.GetComp[Speaker](w, es[i]).Voice); v.Silent {
			t.Errorf("voice %q did not load: %v %v", want, st, VoiceStoreOf(e.Assets()).Err(ecs.GetComp[Speaker](w, es[i]).Voice))
		}
	}
	s.Say(es[0], hello, SayOptions{})
	s.Say(es[1], "Ae tha.", SayOptions{})
	for range 60 * 30 {
		if !s.Speaking(es[0]) && !s.Speaking(es[1]) {
			break
		}
		update(t, e, 1)
	}
	if a, b := ecs.GetComp[gfx.Label](w, es[0]).Text, ecs.GetComp[gfx.Label](w, es[1]).Text; a != hello || b != "Ae tha." {
		t.Errorf("labels %q, %q", a, b)
	}
}

// TestReadingGuardDropsNotes checks the reading guard: under NoAudio nobody reads
// the synthesizer, so the notes of a line are dropped every tick and do not
// play together once reading starts; the patches survive.
func TestReadingGuardDropsNotes(t *testing.T) {
	e, s := newEngine(t, Config{})
	es := talkers(t, e)
	if s.synth == nil || s.stream.Reading() {
		t.Fatal("no synthesizer, or a stream that is read under NoAudio")
	}
	s.Say(es[0], hello, SayOptions{})
	update(t, e, 30)
	if s.Revealed(es[0]) == 0 {
		t.Fatal("the line does not reveal under NoAudio")
	}
	left, right := make([]float32, 4800), make([]float32, 4800)
	s.synth.Render(left, right)
	if p := peak(left, right); p != 0 {
		t.Fatalf("the first read after a stall plays old notes (peak %v)", p)
	}
	// The owner is still patched: a new note sounds.
	s.synth.NoteOn(s.byEnt[es[0]].owner, voice.InstVowel, 60, 1, 0)
	s.synth.Render(left, right)
	if peak(left, right) == 0 {
		t.Error("the patch of the owner was dropped too")
	}
}

func peak(left, right []float32) float32 {
	var p float32
	for i := range left {
		p = max(p, left[i], -left[i], right[i], -right[i])
	}
	return p
}

// TestPauseSceneHoldsSpeech checks that a pause scene above the root holds
// the lines and releases their notes, and that they continue after the pop.
func TestPauseSceneHoldsSpeech(t *testing.T) {
	sink := voicetest.NewRecordingSink(4096)
	e, s := newEngine(t, Config{Sink: sink})
	es := talkers(t, e)
	s.Say(es[0], hello, SayOptions{})
	update(t, e, 10)
	e.Push(game.NewScene("pause", game.SceneOptions{}))
	update(t, e, 2)
	n := s.Revealed(es[0])
	update(t, e, 30)
	if s.Revealed(es[0]) != n || !s.Speaking(es[0]) {
		t.Fatalf("the line moved under the pause scene: %d to %d", n, s.Revealed(es[0]))
	}
	if on := sink.Sounding(s.byEnt[es[0]].owner); len(on) > 0 {
		t.Fatalf("%d notes hang under the pause scene", len(on))
	}
	e.Pop()
	for range 60 * 30 {
		if !s.Speaking(es[0]) {
			break
		}
		update(t, e, 1)
	}
	if got := ecs.GetComp[gfx.Label](e.World(), es[0]).Text; got != hello {
		t.Errorf("label %q after the pop", got)
	}
	s.Say(es[0], hello, SayOptions{})
	update(t, e, 10)
	owner := s.byEnt[es[0]].owner
	s.Close()
	if on := sink.Sounding(owner); s.Speaking(es[0]) || len(on) > 0 {
		t.Error("Close kept a line")
	}
}

// boxLevel is a GOD level with a speech:Box that starts by itself and a
// gfx:Text for the speaker's name.
const boxLevel = `{"objects":[
 {"id":"name","group":"gfx","class":"Text","components":{
  "gfx:Transform":{"Pos":{"X":0,"Y":0},"PrevPos":{"X":0,"Y":0},"Rot":0,"Scale":{"X":1,"Y":1}},
  "gfx:Label":{"Text":"","Layer":0,"Color":0}}},
 {"group":"speech","class":"Box","components":{
  "gfx:Transform":{"Pos":{"X":0,"Y":0},"PrevPos":{"X":0,"Y":0},"Rot":0,"Scale":{"X":1,"Y":1}},
  "gfx:Label":{"Text":"","Layer":0,"Color":0},
  "speech:DialogueBox":{"Dialogue":"talk/town.dialogue.json","Name":"name","AutoStart":true,"AutoAdvance":false}}}]}`

// TestLevelBoxPauseScene checks that a level's box starts by itself, that
// a pause scene freezes it mid-line with no note hanging, and that it
// continues after the pop.
func TestLevelBoxPauseScene(t *testing.T) {
	sink := voicetest.NewRecordingSink(4096)
	e, s := newEngine(t, Config{Sink: sink})
	if _, err := e.LoadLevel("box.level.json"); err != nil {
		t.Fatal(err)
	}
	var b ecs.Entity
	for i := 0; ; i++ {
		if i == 1000 {
			t.Fatal("the box never started")
		}
		update(t, e, 1)
		for en := range ecs.NewView1[DialogueBox](e.World()).Each() {
			b = en
		}
		if _, _, ok := s.Line(b); ok && s.byBox[b].p.Revealed() > 0 {
			break
		}
		time.Sleep(time.Millisecond)
	}
	name := ecs.GetComp[DialogueBox](e.World(), b).Name
	if got := ecs.GetComp[gfx.Label](e.World(), name).Text; got != "bard" {
		t.Errorf("name label %q", got)
	}
	e.Push(game.NewScene("pause", game.SceneOptions{}))
	update(t, e, 2)
	text := ecs.GetComp[gfx.Label](e.World(), b).Text
	update(t, e, 30)
	if got := ecs.GetComp[gfx.Label](e.World(), b).Text; got != text || !s.Playing(b) {
		t.Fatalf("the box moved under the pause scene: %q to %q", text, got)
	}
	for _, id := range s.byBox[b].sink.owners {
		if on := sink.Sounding(id); len(on) > 0 {
			t.Fatalf("%d notes hang under the pause scene", len(on))
		}
	}
	e.Pop()
	update(t, e, 60*5)
	if got := ecs.GetComp[gfx.Label](e.World(), b).Text; got != "Ae tha." {
		t.Errorf("label %q after the pop", got)
	}
}
