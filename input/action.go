package input

import "fmt"

// Action identifies one declared action. IDs are dense and start at 1; the
// zero Action means "none" and reads as zero everywhere.
type Action uint16

// Kind says what value an action carries.
type Kind uint8

// Action kinds.
const (
	// Button is digital: down or up, with edges and held ticks. Analog
	// controls press it when they cross the binding's threshold.
	Button Kind = iota + 1
	// Axis is one float: [-1, 1] from an axis composite or a stick half,
	// [0, 1] from a trigger.
	Axis
	// Vec2 is two floats: a stick, a dpad composite, the mouse delta.
	Vec2
)

// String returns "Button", "Axis" or "Vec2".
func (k Kind) String() string {
	switch k {
	case Button:
		return "Button"
	case Axis:
		return "Axis"
	case Vec2:
		return "Vec2"
	}
	return "invalid"
}

// ActionOption changes how an action is declared.
type ActionOption func(*actionInfo)

// Delta marks a Vec2 action whose value is a per-tick delta (mouse look,
// touch drag). Its length may exceed 1, and the profile's sensitivity
// setting scales it.
func Delta() ActionOption { return func(a *actionInfo) { a.delta = true } }

// AllowMods lets rebinding capture modifier+key combinations (Ctrl+S) for
// the action. Without it a captured key is bound without modifiers.
func AllowMods() ActionOption { return func(a *actionInfo) { a.allowMods = true } }

type actionInfo struct {
	name      string
	doc       string
	kind      Kind
	delta     bool
	allowMods bool
}

// Actions is the game's action table. Declare every action at startup
// (package-level vars are the usual place), then [Actions.Freeze] it before
// building a [Router] or parsing bindings. The zero value is ready to use.
//
// Declaring panics with "input: " for an empty or duplicate name and after
// Freeze. Actions is not safe for concurrent declaration; once frozen it is
// read-only and safe to share (asset workers read it while decoding).
type Actions struct {
	list   []actionInfo // index = Action-1
	byName map[string]Action
	frozen bool
}

// Button declares a Button action and returns its ID. doc is shown in
// rebinding menus and generated docs.
func (a *Actions) Button(name, doc string, opts ...ActionOption) Action {
	return a.declare(name, doc, Button, opts)
}

// Axis declares an Axis action.
func (a *Actions) Axis(name, doc string, opts ...ActionOption) Action {
	return a.declare(name, doc, Axis, opts)
}

// Vec2 declares a Vec2 action.
func (a *Actions) Vec2(name, doc string, opts ...ActionOption) Action {
	return a.declare(name, doc, Vec2, opts)
}

func (a *Actions) declare(name, doc string, k Kind, opts []ActionOption) Action {
	if a.frozen {
		panicf("action %q declared after Freeze", name)
	}
	if name == "" {
		panicf("empty action name")
	}
	if _, dup := a.byName[name]; dup {
		panicf("duplicate action %q", name)
	}
	if len(a.list) >= 0xFFFF {
		panicf("too many actions")
	}
	info := actionInfo{name: name, doc: doc, kind: k}
	for _, o := range opts {
		o(&info)
	}
	if info.delta && k != Vec2 {
		panicf("action %q: Delta is only valid for Vec2 actions", name)
	}
	if a.byName == nil {
		a.byName = map[string]Action{}
	}
	a.list = append(a.list, info)
	id := Action(len(a.list))
	a.byName[name] = id
	return id
}

// Freeze ends declaration. It is idempotent.
func (a *Actions) Freeze() { a.frozen = true }

// Frozen reports whether Freeze was called.
func (a *Actions) Frozen() bool { return a.frozen }

// Lookup returns the action declared under name.
func (a *Actions) Lookup(name string) (Action, bool) {
	id, ok := a.byName[name]
	return id, ok
}

// Len returns the number of declared actions. Valid IDs are 1..Len.
func (a *Actions) Len() int { return len(a.list) }

// Name returns the action's name. Panics with "input: unknown action" for an
// ID that was not declared.
func (a *Actions) Name(id Action) string { return a.info(id).name }

// Doc returns the action's doc string.
func (a *Actions) Doc(id Action) string { return a.info(id).doc }

// Kind returns the action's kind.
func (a *Actions) Kind(id Action) Kind { return a.info(id).kind }

// IsDelta reports whether the action was declared with [Delta].
func (a *Actions) IsDelta(id Action) bool { return a.info(id).delta }

// AllowsMods reports whether the action was declared with [AllowMods].
func (a *Actions) AllowsMods(id Action) bool { return a.info(id).allowMods }

func (a *Actions) info(id Action) *actionInfo {
	if id == 0 || int(id) > len(a.list) {
		panicf("unknown action %d", id)
	}
	return &a.list[id-1]
}

func (a *Actions) mustBeFrozen(who string) {
	if a == nil {
		panicf("%s: nil Actions", who)
	}
	if !a.frozen {
		panicf("%s: Actions not frozen", who)
	}
}

func panicf(format string, args ...any) {
	panic("input: " + fmt.Sprintf(format, args...))
}
