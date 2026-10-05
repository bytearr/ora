package config

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

const (
	MonitorPrimary = "primary"
	MonitorCursor  = "cursor"
	ThemeDark      = "dark"
	ThemeLight     = "light"
)

// Template is written to config.yaml when the file does not exist.
const Template = `# ora window settings. Read once when the window starts;
# quit it with Ctrl+Q and run ora again after a change.
settings:
  # AutoHotkey v2 notation without "::".
  # Modifiers and a key: "^!Space", "<#a". Two keys: "LWin & LAlt".
  hotkey: "LWin & LAlt"
  # primary, or cursor for the monitor under the mouse pointer
  monitor: primary
  # dark or light
  theme: dark
  # true starts the window when you sign in to Windows
  autostart: false
`

// EnsureConfigFile writes Template to config.yaml unless the file exists.
// An existing file is never touched.
func EnsureConfigFile() (path string, created bool, err error) {
	path, err = ConfigFile()
	if err != nil {
		return "", false, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return path, false, err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if errors.Is(err, fs.ErrExist) {
		return path, false, nil
	}
	if err != nil {
		return path, false, err
	}
	if _, err := f.WriteString(Template); err != nil {
		f.Close()
		return path, true, err
	}
	return path, true, f.Close()
}

// CheckSettings returns usable settings and the parsed hotkey. An invalid
// value is reported through warn and replaced by its default.
func CheckSettings(s Settings, warn func(format string, args ...any)) (Settings, Hotkey) {
	hk, err := ParseHotkey(s.Hotkey)
	if err != nil {
		warn("settings.hotkey %q: %v; using %q", s.Hotkey, err, DefaultHotkey)
		hk, _ = ParseHotkey(DefaultHotkey)
		s.Hotkey = DefaultHotkey
	}
	switch m := strings.ToLower(strings.TrimSpace(s.Monitor)); m {
	case MonitorPrimary, MonitorCursor:
		s.Monitor = m
	default:
		warn("settings.monitor %q is not primary or cursor; using primary", s.Monitor)
		s.Monitor = MonitorPrimary
	}
	switch t := strings.ToLower(strings.TrimSpace(s.Theme)); t {
	case ThemeDark, ThemeLight:
		s.Theme = t
	default:
		warn("settings.theme %q is not dark or light; using dark", s.Theme)
		s.Theme = ThemeDark
	}
	return s, hk
}
