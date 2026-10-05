package config

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// DefaultHotkey is left Win plus left Alt, nothing else.
const DefaultHotkey = "LWin & LAlt"

// Virtual-key codes the hook compares against (WinUser.h).
const (
	VKLWin     = 0x5B
	VKRWin     = 0x5C
	VKLShift   = 0xA0
	VKRShift   = 0xA1
	VKLControl = 0xA2
	VKRControl = 0xA3
	VKLAlt     = 0xA4
	VKRAlt     = 0xA5
)

// Side says which of a left/right modifier pair a hotkey requires.
type Side uint8

const (
	SideNone Side = iota // must not be held
	SideAny
	SideLeft
	SideRight
)

// Key matches one or two virtual-key codes, e.g. Alt is LAlt or RAlt.
type Key [2]uint32

func (k Key) Has(vk uint32) bool { return vk != 0 && (k[0] == vk || k[1] == vk) }

// Hotkey is a parsed settings.hotkey.
//
// Form 1 (Trigger != 0): Win, Alt, Ctrl and Shift held as given, then
// Trigger goes down. Form 2 (Trigger == 0): the two Combo keys held
// together in either order, fired on release, nothing else pressed between.
type Hotkey struct {
	Win, Alt, Ctrl, Shift Side
	Trigger               uint32
	Combo                 [2]Key
}

func (h Hotkey) IsCombo() bool { return h.Trigger == 0 }

// HasWin reports whether the hotkey involves a Win key, so releasing Win
// needs the Ctrl mask against the Start menu.
func (h Hotkey) HasWin() bool {
	if !h.IsCombo() {
		return h.Win != SideNone
	}
	for _, k := range h.Combo {
		if k.Has(VKLWin) || k.Has(VKRWin) {
			return true
		}
	}
	return false
}

var modifierKeys = map[string]Key{
	"lwin": {VKLWin}, "rwin": {VKRWin},
	"lalt": {VKLAlt}, "ralt": {VKRAlt}, "alt": {VKLAlt, VKRAlt},
	"lctrl": {VKLControl}, "rctrl": {VKRControl}, "ctrl": {VKLControl, VKRControl},
	"lcontrol": {VKLControl}, "rcontrol": {VKRControl}, "control": {VKLControl, VKRControl},
	"lshift": {VKLShift}, "rshift": {VKRShift}, "shift": {VKLShift, VKRShift},
}

var namedKeys = map[string]uint32{
	"space": 0x20, "tab": 0x09, "enter": 0x0D, "escape": 0x1B, "esc": 0x1B,
	"backspace": 0x08, "bs": 0x08, "delete": 0x2E, "del": 0x2E, "insert": 0x2D, "ins": 0x2D,
	"home": 0x24, "end": 0x23, "pgup": 0x21, "pgdn": 0x22,
	"up": 0x26, "down": 0x28, "left": 0x25, "right": 0x27,
	"appskey": 0x5D, "capslock": 0x14, "scrolllock": 0x91, "numlock": 0x90,
	"printscreen": 0x2C, "pause": 0x13,
	"numpadmult": 0x6A, "numpadadd": 0x6B, "numpadsub": 0x6D, "numpaddot": 0x6E, "numpaddiv": 0x6F,
}

var mouseKeys = map[string]bool{
	"lbutton": true, "rbutton": true, "mbutton": true, "xbutton1": true, "xbutton2": true,
	"wheelup": true, "wheeldown": true, "wheelleft": true, "wheelright": true,
}

