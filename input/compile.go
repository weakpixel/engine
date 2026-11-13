package input

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math/bits"
	"reflect"
	"slices"
	"strconv"
	"time"
)

// SlotClass groups bindings for rebinding menus: each action has numbered
// slots per class (keyboard and mouse primary and secondary, gamepad).
type SlotClass uint8

// Slot classes.
const (
	SlotKBM SlotClass = iota + 1 // keyboard and mouse
	SlotGamepad
	SlotTouch
)

var slotClassNames = [...]string{"", "kbm", "gamepad", "touch"}

// String returns "kbm", "gamepad" or "touch".
func (c SlotClass) String() string {
	if c > 0 && int(c) < len(slotClassNames) {
		return slotClassNames[c]
	}
	return "invalid"
}

// ParseSlotClass is the inverse of [SlotClass.String].
func ParseSlotClass(s string) (SlotClass, bool) {
	for i, n := range slotClassNames {
		if i > 0 && n == s {
			return SlotClass(i), true
		}
	}
	return 0, false
}

// SlotClassOf returns the slot class a device class binds through: keyboard
// and mouse share kbm. Use it to rebind for the player's active device:
// SlotClassOf(p.ActiveDevice().Class).
func SlotClassOf(c DeviceClass) SlotClass {
	switch c {
	case Keyboard, Mouse:
		return SlotKBM
	case Gamepad:
		return SlotGamepad
	case Touch:
		return SlotTouch
	}
	return 0
}

// Bindings are compiled bindings: the default bindings of a game (from
// [ParseBindings]) or a player's effective bindings (defaults plus profile
// overrides). They are immutable once built and safe to share between
// players and goroutines.
type Bindings struct {
	acts     *Actions
	file     BindingsFile
	mappings []mapping // sorted by name
	byName   map[string]int
	binds    []binding // grouped by mapping, see mapping.lo/hi
	regions  []region
}

type mapping struct {
	name              string
	priority          int
	block, strictMods bool
	// related are the mappings (indices, self included) that share a group
	// with this one: the scope of rebinding conflicts.
	related []int
	lo, hi  int   // binds[lo:hi]
	regions []int // touch regions, sorted by name
}

type bindKind uint8

const (
	bindSingle bindKind = iota
	bindAxis            // ctrl[0] neg, ctrl[1] pos
	bindDpad            // ctrl[0..3] up, down, left, right
	bindChord           // ctrl[:n]
)

// Composite part names, in ctrl order.
var (
	axisParts = []string{"neg", "pos"}
	dpadParts = []string{"up", "down", "left", "right"}
)

type binding struct {
	action    Action
	kind      bindKind
	ctrl      [4]Control
	n         uint8 // used entries of ctrl
	mods      uint8 // bit per Mod
	lastWins  bool
	procs     []processor
	inter     interaction
	threshold float32
	locked    bool
	unbound   bool // removed by a profile override; keeps its slot
	mapping   int
	class     SlotClass
	slot      int
	spec      BindingSpec
}

type procOp uint8

const (
	procDeadzone procOp = iota + 1
	procInvert
	procScale
	procNormalize
	procSensitivity
	procCurve
)

type processor struct {
	op   procOp
	a, b float32
}

type interOp uint8

const (
	interPress interOp = iota
	interRelease
	interHold
	interTap
	interDoubleTap
	interRepeat
)

type interaction struct {
	op     interOp
	d1, d2 time.Duration
}

type regionKind uint8

const (
	regionButton regionKind = iota
	regionStick
	regionDrag
)

type region struct {
	name     string
	mapping  int
	kind     regionKind
	circle   bool
	rect     [4]float32 // normalized area, or circle cx, cy, r
	radius   float32
	floating bool
}

// NewBindings returns empty bindings for acts: every action reads zero. It
// is the placeholder while the bindings file loads. Panics with "input: " if
// acts is nil or not frozen.
func NewBindings(acts *Actions) *Bindings {
	acts.mustBeFrozen("NewBindings")
	return &Bindings{acts: acts, byName: map[string]int{}}
}

