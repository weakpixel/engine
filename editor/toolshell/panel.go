package toolshell

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
)

// Widget is the kind of a panel [Field].
type Widget string

// The widgets of M4a. A widget exists only once a tool needs it, and then in
// both renderers in the same change.
const (
	// WidgetNumber is a number field (a slider too when Min and Max are
	// both set). Its value is a JSON number.
	WidgetNumber Widget = "number"
	// WidgetCheckbox is a checkbox. Its value is a JSON boolean.
	WidgetCheckbox Widget = "checkbox"
	// WidgetDropdown picks one of Options. Its value is the option index.
	WidgetDropdown Widget = "dropdown"
	// WidgetText is a one-line text field; Set receives the text when the
	// user confirms it (Enter or blur). Its value is a JSON string.
	WidgetText Widget = "text"
	// WidgetButton is a button. It has no value; Set receives null.
	WidgetButton Widget = "button"
	// WidgetLabel is read-only text such as "512 × 256 px" or the mouse
	// position. Its value is a JSON string.
	WidgetLabel Widget = "label"
	// WidgetRange is a pair of numbers, low and high, that never cross. Min,
	// Max, Step and Digits bound and format both. Its value is the JSON array
	// [lo, hi].
	WidgetRange Widget = "range"
	// WidgetColor is a color picker with an alpha channel. Its value is the
	// string "#rrggbbaa".
	WidgetColor Widget = "color"
	// WidgetAsset picks an asset key from Options. Its value is the key, ""
	// for none. Kind says what the asset is and Thumbs holds the pictures of
	// the entries the page cannot fetch.
	WidgetAsset Widget = "asset"
)

// The kinds of an asset field.
const (
	AssetImage  = "image"
	AssetShader = "shader"
)

// Panel is what a tool shows next to the viewport. It is plain data: the
// shell marshals it for the extension and the debugui renderer walks it.
type Panel struct {
	// Title names the panel (a window title, a heading).
	Title string `json:"title"`
	// Status is a line at the bottom for messages: a dirty document that
	// changed on disk, a failed load. The shell sets it; tools leave it
	// empty.
	Status   string    `json:"status,omitempty"`
	Sections []Section `json:"sections"`
}

// Side is the panel of the native window a [Section] is drawn in.
type Side string

// The sides of a section. The native window shows the view settings and the
// lists content is picked from on the left, the properties of the selection
// on the right. The extension shows one column and ignores the side.
const (
	SideLeft  Side = "left"
	SideRight Side = "right"
)

// Section is a collapsible group of fields.
type Section struct {
	// ID is unique in the panel.
	ID    string `json:"id"`
	Label string `json:"label"`
	// Side is the native panel the section is drawn in: [SideLeft] or
	// [SideRight]; empty means right.
	Side Side `json:"side,omitempty"`
	// Collapsed is the initial state; a renderer keeps what the user chose.
	Collapsed bool    `json:"collapsed,omitempty"`
	Fields    []Field `json:"fields"`
}

// Field is one widget of a [Section].
type Field struct {
	// ID is unique in the panel; [Tool.Set] receives it.
	ID    string `json:"id"`
	Label string `json:"label"`
	// Widget is the kind of widget.
	Widget Widget `json:"widget"`
	// Value is the current value (see the Widget constants); none for a
	// button.
	Value json.RawMessage `json:"value,omitempty"`
	// Min, Max and Step bound and step a number. A renderer keeps what the
	// user types inside the bounds that are set; with both set it shows a
	// slider too. One bound alone (see [Field.AtLeast]) limits the number
	// without a slider.
	Min  *float64 `json:"min,omitempty"`
	Max  *float64 `json:"max,omitempty"`
	Step *float64 `json:"step,omitempty"`
	// Digits is the decimals a number shows; 0 is an integer and Set
	// receives an integer.
	Digits int `json:"digits,omitempty"`
	// Options are the entries of a dropdown, or the candidate asset keys of
	// an asset field.
	Options []string `json:"options,omitempty"`
	// Kind is the kind of an asset field: [AssetImage] or [AssetShader].
	Kind string `json:"kind,omitempty"`
	// Thumbs maps an asset key to a PNG data URI for the entries of an asset
	// field that are not files the page can fetch (built-in sprites).
	Thumbs map[string]string `json:"thumbs,omitempty"`
	// Readonly shows the widget but does not let the user change it.
	Readonly bool `json:"readonly,omitempty"`
	// Hint is a tooltip.
	Hint string `json:"hint,omitempty"`
}