// lookupKey resolves one AutoHotkey key name.
func lookupKey(name string) (Key, error) {
	n := strings.ToLower(strings.TrimSpace(name))
	switch {
	case n == "":
		return Key{}, errors.New("missing key")
	case mouseKeys[n]:
		return Key{}, fmt.Errorf("mouse %q is not supported", name)
	case len(n) > 2 && (strings.HasPrefix(n, "sc") || strings.HasPrefix(n, "vk")):
		if _, err := strconv.ParseUint(n[2:], 16, 32); err == nil {
			return Key{}, fmt.Errorf("scan and virtual key codes (%q) are not supported", name)
		}
	}
	if k, ok := modifierKeys[n]; ok {
		return k, nil
	}
	if vk, ok := namedKeys[n]; ok {
		return Key{vk}, nil
	}
	if len(n) == 1 && (n[0] >= 'a' && n[0] <= 'z' || n[0] >= '0' && n[0] <= '9') {
		return Key{uint32(strings.ToUpper(n)[0])}, nil
	}
	if strings.HasPrefix(n, "numpad") {
		if d, err := strconv.Atoi(n[6:]); err == nil && d >= 0 && d <= 9 {
			return Key{uint32(0x60 + d)}, nil
		}
	}
	if strings.HasPrefix(n, "f") {
		if d, err := strconv.Atoi(n[1:]); err == nil && d >= 1 && d <= 24 {
			return Key{uint32(0x70 + d - 1)}, nil
		}
	}
	return Key{}, fmt.Errorf("unknown key %q", name)
}

func isModifierKey(k Key) bool {
	for _, m := range modifierKeys {
		if m == k {
			return true
		}
	}
	return false
}

// ParseHotkey reads AutoHotkey v2 hotkey notation without "::".
func ParseHotkey(s string) (Hotkey, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return Hotkey{}, errors.New("empty")
	}
	if strings.ContainsAny(s, "~$*") {
		return Hotkey{}, errors.New("prefixes ~ $ * are not supported")
	}
	if f := strings.Fields(s); len(f) > 1 && strings.EqualFold(f[len(f)-1], "up") {
		return Hotkey{}, errors.New(`release hotkeys ("Up") are not supported`)
	}
	if strings.Contains(s, "&") {
		return parseCombo(s)
	}
	return parseModified(s)
}

func parseCombo(s string) (Hotkey, error) {
	if strings.ContainsAny(s, "#!^+<>") {
		return Hotkey{}, errors.New("& cannot be mixed with # ! ^ + < >")
	}
	parts := strings.Split(s, "&")
	if len(parts) != 2 {
		return Hotkey{}, errors.New("& joins exactly two keys")
	}
	var h Hotkey
	for i, p := range parts {
		k, err := lookupKey(p)
		if err != nil {
			return Hotkey{}, err
		}
		h.Combo[i] = k
	}
	a, b := h.Combo[0], h.Combo[1]
	if a.Has(b[0]) || a.Has(b[1]) {
		return Hotkey{}, errors.New("the two keys must differ")
	}
	return h, nil
}

func parseModified(s string) (Hotkey, error) {
	if strings.Contains(s, "<^>!") {
		return Hotkey{}, errors.New("AltGr (<^>!) is not supported")
	}
	var h Hotkey
	side := SideAny
	i := 0
loop:
	for ; i < len(s); i++ {
		var m *Side
		switch s[i] {
		case '<':
			side = SideLeft
			continue
		case '>':
			side = SideRight
			continue
		case '#':
			m = &h.Win
		case '!':
			m = &h.Alt
		case '^':
			m = &h.Ctrl
		case '+':
			m = &h.Shift
		default:
			break loop
		}
		if *m != SideNone {
			return Hotkey{}, fmt.Errorf("modifier %c given twice", s[i])
		}
		*m, side = side, SideAny
	}
	if side != SideAny {
		return Hotkey{}, errors.New("< or > must stand right before a modifier")
	}
	if i == 0 {
		return Hotkey{}, errors.New("a single key is not a hotkey; add a modifier or use \"A & B\"")
	}
	k, err := lookupKey(s[i:])
	if err != nil {
		return Hotkey{}, err
	}
	if isModifierKey(k) {
		return Hotkey{}, errors.New("a modifier key after modifiers is not supported; use \"A & B\"")
	}
	h.Trigger = k[0]
	return h, nil
}