// ParseBindings decodes, validates and compiles a bindings file against
// acts. It reports every problem at once ([errors.Join]), each prefixed with
// its path in the file:
//
//	$.mappings.gameplay.bindings.jump[1].control: unknown control "pad:soth"
//
// Panics with "input: " if acts is nil or not frozen.
func ParseBindings(data []byte, acts *Actions) (*Bindings, error) {
	acts.mustBeFrozen("ParseBindings")
	var errs []error
	checkJSON(data, reflect.TypeFor[BindingsFile](), "$", &errs)
	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}
	var f BindingsFile
	dec := json.NewDecoder(bytes.NewReader(data))
	if err := dec.Decode(&f); err != nil {
		return nil, fmt.Errorf("$: %w", err)
	}
	return CompileBindings(f, acts)
}

// CompileBindings validates and compiles a decoded bindings file, like
// [ParseBindings].
func CompileBindings(f BindingsFile, acts *Actions) (*Bindings, error) {
	acts.mustBeFrozen("CompileBindings")
	var errs []error
	if f.Version != 1 {
		errs = append(errs, fmt.Errorf("$.version: unsupported version %d, want 1", f.Version))
	}
	b := &Bindings{acts: acts, file: f, byName: map[string]int{}}
	names := sortedKeys(f.Mappings)
	for i, n := range names {
		b.byName[n] = i
	}
	for i, n := range names {
		ms := f.Mappings[n]
		path := "$.mappings." + n
		m := mapping{name: n, priority: ms.Priority, block: ms.Block, strictMods: ms.StrictMods}
		regionIdx := map[string]uint16{}
		for _, rn := range sortedKeys(ms.Touch) {
			r, ok := compileRegion(ms.Touch[rn], path+".touch."+rn, &errs)
			if !ok {
				continue
			}
			r.name, r.mapping = rn, i
			regionIdx[rn] = uint16(len(b.regions))
			m.regions = append(m.regions, len(b.regions))
			b.regions = append(b.regions, r)
		}
		lookup := func(name string) (uint16, bool) {
			idx, ok := regionIdx[name]
			return idx, ok
		}
		m.lo = len(b.binds)
		for _, an := range sortedKeys(ms.Bindings) {
			apath := path + ".bindings." + an
			act, ok := acts.Lookup(an)
			if !ok {
				errs = append(errs, fmt.Errorf("%s: unknown action %q", apath, an))
				continue
			}
			slots := map[SlotClass]int{}
			for j, spec := range ms.Bindings[an] {
				bd, ok := b.compileBinding(spec, act, lookup, apath+"["+strconv.Itoa(j)+"]", &errs)
				if !ok {
					continue
				}
				bd.mapping = i
				bd.slot = slots[bd.class]
				slots[bd.class]++
				b.binds = append(b.binds, bd)
			}
		}
		m.hi = len(b.binds)
		b.mappings = append(b.mappings, m)
	}
	for _, gn := range sortedKeys(f.Groups) {
		for j, mn := range f.Groups[gn] {
			if _, ok := b.byName[mn]; !ok {
				errs = append(errs, fmt.Errorf("$.groups.%s[%d]: unknown mapping %q", gn, j, mn))
			}
		}
	}
	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}
	b.link()
	return b, nil
}

// link computes the derived tables: related mappings and the specificity
// order of each mapping's bindings.
func (b *Bindings) link() {
	for i := range b.mappings {
		m := &b.mappings[i]
		m.related = []int{i}
		for _, gn := range sortedKeys(b.file.Groups) {
			g := b.file.Groups[gn]
			if !slices.Contains(g, m.name) {
				continue
			}
			for _, other := range g {
				if j := b.byName[other]; !slices.Contains(m.related, j) {
					m.related = append(m.related, j)
				}
			}
		}
		// More modifiers first: Ctrl+S is evaluated before S and consumes
		// the S key, so the plain binding does not also fire.
		slices.SortStableFunc(b.binds[m.lo:m.hi], func(x, y binding) int {
			return bits.OnesCount8(y.mods) - bits.OnesCount8(x.mods)
		})
	}
}

