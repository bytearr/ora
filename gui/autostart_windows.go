package gui

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// AutostartArg is the one argument of the Run entry: start the window
// hidden, it waits for the hotkey.
const AutostartArg = "--autostart"

const (
	runKey   = `Software\Microsoft\Windows\CurrentVersion\Run`
	runValue = "ora"
)

// runCommand starts ora through a headless console host. ora.exe is a
// console program; started from the Run key directly, Windows would open
// a console or Windows Terminal for it at every sign-in. `--headless` is
// undocumented, conhost has it since Windows 10 1809.
func runCommand(systemDir, exe string) string {
	return fmt.Sprintf(`"%s" --headless "%s" %s`, filepath.Join(systemDir, "conhost.exe"), exe, AutostartArg)
}

// syncAutostart makes the Run entry match on: written or rewritten when
// the exe moved, removed when on is false.
func syncAutostart(on bool) error {
	k, err := registry.OpenKey(registry.CURRENT_USER, runKey, registry.QUERY_VALUE|registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	cur, _, err := k.GetStringValue(runValue)
	exists := !errors.Is(err, registry.ErrNotExist)
	if !on {
		if exists {
			return k.DeleteValue(runValue)
		}
		return nil
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	if p, err := filepath.EvalSymlinks(exe); err == nil {
		exe = p
	}
	sys, err := windows.GetSystemDirectory()
	if err != nil {
		return err
	}
	if want := runCommand(sys, exe); cur != want {
		return k.SetStringValue(runValue, want)
	}
	return nil
}
