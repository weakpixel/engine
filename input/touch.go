package input

// TouchKind is the kind of a touch region.
type TouchKind uint8

// Touch region kinds.
const (
	// TouchButton is digital: down while a touch that started in it lasts.
	TouchButton TouchKind = iota
	// TouchStick is a Vec2 from the stick's base to the touch, clamped to
	// its radius.
	TouchStick
	// TouchDrag is a Vec2 delta in logical pixels per tick, like the mouse.
	TouchDrag
)

// TouchControl is the live geometry of one touch region, in logical pixels,
// for drawing virtual controls (inputsys.TouchControlsExtract does).
type TouchControl struct {
	// Name and Mapping identify the region in the bindings file.
	Name, Mapping string
	Kind          TouchKind
	// X0, Y0, X1, Y1 bound the region.
	X0, Y0, X1, Y1 float32
	// Circle is set for circular regions; CX, CY, R are then the circle.
	// For sticks CX, CY is the stick's base and R its travel.
	Circle    bool
	CX, CY, R float32
	// KnobX, KnobY is where a stick's knob is drawn.
	KnobX, KnobY float32
	// Pressed is set while a touch owns the region.
	Pressed bool
}

type trackedTouch struct {
	id     int32
	region int // -1: started outside every region
	sx, sy float32
	px, py float32
}

type regionValue struct {
	x, y    float32
	pressed bool
	button  bool
	bx, by  float32 // stick base
}

// touchTracker follows the touches of the touch screen. A touch belongs to
// the first region (in evaluation order) it started in until it ends, so a
// thumb sliding off the stick keeps steering and never presses a button.
type touchTracker struct {
	list     [MaxTouches]trackedTouch
	n        int
	vals     []regionValue
	controls []TouchControl
}

func (t *touchTracker) reset() {
	t.n = 0
	clear(t.vals)
}

func (t *touchTracker) value(region uint16) (float32, float32) {
	if int(region) >= len(t.vals) {
		return 0, 0
	}
	v := &t.vals[region]
	if v.button {
		return 1, 0 // sticks and drags report their vector
	}
	return v.x, v.y
}

// update runs once per tick for the player that has the touch screen.
func (t *touchTracker) update(p *Player) {
	f := &p.r.frame
	b := p.eff
	w, h := float32(max(f.Width, 1)), float32(max(f.Height, 1))
	if len(t.vals) != len(b.regions) {
		t.vals = make([]regionValue, len(b.regions)) // only when the bindings change
	}
	clear(t.vals)

	// Drop touches that ended.
	keep := 0
	for _, tt := range t.list[:t.n] {
		if findTouch(f, tt.id) >= 0 && tt.region < len(b.regions) {
			t.list[keep] = tt
			keep++
		}
	}
	t.n = keep

	// Adopt new touches.
	for _, ft := range f.Touches[:f.NTouches] {
		if t.indexOf(ft.ID) >= 0 || t.n == MaxTouches {
			continue
		}
		t.list[t.n] = trackedTouch{id: ft.ID, region: t.hit(p, ft.X, ft.Y, w, h), sx: ft.X, sy: ft.Y, px: ft.X, py: ft.Y}
		t.n++
	}

	// Region values.
	for i := range t.list[:t.n] {
		tt := &t.list[i]
		ft := &f.Touches[findTouch(f, tt.id)]
		if tt.region >= 0 && !t.vals[tt.region].pressed {
			rg := &b.regions[tt.region]
			v := &t.vals[tt.region]
			v.pressed = true
			v.button = rg.kind == regionButton
			switch rg.kind {
			case regionStick:
				v.bx, v.by = tt.sx, tt.sy
				if !rg.floating {
					v.bx, v.by = regionCenter(rg, w, h)
				}
				rad := rg.radius * h
				x, y := (ft.X-v.bx)/rad, (ft.Y-v.by)/rad
				if l := len32(x, y); l > 1 {
					x, y = x/l, y/l
				}
				v.x, v.y = x, y
			case regionDrag:
				v.x, v.y = ft.X-tt.px, ft.Y-tt.py
			}
		}
		tt.px, tt.py = ft.X, ft.Y
	}

	// Geometry of the regions of the evaluated mappings.
	t.controls = t.controls[:0]
	for _, mi := range p.order {
		for _, ri := range b.mappings[mi].regions {
			t.controls = append(t.controls, t.geometry(b, ri, w, h))
		}
	}
}

func (t *touchTracker) geometry(b *Bindings, ri int, w, h float32) TouchControl {
	rg := &b.regions[ri]
	v := &t.vals[ri]
	tc := TouchControl{Name: rg.name, Mapping: b.mappings[rg.mapping].name, Kind: TouchKind(rg.kind), Circle: rg.circle, Pressed: v.pressed}
	if rg.circle {
		cx, cy, r := rg.rect[0]*w, rg.rect[1]*h, rg.rect[2]*h
		tc.X0, tc.Y0, tc.X1, tc.Y1 = cx-r, cy-r, cx+r, cy+r
		tc.CX, tc.CY, tc.R = cx, cy, r
	} else {
		tc.X0, tc.Y0, tc.X1, tc.Y1 = rg.rect[0]*w, rg.rect[1]*h, rg.rect[2]*w, rg.rect[3]*h
		tc.CX, tc.CY = (tc.X0+tc.X1)/2, (tc.Y0+tc.Y1)/2
	}
	if rg.kind == regionStick {
		tc.R = rg.radius * h
		if v.pressed {
			tc.CX, tc.CY = v.bx, v.by
		}
		tc.KnobX, tc.KnobY = tc.CX+v.x*tc.R, tc.CY+v.y*tc.R
	}
	return tc
}

// hit returns the first region of the evaluated mappings that contains
// (x, y), or -1.
func (t *touchTracker) hit(p *Player, x, y, w, h float32) int {
	b := p.eff
	for _, mi := range p.order {
		for _, ri := range b.mappings[mi].regions {
			rg := &b.regions[ri]
			if rg.circle {
				dx, dy := x-rg.rect[0]*w, y-rg.rect[1]*h
				if r := rg.rect[2] * h; dx*dx+dy*dy <= r*r {
					return ri
				}
			} else if x >= rg.rect[0]*w && x < rg.rect[2]*w && y >= rg.rect[1]*h && y < rg.rect[3]*h {
				return ri
			}
		}
	}
	return -1
}

func regionCenter(rg *region, w, h float32) (float32, float32) {
	if rg.circle {
		return rg.rect[0] * w, rg.rect[1] * h
	}
	return (rg.rect[0] + rg.rect[2]) / 2 * w, (rg.rect[1] + rg.rect[3]) / 2 * h
}

func (t *touchTracker) indexOf(id int32) int {
	for i := range t.list[:t.n] {
		if t.list[i].id == id {
			return i
		}
	}
	return -1
}

func findTouch(f *Frame, id int32) int {
	for i := range f.Touches[:f.NTouches] {
		if f.Touches[i].ID == id {
			return i
		}
	}
	return -1
}
