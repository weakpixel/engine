package input

// Key is a physical keyboard key, named by its position on a US layout
// ("W" stays the key below "2" on AZERTY). The names are the same as
// Ebiten's key names without the "Key" prefix; [KeyName] and [ParseKey]
// convert. The zero Key is "none".
type Key uint8

// Keys. The order is part of the recording format (see [Recorder]); append
// new keys at the end.
const (
	KeyA Key = iota + 1
	KeyB
	KeyC
	KeyD
	KeyE
	KeyF
	KeyG
	KeyH
	KeyI
	KeyJ
	KeyK
	KeyL
	KeyM
	KeyN
	KeyO
	KeyP
	KeyQ
	KeyR
	KeyS
	KeyT
	KeyU
	KeyV
	KeyW
	KeyX
	KeyY
	KeyZ
	KeyAltLeft
	KeyAltRight
	KeyArrowDown
	KeyArrowLeft
	KeyArrowRight
	KeyArrowUp
	KeyBackquote
	KeyBackslash
	KeyBackspace
	KeyBracketLeft
	KeyBracketRight
	KeyCapsLock
	KeyComma
	KeyContextMenu
	KeyControlLeft
	KeyControlRight
	KeyDelete
	KeyDigit0
	KeyDigit1
	KeyDigit2
	KeyDigit3
	KeyDigit4
	KeyDigit5
	KeyDigit6
	KeyDigit7
	KeyDigit8
	KeyDigit9
	KeyEnd
	KeyEnter
	KeyEqual
	KeyEscape
	KeyF1
	KeyF2
	KeyF3
	KeyF4
	KeyF5
	KeyF6
	KeyF7
	KeyF8
	KeyF9
	KeyF10
	KeyF11
	KeyF12
	KeyF13
	KeyF14
	KeyF15
	KeyF16
	KeyF17
	KeyF18
	KeyF19
	KeyF20
	KeyF21
	KeyF22
	KeyF23
	KeyF24
	KeyHome
	KeyInsert
	KeyIntlBackslash
	KeyMetaLeft
	KeyMetaRight
	KeyMinus
	KeyNumLock
	KeyNumpad0
	KeyNumpad1
	KeyNumpad2
	KeyNumpad3
	KeyNumpad4
	KeyNumpad5
	KeyNumpad6
	KeyNumpad7
	KeyNumpad8
	KeyNumpad9
	KeyNumpadAdd
	KeyNumpadDecimal
	KeyNumpadDivide
	KeyNumpadEnter
	KeyNumpadEqual
	KeyNumpadMultiply
	KeyNumpadSubtract
	KeyPageDown
	KeyPageUp
	KeyPause
	KeyPeriod
	KeyPrintScreen
	KeyQuote
	KeyScrollLock
	KeySemicolon
	KeyShiftLeft
	KeyShiftRight
	KeySlash
	KeySpace
	KeyTab

	numKeys = iota + 1 // one past the last key
)

var keyNames = [numKeys]string{
	"",
	"A",
	"B",
	"C",
	"D",
	"E",
	"F",
	"G",
	"H",
	"I",
	"J",
	"K",
	"L",
	"M",
	"N",
	"O",
	"P",
	"Q",
	"R",
	"S",
	"T",
	"U",
	"V",
	"W",
	"X",
	"Y",
	"Z",
	"AltLeft",
	"AltRight",
	"ArrowDown",
	"ArrowLeft",
	"ArrowRight",
	"ArrowUp",
	"Backquote",
	"Backslash",
	"Backspace",
	"BracketLeft",
	"BracketRight",
	"CapsLock",
	"Comma",
	"ContextMenu",
	"ControlLeft",
	"ControlRight",
	"Delete",
	"Digit0",
	"Digit1",
	"Digit2",
	"Digit3",
	"Digit4",
	"Digit5",
	"Digit6",
	"Digit7",
	"Digit8",
	"Digit9",
	"End",
	"Enter",
	"Equal",
	"Escape",
	"F1",
	"F2",
	"F3",
	"F4",
	"F5",
	"F6",
	"F7",
	"F8",
	"F9",
	"F10",
	"F11",
	"F12",
	"F13",
	"F14",
	"F15",
	"F16",
	"F17",
	"F18",
	"F19",
	"F20",
	"F21",
	"F22",
	"F23",
	"F24",
	"Home",
	"Insert",
	"IntlBackslash",
	"MetaLeft",
	"MetaRight",
	"Minus",
	"NumLock",
	"Numpad0",
	"Numpad1",
	"Numpad2",
	"Numpad3",
	"Numpad4",
	"Numpad5",
	"Numpad6",
	"Numpad7",
	"Numpad8",
	"Numpad9",
	"NumpadAdd",
	"NumpadDecimal",
	"NumpadDivide",
	"NumpadEnter",
	"NumpadEqual",
	"NumpadMultiply",
	"NumpadSubtract",
	"PageDown",
	"PageUp",
	"Pause",
	"Period",
	"PrintScreen",
	"Quote",
	"ScrollLock",
	"Semicolon",
	"ShiftLeft",
	"ShiftRight",
	"Slash",
	"Space",
	"Tab",
}

var keyByName = func() map[string]Key {
	m := make(map[string]Key, numKeys)
	for k := Key(1); k < numKeys; k++ {
		m[keyNames[k]] = k
	}
	return m
}()

// KeyCount is the number of keys; valid keys are 1..KeyCount.
const KeyCount = int(numKeys) - 1

// String returns the key's name ("Space", "ArrowUp"), or "" for an invalid key.
func (k Key) String() string {
	if k == 0 || k >= numKeys {
		return ""
	}
	return keyNames[k]
}

// ParseKey returns the key named name (case-sensitive, as [Key.String] writes it).
func ParseKey(name string) (Key, bool) {
	k, ok := keyByName[name]
	return k, ok
}

// KeySet is the set of keys that are down: one bit per key, no pointers, so
// a Frame stays a plain value that can be copied and recorded.
type KeySet [2]uint64

// Has reports whether k is in the set.
func (s *KeySet) Has(k Key) bool { return k != 0 && s[k>>6]&(1<<(k&63)) != 0 }

// Set adds k to the set. The zero Key is ignored.
func (s *KeySet) Set(k Key) {
	if k != 0 && k < numKeys {
		s[k>>6] |= 1 << (k & 63)
	}
}

// Clear removes k from the set.
func (s *KeySet) Clear(k Key) {
	if k != 0 && k < numKeys {
		s[k>>6] &^= 1 << (k & 63)
	}
}

// Reset empties the set.
func (s *KeySet) Reset() { *s = KeySet{} }

// Empty reports whether no key is down.
func (s *KeySet) Empty() bool { return s[0] == 0 && s[1] == 0 }