// controlDims returns the dimension of c's value: 2 for sticks, wheel,
// delta and stick/drag touch regions, 1 otherwise.
func (b *Bindings) controlDims(c Control) int {
	if c.Type == CtrlTouch {
		if c.Code != TouchAny && int(c.Code) < len(b.regions) && b.regions[c.Code].kind != regionButton {
			return 2
		}
		return 1
	}
	return c.dims()
}

func (b *Bindings) compileBinding(s BindingSpec, act Action, region func(string) (uint16, bool), path string, errs *[]error) (binding, bool) {
	kind := b.acts.Kind(act)
	bd := binding{action: act, spec: s}
	n0 := len(*errs)
	fail := func(sub, format string, args ...any) {
		*errs = append(*errs, fmt.Errorf("%s%s: %s", path, sub, fmt.Sprintf(format, args...)))
	}
	parse := func(sub, str string, wantDims int) Control {
		c, err := ParseControl(str, region)
		if err != nil {
			fail(sub, "%v", err)
			return Control{}
		}
		if c.Type == CtrlMod {
			fail(sub, "%q is only valid in mods", str)
			return Control{}
		}
		if d := b.controlDims(c); d != wantDims {
			fail(sub, "control %q has %d dimension(s), %s action %q needs %d", str, d, kind, b.acts.Name(act), wantDims)
			return Control{}
		}
		return c
	}
	scalarParts := func(names []string, values []string, needKind Kind, comp string) {
		if kind != needKind {
			fail("", "composite %q produces %s, but action %q is %s", comp, needKind, b.acts.Name(act), kind)
			return
		}
		found := false
		for i, v := range values {
			if v == "" {
				continue
			}
			found = true
			bd.ctrl[i] = parse("."+names[i], v, 1)
		}
		if !found {
			fail("", "composite %q has no parts", comp)
		}
		bd.n = uint8(len(values))
	}

	switch {
	case s.Control != "" && s.Composite != "":
		fail("", "control and composite are mutually exclusive")
	case s.Control != "":
		want := 1
		if kind == Vec2 {
			want = 2
		}
		bd.ctrl[0] = parse(".control", s.Control, want)
		bd.n = 1
	case s.Composite == "axis":
		bd.kind = bindAxis
		scalarParts(axisParts, []string{s.Neg, s.Pos}, Axis, "axis")
	case s.Composite == "dpad":
		bd.kind = bindDpad
		scalarParts(dpadParts, []string{s.Up, s.Down, s.Left, s.Right}, Vec2, "dpad")
	case s.Composite == "chord":
		bd.kind = bindChord
		if kind != Button {
			fail("", "composite \"chord\" produces Button, but action %q is %s", b.acts.Name(act), kind)
		}
		if len(s.Controls) < 2 || len(s.Controls) > 4 {
			fail(".controls", "a chord needs 2 to 4 controls, got %d", len(s.Controls))
		}
		for i, v := range s.Controls[:min(len(s.Controls), 4)] {
			bd.ctrl[i] = parse(".controls["+strconv.Itoa(i)+"]", v, 1)
		}
		bd.n = uint8(min(len(s.Controls), 4))
	case s.Composite != "":
		fail(".composite", "unknown composite %q (want axis, dpad or chord)", s.Composite)
	default:
		fail("", "missing control or composite")
	}
	if s.Composite == "" && (s.Neg != "" || s.Pos != "" || s.Up != "" || s.Down != "" || s.Left != "" || s.Right != "" || len(s.Controls) > 0) {
		fail("", "composite parts without a composite")
	}

	switch s.WhichWins {
	case "", "none":
	case "last":
		if bd.kind != bindAxis && bd.kind != bindDpad {
			fail(".whichWins", "only axis and dpad composites take whichWins")
		}
		bd.lastWins = true
	default:
		fail(".whichWins", "unknown value %q (want none or last)", s.WhichWins)
	}

	for i, m := range s.Mods {
		c, err := ParseControl(m, nil)
		if err != nil || c.Type != CtrlMod {
			fail(".mods["+strconv.Itoa(i)+"]", "unknown modifier %q (want mod:shift, mod:ctrl, mod:alt or mod:meta)", m)
			continue
		}
		bd.mods |= 1 << c.Code
	}

	for i, p := range s.Processors {
		if pr, ok := compileProcessor(p, kind, path+".processors["+strconv.Itoa(i)+"]", errs); ok {
			bd.procs = append(bd.procs, pr)
		}
	}
	if s.Interaction != nil {
		bd.inter = compileInteraction(*s.Interaction, kind, path+".interaction", errs)
	}
	switch {
	case s.Threshold < 0 || s.Threshold > 1:
		fail(".threshold", "must be in [0, 1], got %v", s.Threshold)
	case s.Threshold == 0:
		bd.threshold = 0.5
	default:
		bd.threshold = s.Threshold
	}
	bd.locked = s.Locked
	for i := range bd.n {
		if c := bd.ctrl[i]; !c.IsZero() {
			bd.class = SlotClassOf(c.Class())
			break
		}
	}
	return bd, len(*errs) == n0
}

