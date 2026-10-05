package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/viper"
)

func TestParseHotkeyDefault(t *testing.T) {
	h, err := ParseHotkey(DefaultHotkey)
	if err != nil {
		t.Fatal(err)
	}
	if !h.IsCombo() || !h.HasWin() {
		t.Fatalf("%+v", h)
	}
	if !h.Combo[0].Has(VKLWin) || h.Combo[0].Has(VKRWin) {
		t.Errorf("first key %v", h.Combo[0])
	}
	if !h.Combo[1].Has(VKLAlt) || h.Combo[1].Has(VKRAlt) {
		t.Errorf("RAlt must not trigger the default: %v", h.Combo[1])
	}
}

func TestParseHotkeyModified(t *testing.T) {
	h, err := ParseHotkey("^!Space")
	if err != nil {
		t.Fatal(err)
	}
	if h.Ctrl != SideAny || h.Alt != SideAny || h.Win != SideNone || h.Shift != SideNone || h.Trigger != 0x20 || h.HasWin() {
		t.Errorf("^!Space = %+v", h)
	}
	h, err = ParseHotkey("<#a")
	if err != nil {
		t.Fatal(err)
	}
	if h.Win != SideLeft || h.Trigger != 'A' || !h.HasWin() {
		t.Errorf("<#a = %+v", h)
	}
	h, err = ParseHotkey(">!F12")
	if err != nil || h.Alt != SideRight || h.Trigger != 0x7B {
		t.Errorf(">!F12 = %+v %v", h, err)
	}
	if h, err := ParseHotkey("^Up"); err != nil || h.Trigger != 0x26 {
		t.Errorf("^Up is the arrow key: %+v %v", h, err)
	}
	if h, err := ParseHotkey("RAlt & j"); err != nil || !h.Combo[0].Has(VKRAlt) || !h.Combo[1].Has('J') {
		t.Errorf("RAlt & j = %+v %v", h, err)
	}
	if h, err := ParseHotkey("Alt & Space"); err != nil || !h.Combo[0].Has(VKLAlt) || !h.Combo[0].Has(VKRAlt) {
		t.Errorf("Alt & Space = %+v %v", h, err)
	}
	if _, err := ParseHotkey("^ScrollLock"); err != nil {
		t.Errorf("ScrollLock: %v", err)
	}
}

func TestParseHotkeyInvalid(t *testing.T) {
	for _, s := range []string{
		"", "a", "LWin", "Space",
		"^LButton", "LButton & a", "^WheelUp",
		"<^>!a", "~^a", "$^a", "*^a",
		"^a Up", "LWin & LAlt up",
		"SC029 & a", "^vk41",
		"#LWin & LAlt", "^a & b", "a & b & c", "a &", "LAlt & Alt",
		"^^a", "<a", "#<", "^!Nope",
	} {
		if h, err := ParseHotkey(s); err == nil {
			t.Errorf("%q must be invalid, got %+v", s, h)
		}
	}
}

func TestCheckSettingsFallbacks(t *testing.T) {
	var warns []string
	warn := func(f string, a ...any) { warns = append(warns, f) }
	s, h := CheckSettings(Settings{Hotkey: "a", Monitor: "2", Theme: "blue"}, warn)
	if len(warns) != 3 {
		t.Fatalf("warnings %v", warns)
	}
	if s.Hotkey != DefaultHotkey || !h.IsCombo() || !h.Combo[0].Has(VKLWin) {
		t.Errorf("hotkey fallback %+v %+v", s, h)
	}
	if s.Monitor != MonitorPrimary || s.Theme != ThemeDark {
		t.Errorf("fallbacks %+v", s)
	}
	warns = nil
	s, _ = CheckSettings(Settings{Hotkey: "^!Space", Monitor: "Cursor", Theme: " light "}, warn)
	if len(warns) != 0 || s.Monitor != MonitorCursor || s.Theme != ThemeLight {
		t.Errorf("%+v %v", s, warns)
	}
}

func TestEnsureConfigFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("APPDATA", dir)
	path, created, err := EnsureConfigFile()
	if err != nil || !created || path != filepath.Join(dir, "ora", "config.yaml") {
		t.Fatalf("%s %v %v", path, created, err)
	}
	if err := os.WriteFile(path, []byte("settings:\n  theme: light\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, created, err := EnsureConfigFile(); err != nil || created {
		t.Fatalf("second start: created=%v %v", created, err)
	}
	c, err := Load(viper.New())
	if err != nil {
		t.Fatal(err)
	}
	if c.Settings.Theme != ThemeLight || c.Settings.Hotkey != DefaultHotkey || c.Settings.Monitor != MonitorPrimary {
		t.Errorf("existing file kept, missing keys from defaults: %+v", c.Settings)
	}
}

func TestTemplate(t *testing.T) {
	for _, k := range []string{"min_score", "ignore", "everything", "Settings:", "main"} {
		if strings.Contains(Template, k) {
			t.Errorf("template must not contain %q", k)
		}
	}
	dir := t.TempDir()
	t.Setenv("APPDATA", dir)
	if _, _, err := EnsureConfigFile(); err != nil {
		t.Fatal(err)
	}
	c, err := Load(viper.New())
	if err != nil {
		t.Fatal(err)
	}
	if c.Settings != (Settings{Hotkey: DefaultHotkey, Monitor: MonitorPrimary, Theme: ThemeDark}) {
		t.Errorf("%+v", c.Settings)
	}
	if !strings.Contains(Template, `hotkey: "LWin & LAlt"`) {
		t.Error("hotkey value must be quoted")
	}
}
