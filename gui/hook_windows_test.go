package gui

import (
	"testing"

	"ora/core/config"
)

type keyEv struct {
	vk   uint32
	down bool
}

func dn(vk uint32) keyEv { return keyEv{vk, true} }
func up(vk uint32) keyEv { return keyEv{vk, false} }

// run feeds events and records "F" for a fire and "M" for a mask, with
// the index of the event that caused it.
func run(t *testing.T, hotkey string, evs ...keyEv) (fires, masks []int) {
	t.Helper()
	hk, err := config.ParseHotkey(hotkey)
	if err != nil {
		t.Fatal(err)
	}
	s := &hotkeyState{hk: hk}
	cur := 0
	s.fire = func() { fires = append(fires, cur) }
	s.mask = func() { masks = append(masks, cur) }
	for i, e := range evs {
		cur = i
		s.key(e.vk, e.down)
	}
	return fires, masks
}

const (
	lwin = config.VKLWin
	lalt = config.VKLAlt
	ralt = config.VKRAlt
	lctl = config.VKLControl
	tab  = 0x09
)

func eq(a []int, b ...int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestComboBothOrders(t *testing.T) {
	f, m := run(t, config.DefaultHotkey, dn(lwin), dn(lalt), up(lalt), up(lwin))
	if !eq(f, 2) || !eq(m, 3) {
		t.Errorf("win first: fires %v masks %v", f, m)
	}
	f, m = run(t, config.DefaultHotkey, dn(lalt), dn(lwin), up(lwin), up(lalt))
	if !eq(f, 2) || !eq(m, 2) {
		t.Errorf("alt first, win released first: fires %v masks %v", f, m)
	}
}

func TestComboRepeatsAreNotAThirdKey(t *testing.T) {
	f, _ := run(t, config.DefaultHotkey, dn(lwin), dn(lwin), dn(lalt), dn(lalt), dn(lalt), up(lalt), up(lwin))
	if !eq(f, 5) {
		t.Errorf("fires %v", f)
	}
}

func TestComboOtherKeyCancels(t *testing.T) {
	f, m := run(t, config.DefaultHotkey, dn(lwin), dn(lalt), dn(tab), up(tab), up(lalt), up(lwin))
	if len(f) != 0 || len(m) != 0 {
		t.Errorf("Win+Alt+Tab: fires %v masks %v", f, m)
	}
	f, _ = run(t, config.DefaultHotkey, dn(lwin), dn('E'), up('E'), dn(lalt), up(lalt), up(lwin))
	if len(f) != 0 {
		t.Errorf("key before the second: fires %v", f)
	}
}

func TestComboRightAltAndAltGr(t *testing.T) {
	f, _ := run(t, config.DefaultHotkey, dn(lwin), dn(ralt), up(ralt), up(lwin))
	if len(f) != 0 {
		t.Errorf("RAlt fired the default: %v", f)
	}
	f, _ = run(t, config.DefaultHotkey, dn(lctl), dn(ralt), up(ralt), up(lctl))
	if len(f) != 0 {
		t.Errorf("AltGr fired: %v", f)
	}
}

func TestComboOnceAndAgain(t *testing.T) {
	f, _ := run(t, config.DefaultHotkey,
		dn(lwin), dn(lalt), up(lalt), dn(lalt), up(lalt), up(lwin),
		dn(lwin), dn(lalt), up(lwin), up(lalt))
	if !eq(f, 2, 8) {
		t.Errorf("a second tap while Win stays down must not fire; fires %v", f)
	}
}

func TestPlainWinNoMask(t *testing.T) {
	f, m := run(t, config.DefaultHotkey, dn(lwin), up(lwin))
	if len(f) != 0 || len(m) != 0 {
		t.Errorf("Win alone opens the Start menu as usual: %v %v", f, m)
	}
}

func TestModifiedHotkey(t *testing.T) {
	f, m := run(t, "^!Space", dn(lctl), dn(lalt), dn(0x20), up(0x20), up(lalt), up(lctl))
	if !eq(f, 2) || len(m) != 0 {
		t.Errorf("^!Space: %v %v", f, m)
	}
	f, _ = run(t, "^!Space", dn(lctl), dn(0x20))
	if len(f) != 0 {
		t.Errorf("missing Alt: %v", f)
	}
	f, _ = run(t, "^!Space", dn(lctl), dn(lalt), dn(config.VKLShift), dn(0x20))
	if len(f) != 0 {
		t.Errorf("extra Shift: %v", f)
	}
	f, _ = run(t, "^!Space", dn(lctl), dn(lalt), dn(0x20), dn(0x20), dn(0x20))
	if !eq(f, 2) {
		t.Errorf("auto-repeat fired again: %v", f)
	}
}

func TestModifiedWinSides(t *testing.T) {
	f, m := run(t, "<#a", dn(lwin), dn('A'), up('A'), up(lwin))
	if !eq(f, 1) || !eq(m, 3) {
		t.Errorf("<#a: %v %v", f, m)
	}
	f, _ = run(t, "<#a", dn(config.VKRWin), dn('A'))
	if len(f) != 0 {
		t.Errorf("right Win for <#: %v", f)
	}
	f, _ = run(t, ">!x", dn(lalt), dn('X'))
	if len(f) != 0 {
		t.Errorf("left Alt for >!: %v", f)
	}
	f, _ = run(t, ">!x", dn(ralt), dn('X'))
	if !eq(f, 1) {
		t.Errorf(">!x: %v", f)
	}
}