func compileProcessor(p ProcessorSpec, kind Kind, path string, errs *[]error) (processor, bool) {
	set := 0
	var pr processor
	fail := func(sub, format string, args ...any) {
		*errs = append(*errs, fmt.Errorf("%s%s: %s", path, sub, fmt.Sprintf(format, args...)))
	}
	if p.Deadzone != nil {
		set++
		pr = processor{op: procDeadzone, a: *p.Deadzone, b: 1}
		if p.Max != nil {
			pr.b = *p.Max
		}
		if pr.a < 0 || pr.a >= 1 {
			fail(".deadzone", "must be in [0, 1), got %v", pr.a)
		}
		if pr.b <= pr.a || pr.b > 1 {
			fail(".max", "must be in (deadzone, 1], got %v", pr.b)
		}
	} else if p.Max != nil {
		fail(".max", "max needs deadzone")
	}
	if p.Invert != "" {
		set++
		pr = processor{op: procInvert}
		switch p.Invert {
		case "x":
			pr.a = 1
		case "y":
			pr.a = 2
		case "xy":
			pr.a = 3
		default:
			fail(".invert", "unknown value %q (want x, y or xy)", p.Invert)
		}
	}
	if p.Scale != nil {
		set++
		pr = processor{op: procScale}
		switch len(p.Scale) {
		case 1:
			pr.a, pr.b = p.Scale[0], p.Scale[0]
		case 2:
			pr.a, pr.b = p.Scale[0], p.Scale[1]
		default:
			fail(".scale", "want 1 or 2 values, got %d", len(p.Scale))
		}
	}
	if p.Normalize {
		set++
		pr = processor{op: procNormalize}
		if kind != Vec2 {
			fail(".normalize", "only Vec2 actions can be normalized")
		}
	}
	if p.Sensitivity {
		set++
		pr = processor{op: procSensitivity}
	}
	if p.Curve != "" {
		set++
		pr = processor{op: procCurve}
		switch p.Curve {
		case "linear":
			pr.a = 1
		case "quadratic":
			pr.a = 2
		case "cubic":
			pr.a = 3
		default:
			fail(".curve", "unknown curve %q (want linear, quadratic or cubic)", p.Curve)
		}
	}
	if set != 1 {
		fail("", "a processor needs exactly one of deadzone, invert, scale, normalize, sensitivity, curve; got %d", set)
		return processor{}, false
	}
	return pr, true
}

