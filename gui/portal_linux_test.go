//go:build linux

package gui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/godbus/dbus/v5"

	"ora/core/config"
)

func TestShortcutTrigger(t *testing.T) {
	for _, c := range []struct{ hotkey, want string }{
		{"^!Space", "CTRL+ALT+space"},
		{"#Space", "LOGO+space"},
		{"+#F12", "SHIFT+LOGO+F12"},
		{"!a", "ALT+a"},
		{"^Numpad3", "CTRL+KP_3"},
		{"LWin & LAlt", ""}, // a combo has no XDG form; the desktop asks
	} {
		h, err := config.ParseHotkey(c.hotkey)
		if err != nil {
			t.Fatalf("%s: %v", c.hotkey, err)
		}
		if got := shortcutTrigger(h); got != c.want {
			t.Errorf("%s: %q, want %q", c.hotkey, got, c.want)
		}
	}
}

func TestRGBHex(t *testing.T) {
	if got := rgbHex([]any{0.2, 0.5, 1.0}); got != "#3380ff" {
		t.Fatalf("got %q", got)
	}
	if got := rgbHex(dbus.MakeVariant([]any{0.0, 0.0, 0.0})); got != "#000000" {
		t.Fatalf("variant: %q", got)
	}
	for _, v := range []any{[]any{-1.0, 0.0, 0.0}, []any{0.1, 0.2}, "x", nil} {
		if got := rgbHex(v); got != "" {
			t.Fatalf("%v: %q, want none", v, got)
		}
	}
}

func TestBoundTrigger(t *testing.T) {
	res := map[string]dbus.Variant{"shortcuts": dbus.MakeVariant([][]any{
		{"other", map[string]dbus.Variant{"trigger_description": dbus.MakeVariant("x")}},
		{toggleID, map[string]dbus.Variant{"trigger_description": dbus.MakeVariant("Ctrl+Alt+Space")}},
	})}
	if got := boundTrigger(res); got != "Ctrl+Alt+Space" {
		t.Fatalf("got %q", got)
	}
	if got := boundTrigger(map[string]dbus.Variant{}); got != "" {
		t.Fatalf("empty: %q", got)
	}
}

func TestExecQuote(t *testing.T) {
	for in, want := range map[string]string{
		"/usr/bin/ora":           "/usr/bin/ora",
		"/home/a b/ora":          `"/home/a b/ora"`,
		`/home/x$y/o"r\a`:        `"/home/x\$y/o\"r\\a"`,
		"/opt/ora (beta)/ora":    `"/opt/ora (beta)/ora"`,
		"/home/u/go/bin/ora-dev": "/home/u/go/bin/ora-dev",
	} {
		if got := execQuote(in); got != want {
			t.Errorf("%s: %s, want %s", in, got, want)
		}
	}
	// In the file the string escapes double the backslashes once more.
	if !strings.Contains(autostartEntry(`/a\b c/ora`), `Exec="/a\\\\b c/ora" --autostart`+"\n") {
		t.Fatalf("entry:\n%s", autostartEntry(`/a\b c/ora`))
	}
}

func TestSyncAutostart(t *testing.T) {
	cfg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", cfg)
	path := filepath.Join(cfg, "autostart", appID+".desktop")
	if err := syncAutostart(true); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	exe, _ := executable()
	if !strings.Contains(string(data), "Exec="+desktopEscape(execQuote(exe))+" --autostart\n") {
		t.Fatalf("autostart entry:\n%s", data)
	}
	st1, _ := os.Stat(path)
	if err := syncAutostart(true); err != nil {
		t.Fatal(err)
	}
	if st2, _ := os.Stat(path); !st2.ModTime().Equal(st1.ModTime()) {
		t.Fatal("an unchanged entry was rewritten")
	}
	if err := syncAutostart(false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("entry still there: %v", err)
	}
	if err := syncAutostart(false); err != nil {
		t.Fatalf("removing twice: %v", err)
	}
}

func TestSyncAppEntry(t *testing.T) {
	data := t.TempDir()
	t.Setenv("XDG_DATA_HOME", data)
	if err := syncAppEntry(); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(data, "applications", appID+".desktop"))
	if err != nil || !strings.Contains(string(b), "Name=ora\n") {
		t.Fatalf("app entry: %v\n%s", err, b)
	}
}