// mustJSON marshals v, a number, bool or string, for a Field value.
func mustJSON(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		panic("toolshell: " + err.Error())
	}
	return b
}

// Number returns a number field with value v and step 1.
func Number(id, label string, v float64) Field {
	step := 1.0
	return Field{ID: id, Label: label, Widget: WidgetNumber, Value: mustJSON(v), Step: &step}
}

// Range returns f with its bounds set, which makes a renderer show a slider.
func (f Field) Range(lo, hi float64) Field {
	f.Min, f.Max = &lo, &hi
	return f
}

// AtLeast returns f with only a lower bound: no slider, but a renderer
// does not let the value go below lo.
func (f Field) AtLeast(lo float64) Field {
	f.Min, f.Max = &lo, nil
	return f
}

// WithStep returns f with its step set.
func (f Field) WithStep(step float64) Field {
	f.Step = &step
	return f
}

// WithDigits returns f with the number of decimals shown set.
func (f Field) WithDigits(n int) Field {
	f.Digits = n
	return f
}

// WithHint returns f with its tooltip set.
func (f Field) WithHint(hint string) Field {
	f.Hint = hint
	return f
}

// AsReadonly returns f with Readonly set.
func (f Field) AsReadonly() Field {
	f.Readonly = true
	return f
}

// Checkbox returns a checkbox field.
func Checkbox(id, label string, v bool) Field {
	return Field{ID: id, Label: label, Widget: WidgetCheckbox, Value: mustJSON(v)}
}

// Dropdown returns a dropdown field with the option at index selected.
func Dropdown(id, label string, index int, options ...string) Field {
	return Field{ID: id, Label: label, Widget: WidgetDropdown, Value: mustJSON(index), Options: options}
}

// Text returns a text field.
func Text(id, label, v string) Field {
	return Field{ID: id, Label: label, Widget: WidgetText, Value: mustJSON(v)}
}

// Button returns a button. Its Set value is null.
func Button(id, label string) Field {
	return Field{ID: id, Label: label, Widget: WidgetButton}
}

// Label returns read-only text: label on the left, text on the right.
func Label(id, label, text string) Field {
	return Field{ID: id, Label: label, Widget: WidgetLabel, Value: mustJSON(text), Readonly: true}
}

// NumberRange returns a range field with the value [lo, hi] and step 1.
func NumberRange(id, label string, lo, hi float64) Field {
	step := 1.0
	return Field{ID: id, Label: label, Widget: WidgetRange, Value: mustJSON([2]float64{lo, hi}), Step: &step}
}

// Color returns a color field; hex is "#rrggbbaa".
func Color(id, label, hex string) Field {
	return Field{ID: id, Label: label, Widget: WidgetColor, Value: mustJSON(hex)}
}

// Asset returns an asset field of kind ([AssetImage] or [AssetShader]) with
// the key selected, "" for none, and the candidate keys.
func Asset(id, label, kind, key string, options []string) Field {
	return Field{ID: id, Label: label, Widget: WidgetAsset, Kind: kind, Value: mustJSON(key), Options: options}
}

// WithThumbs returns f with the thumbnails of its entries set.
func (f Field) WithThumbs(thumbs map[string]string) Field {
	f.Thumbs = thumbs
	return f
}

