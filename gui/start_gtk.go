//go:build linux && cgo && !nogui

package gui

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"runtime/debug"
	"syscall"

	"github.com/diamondburned/gotk4/pkg/gio/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

const (
	envChild    = "ORA_GUI"
	childHidden = "hidden"
)

// Run is `ora` without arguments. From a terminal it hands over to a
// running window (which shows or hides itself) or starts a detached one
// and returns; in that detached child it runs the window until Ctrl+Q.
// hidden is the autostart: start the window without showing it, and leave
// a running one alone. So `ora` is also what a desktop shortcut runs.
func Run(hidden bool) error {
	if v := os.Getenv(envChild); v == "1" || v == childHidden {
		os.Unsetenv(envChild) // apps started from the window must not inherit it
		return runApp(v == childHidden)
	}
	if os.Getenv("WAYLAND_DISPLAY") == "" && os.Getenv("DISPLAY") == "" {
		return errors.New("no graphical session (neither WAYLAND_DISPLAY nor DISPLAY is set); see ora --help for the commands")
	}
	if running, err := activateRunning(hidden); running {
		return err
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	cmd := exec.Command(exe)
	mode := "1"
	if hidden {
		mode = childHidden
	}
	cmd.Env = append(os.Environ(), envChild+"="+mode)
	// The window lives for the whole session; in the terminal's directory
	// it would keep that directory busy.
	if home, err := os.UserHomeDir(); err == nil {
		cmd.Dir = home
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true} // survives the terminal
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start the window: %w", err)
	}
	return cmd.Process.Release()
}

func runApp(hidden bool) (err error) {
	runtime.LockOSThread()
	log := newLogger()
	w := &window{log: log, startHidden: hidden}
	defer func() {
		if r := recover(); r != nil {
			log.printf("panic: %v\n%s", r, debug.Stack())
			err = fmt.Errorf("the window crashed: %v; details in %s", r, log.file)
		}
	}()
	w.app = gtk.NewApplication(appID, gio.ApplicationFlagsNone)
	w.app.ConnectActivate(w.activate)
	if err := w.app.Register(context.Background()); err != nil {
		log.printf("start failed: register %s: %v", appID, err)
		return err
	}
	if w.app.IsRemote() { // another window won the race
		if !hidden {
			w.app.Activate()
		}
		return nil
	}
	if code := w.app.Run([]string{os.Args[0]}); code != 0 || w.fatal != nil {
		if w.fatal != nil {
			return w.fatal
		}
		return fmt.Errorf("the window ended with code %d; details in %s", code, log.file)
	}
	return nil
}
