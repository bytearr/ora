//go:build linux

package gui

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/viper"

	"ora/core/config"
)

// AutostartArg is the one argument of the autostart entry: start the
// window hidden, it waits for the hotkey.
const AutostartArg = "--autostart"

// appID names the window on D-Bus, in the portals and for the desktop.
const appID = "io.github.bytearr.ora"

// execQuote quotes one Exec argument as the desktop entry spec asks.
func execQuote(s string) string {
	if !strings.ContainsAny(s, " \t\n\"'\\><~|&;$*?#()`") {
		return s
	}
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "`", "\\`", `$`, `\$`)
	return `"` + r.Replace(s) + `"`
}

// desktopEscape escapes a string value: Exec is a string, so its
// backslashes are escaped once more.
func desktopEscape(s string) string {
	return strings.ReplaceAll(s, `\`, `\\`)
}

func autostartFile() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "autostart", appID+".desktop"), nil
}

func appFile() (string, error) {
	dir := os.Getenv("XDG_DATA_HOME")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		dir = filepath.Join(home, ".local", "share")
	}
	return filepath.Join(dir, "applications", appID+".desktop"), nil
}

func autostartEntry(exe string) string {
	return "[Desktop Entry]\nType=Application\nName=ora\nComment=Start the ora window hidden; the hotkey shows it\n" +
		"Exec=" + desktopEscape(execQuote(exe)+" "+AutostartArg) + "\nTerminal=false\nNoDisplay=true\nX-GNOME-Autostart-enabled=true\n"
}

func appEntry(exe string) string {
	return "[Desktop Entry]\nType=Application\nName=ora\nComment=Start an app or open a file by a short name\n" +
		"Exec=" + desktopEscape(execQuote(exe)) + "\nIcon=system-search\nTerminal=false\nCategories=Utility;\n" +
		"StartupNotify=true\nSingleMainWindow=true\n"
}

// writeIfChanged writes data to path unless it already holds exactly that.
func writeIfChanged(path, data string) error {
	if cur, err := os.ReadFile(path); err == nil && bytes.Equal(cur, []byte(data)) {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(data), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func executable() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	if p, err := filepath.EvalSymlinks(exe); err == nil {
		exe = p
	}
	return exe, nil
}

// syncAutostart makes the XDG autostart entry match on: written or
// rewritten when the binary moved, removed when on is false.
func syncAutostart(on bool) error {
	path, err := autostartFile()
	if err != nil {
		return err
	}
	if !on {
		if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		return nil
	}
	exe, err := executable()
	if err != nil {
		return err
	}
	return writeIfChanged(path, autostartEntry(exe))
}

// syncAppEntry keeps a desktop entry for the window, so the desktop and
// its portals know the app id (window icon, shortcut dialogs).
func syncAppEntry() error {
	path, err := appFile()
	if err != nil {
		return err
	}
	exe, err := executable()
	if err != nil {
		return err
	}
	return writeIfChanged(path, appEntry(exe))
}

// refreshAutostart re-reads settings.autostart, the one key read again
// while the window runs. Runs off the UI thread.
func refreshAutostart(logf func(string, ...any)) {
	cfg, err := config.Load(viper.New())
	if err != nil {
		logf("autostart: config: %v", err)
		return
	}
	if err := syncAutostart(cfg.Settings.Autostart); err != nil {
		logf("autostart: %v", err)
	}
}

// logger appends to gui.log in the cache folder; the window has no console.
type logger struct{ file string }

func newLogger() *logger {
	dir, err := config.CacheDir()
	if err != nil {
		return &logger{}
	}
	return &logger{file: filepath.Join(dir, "gui.log")}
}

func (l *logger) printf(format string, args ...any) {
	if l.file == "" {
		return
	}
	_ = os.MkdirAll(filepath.Dir(l.file), 0o755)
	f, err := os.OpenFile(l.file, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "%s %s\n", time.Now().Format("2006-01-02 15:04:05"), strings.TrimRight(fmt.Sprintf(format, args...), "\n"))
}