func compileInteraction(s InteractionSpec, kind Kind, path string, errs *[]error) interaction {
	set := 0
	var in interaction
	dur := func(sub, v string) time.Duration {
		d, err := time.ParseDuration(v)
		if err != nil || d <= 0 {
			*errs = append(*errs, fmt.Errorf("%s%s: invalid duration %q", path, sub, v))
			return 0
		}
		return d
	}
	if s.Press {
		set++
		in = interaction{op: interPress}
	}
	if s.Release {
		set++
		in = interaction{op: interRelease}
	}
	if s.Hold != "" {
		set++
		in = interaction{op: interHold, d1: dur(".hold", s.Hold)}
	}
	if s.Tap != "" {
		set++
		in = interaction{op: interTap, d1: dur(".tap", s.Tap)}
	}
	if s.DoubleTap != "" {
		set++
		in = interaction{op: interDoubleTap, d1: dur(".doubleTap", s.DoubleTap)}
	}
	if s.Repeat != nil {
		set++
		in = interaction{op: interRepeat, d1: dur(".repeat.delay", s.Repeat.Delay), d2: dur(".repeat.every", s.Repeat.Every)}
	}
	if set != 1 {
		*errs = append(*errs, fmt.Errorf("%s: an interaction needs exactly one of press, release, hold, tap, doubleTap, repeat; got %d", path, set))
		return interaction{}
	}
	if kind != Button && in.op != interPress && in.op != interRepeat {
		*errs = append(*errs, fmt.Errorf("%s: %s actions only take press or repeat", path, kind))
	}
	return in
}

func compileRegion(s TouchRegionSpec, path string, errs *[]error) (region, bool) {
	n0 := len(*errs)
	fail := func(sub, format string, args ...any) {
		*errs = append(*errs, fmt.Errorf("%s%s: %s", path, sub, fmt.Sprintf(format, args...)))
	}
	r := region{radius: s.Radius, floating: s.Floating}
	switch s.Kind {
	case "button":
		r.kind = regionButton
	case "stick":
		r.kind = regionStick
	case "drag":
		r.kind = regionDrag
	default:
		fail(".kind", "unknown kind %q (want button, stick or drag)", s.Kind)
	}
	switch {
	case s.Area != nil && s.Circle != nil:
		fail("", "area and circle are mutually exclusive")
	case s.Area != nil:
		if len(s.Area) != 4 {
			fail(".area", "want [x0, y0, x1, y1], got %d values", len(s.Area))
			break
		}
		copy(r.rect[:], s.Area)
		if r.rect[0] >= r.rect[2] || r.rect[1] >= r.rect[3] || r.rect[0] < 0 || r.rect[1] < 0 || r.rect[2] > 1 || r.rect[3] > 1 {
			fail(".area", "want 0 <= x0 < x1 <= 1 and 0 <= y0 < y1 <= 1, got %v", s.Area)
		}
	case s.Circle != nil:
		if len(s.Circle) != 3 {
			fail(".circle", "want [cx, cy, r], got %d values", len(s.Circle))
			break
		}
		r.circle = true
		copy(r.rect[:3], s.Circle)
		if r.rect[2] <= 0 {
			fail(".circle", "radius must be positive, got %v", r.rect[2])
		}
	default:
		fail("", "missing area or circle")
	}
	if s.Radius < 0 {
		fail(".radius", "must not be negative, got %v", s.Radius)
	}
	if r.radius == 0 {
		r.radius = 0.1
	}
	if s.Floating && r.kind != regionStick {
		fail(".floating", "only sticks float")
	}
	return r, len(*errs) == n0
}

// Actions returns the action table the bindings were compiled against.
func (b *Bindings) Actions() *Actions { return b.acts }

// File returns the decoded file the bindings were compiled from. Treat it
// as read-only.
func (b *Bindings) File() BindingsFile { return b.file }

// HasMapping reports whether the bindings define the mapping name.
func (b *Bindings) HasMapping(name string) bool {
	_, ok := b.byName[name]
	return ok
}

// MappingNames returns the mapping names, sorted.
func (b *Bindings) MappingNames() []string {
	out := make([]string, len(b.mappings))
	for i, m := range b.mappings {
		out[i] = m.name
	}
	return out
}

// ControlString returns the file form of c, with touch region names.
func (b *Bindings) ControlString(c Control) string {
	return c.format(func(i uint16) string {
		if int(i) < len(b.regions) {
			return b.regions[i].name
		}
		return "#" + strconv.Itoa(int(i))
	})
}
