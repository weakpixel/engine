package input

import (
	"strings"
	"testing"
)

func TestControlRoundTrip(t *testing.T) {
	for _, s := range []string{
		"key:Space", "key:ArrowUp", "key:Numpad0", "mod:ctrl", "mouse:left", "mouse:forward",
		"mouse:wheel", "mouse:wheelY-", "mouse:delta", "mouse:deltaX",
		"pad:south", "pad:rt", "pad:home", "pad:leftstick", "pad:rightstickY+", "pad:leftstickX",
		"padraw:b12", "padraw:a3", "padraw:a3-", "touch:any",
	} {
		c, err := ParseControl(s, nil)
		if err != nil {
			t.Fatalf("%s: %v", s, err)
		}
		if got := c.String(); got != s {
			t.Errorf("round trip %s = %s", s, got)
		}
	}
	for _, s := range []string{"key:Spcae", "pad:a", "mouse:wheelZ", "padraw:b64", "padraw:a8", "touch:jump", "nope", "key:"} {
		if _, err := ParseControl(s, nil); err == nil {
			t.Errorf("%s: no error", s)
		}
	}
}

func TestParseBindingsReportsAllErrors(t *testing.T) {
	ta := newTestActions()
	src := `{
	  "version": 2,
	  "groups": {"g": ["nope"]},
	  "mappings": {
	    "m": {
	      "block": "yes",
	      "bindings": {
	        "jump": [{"control": "pad:soth"}, {"control": "pad:leftstick"}, {"composite": "dpad", "up": "key:W"}],
	        "mvoe": [{"control": "key:W"}],
	        "move": [{"control": "key:W"}, {"composite": "dpad"}, {"control": "pad:leftstick", "processors": [{"deadzone": 1.5}, {"invert": "z"}, {}]}],
	        "throttle": [{"control": "pad:rt", "interaction": {"hold": "1s"}}],
	        "charge": [{"control": "key:C", "interaction": {"hold": "soon"}, "threshold": 2}],
	        "attack": [{"control": "touch:nowhere", "mods": ["mod:hyper"]}]
	      },
	      "touch": {"t": {"kind": "wheel", "area": [0.5, 0, 0.2, 1]}},
	      "extra": 1
	    }
	  }
	}`
	_, err := ParseBindings([]byte(src), ta.acts)
	if err == nil {
		t.Fatal("no error")
	}
	// Shape errors come first and stop before semantics.
	for _, want := range []string{
		`$.mappings.m.block: expected boolean, got string`,
		`$.mappings.m.extra: unknown property "extra"`,
	} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("missing %q in:\n%v", want, err)
		}
	}

	src = strings.Replace(strings.Replace(src, `"block": "yes",`, "", 1), `,
	      "extra": 1`, "", 1)
	_, err = ParseBindings([]byte(src), ta.acts)
	if err == nil {
		t.Fatal("no error")
	}
	for _, want := range []string{
		`$.version: unsupported version 2, want 1`,
		`$.groups.g[0]: unknown mapping "nope"`,
		`$.mappings.m.bindings.jump[0].control: unknown control "pad:soth"`,
		`$.mappings.m.bindings.jump[1].control: control "pad:leftstick" has 2 dimension(s), Button action "jump" needs 1`,
		`$.mappings.m.bindings.jump[2]: composite "dpad" produces Vec2, but action "jump" is Button`,
		`$.mappings.m.bindings.mvoe: unknown action "mvoe"`,
		`$.mappings.m.bindings.move[0].control: control "key:W" has 1 dimension(s), Vec2 action "move" needs 2`,
		`$.mappings.m.bindings.move[1]: composite "dpad" has no parts`,
		`$.mappings.m.bindings.move[2].processors[0].deadzone: must be in [0, 1), got 1.5`,
		`$.mappings.m.bindings.move[2].processors[1].invert: unknown value "z"`,
		`$.mappings.m.bindings.move[2].processors[2]: a processor needs exactly one`,
		`$.mappings.m.bindings.throttle[0].interaction: Axis actions only take press or repeat`,
		`$.mappings.m.bindings.charge[0].interaction.hold: invalid duration "soon"`,
		`$.mappings.m.bindings.charge[0].threshold: must be in [0, 1], got 2`,
		`$.mappings.m.bindings.attack[0].control: unknown touch region "nowhere"`,
		`$.mappings.m.bindings.attack[0].mods[0]: unknown modifier "mod:hyper"`,
		`$.mappings.m.touch.t.kind: unknown kind "wheel"`,
		`$.mappings.m.touch.t.area: want 0 <= x0 < x1 <= 1`,
	} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("missing %q in:\n%v", want, err)
		}
	}
}

func TestParseBindingsTestdata(t *testing.T) {
	ta := newTestActions()
	b := loadTestBindings(t, ta.acts)
	if got := strings.Join(b.MappingNames(), ","); got != "gameplay,menu,vehicle" {
		t.Fatalf("mappings = %s", got)
	}
	if !b.HasMapping("menu") || b.HasMapping("nope") {
		t.Fatal("HasMapping")
	}
	// Ctrl+S sorts before the plain S of the move dpad.
	m := b.mappings[b.byName["gameplay"]]
	if b.binds[m.lo].action != ta.save {
		t.Fatalf("first gameplay binding is %s, want save", ta.acts.Name(b.binds[m.lo].action))
	}
}

func TestActionsPanics(t *testing.T) {
	var a Actions
	a.Button("x", "")
	mustPanic(t, `input: duplicate action "x"`, func() { a.Button("x", "") })
	mustPanic(t, "input: empty action name", func() { a.Axis("", "") })
	mustPanic(t, "Delta is only valid for Vec2", func() { a.Button("y", "", Delta()) })
	mustPanic(t, "input: ParseBindings: Actions not frozen", func() { _, _ = ParseBindings([]byte(`{}`), &a) })
	a.Freeze()
	mustPanic(t, `input: action "z" declared after Freeze`, func() { a.Button("z", "") })
	mustPanic(t, "input: unknown action 9", func() { a.Name(9) })
}

func TestParseProfile(t *testing.T) {
	p, err := ParseProfile([]byte(`{"version":1,"settings":{"invertY":true},"overrides":[
	  {"mapping":"gameplay","action":"jump","class":"kbm","slot":0,"binding":{"control":"key:K"}}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if !p.Settings.InvertY || len(p.Overrides) != 1 {
		t.Fatalf("profile = %+v", p)
	}
	_, err = ParseProfile([]byte(`{"version":1,"overrides":[
	  {"mapping":"m","action":"a","class":"wii","slot":-1},
	  {"mapping":"m","action":"a","class":"kbm","slot":0,"control":"key:K"},
	  {"mapping":"m","action":"a","class":"kbm","slot":0,"part":"up","binding":{"control":"key:K"}}]}`))
	for _, want := range []string{
		`$.overrides[0].class: unknown class "wii"`,
		`$.overrides[0].slot: must not be negative`,
		`$.overrides[0]: needs exactly one of binding, control, unbind`,
		`$.overrides[1].control: only valid with part`,
		`$.overrides[2].binding: not valid with part`,
	} {
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("missing %q in:\n%v", want, err)
		}
	}
}