// Check validates the panel and returns every problem, joined: duplicate
// ids (sections and fields share one id space), an unknown widget or side, a
// missing label, a dropdown value outside its options, a value of the wrong kind.
func (p Panel) Check() error {
	var errs []error
	seen := map[string]bool{}
	use := func(id, what string) {
		switch {
		case id == "":
			errs = append(errs, fmt.Errorf("%s has no id", what))
		case seen[id]:
			errs = append(errs, fmt.Errorf("duplicate id %q", id))
		default:
			seen[id] = true
		}
	}
	for _, s := range p.Sections {
		use(s.ID, "section "+fmt.Sprintf("%q", s.Label))
		if s.Label == "" {
			errs = append(errs, fmt.Errorf("section %q has no label", s.ID))
		}
		if s.Side != "" && s.Side != SideLeft && s.Side != SideRight {
			errs = append(errs, fmt.Errorf("section %q: unknown side %q", s.ID, s.Side))
		}
		for _, f := range s.Fields {
			use(f.ID, "field "+fmt.Sprintf("%q", f.Label))
			errs = append(errs, f.check()...)
		}
	}
	return errors.Join(errs...)
}

// check returns the problems of one field.
func (f Field) check() []error {
	var errs []error
	bad := func(format string, a ...any) {
		errs = append(errs, fmt.Errorf("field %q: %s", f.ID, fmt.Sprintf(format, a...)))
	}
	if f.Label == "" {
		bad("no label")
	}
	valueIs := func(kind string, into any) {
		if err := json.Unmarshal(f.Value, into); err != nil {
			bad("%s value: %v", kind, err)
		}
	}
	switch f.Widget {
	case WidgetNumber:
		var v float64
		valueIs("number", &v)
		if f.Min != nil && f.Max != nil && *f.Min > *f.Max {
			bad("Min %v is above Max %v", *f.Min, *f.Max)
		}
	case WidgetCheckbox:
		var v bool
		valueIs("boolean", &v)
	case WidgetDropdown:
		var v int
		valueIs("integer", &v)
		if len(f.Options) == 0 {
			bad("dropdown has no options")
		} else if v < 0 || v >= len(f.Options) {
			bad("dropdown value %d is outside its %d options", v, len(f.Options))
		}
	case WidgetText, WidgetLabel:
		var v string
		valueIs("string", &v)
	case WidgetRange:
		var v []float64
		valueIs("[lo, hi]", &v)
		if len(v) != 2 {
			bad("range value has %d numbers, want 2", len(v))
		} else if v[0] > v[1] {
			bad("range value %v is above %v", v[0], v[1])
		}
		if f.Min != nil && f.Max != nil && *f.Min > *f.Max {
			bad("Min %v is above Max %v", *f.Min, *f.Max)
		}
	case WidgetColor:
		var v string
		valueIs("string", &v)
		if !validHexColor(v) {
			bad("color %q is not #rrggbbaa", v)
		}
	case WidgetAsset:
		var v string
		valueIs("string", &v)
		if f.Kind != AssetImage && f.Kind != AssetShader {
			bad("asset kind %q is not image or shader", f.Kind)
		}
		if v != "" && !slices.Contains(f.Options, v) {
			bad("asset %q is not one of its options", v)
		}
	case WidgetButton:
	default:
		bad("unknown widget %q", f.Widget)
	}
	return errs
}

// validHexColor reports whether s is "#rrggbbaa".
func validHexColor(s string) bool {
	if len(s) != 9 || s[0] != '#' {
		return false
	}
	for _, c := range s[1:] {
		if !strings.ContainsRune("0123456789abcdefABCDEF", c) {
			return false
		}
	}
	return true
}

// Find returns the field with id.
func (p Panel) Find(id string) (Field, bool) {
	for _, s := range p.Sections {
		for _, f := range s.Fields {
			if f.ID == id {
				return f, true
			}
		}
	}
	return Field{}, false
}
