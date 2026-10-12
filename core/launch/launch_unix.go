//go:build !windows

package launch

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	"ora/core/discover"
	"ora/core/index"
)

// helperWait is how long a helper (gio, xdg-open, gdbus) may run before it
// counts as started; most hand off to the desktop and exit at once.
const helperWait = 3 * time.Second

type Shell struct {
	// FindAutoHotkey is unused outside Windows.
	FindAutoHotkey func() string
	Warn           func(format string, args ...any)
}

func (s Shell) warn(format string, args ...any) {
	if s.Warn != nil {
		s.Warn(format, args...)
	}
}

func (s Shell) Launch(e index.Entry, extra []string) error {
	switch e.Kind {
	case index.KindDesktop:
		return s.launchDesktop(e, extra)
	case index.KindFile, index.KindFolder:
		if len(extra) > 0 {
			s.warn("arguments are ignored when opening a file")
		}
		return openWithDefault(e.Target)
	default:
		dir := filepath.Dir(e.Target)
		if len(extra) > 0 {
			dir, _ = os.Getwd()
		}
		return spawn(dir, e.Target, extra...)
	}
}

// launchDesktop prefers `gio launch`, which knows D-Bus activation and
// startup notification; without gio the Exec line is run directly.
func (s Shell) launchDesktop(e index.Entry, extra []string) error {
	if gio, err := exec.LookPath("gio"); err == nil {
		args := append([]string{"launch", e.Target}, extra...)
		if err := runHelper(gio, args...); err == nil {
			return nil
		} else {
			s.warn("gio launch failed (%v), running Exec directly", err)
		}
	}
	data, err := os.ReadFile(e.Target)
	if err != nil {
		return err
	}
	df := discover.ParseDesktop(string(data), nil)
	argv, err := ExecArgs(discover.UnescapeDesktop(df.Exec), extra, e.Name, df.Icon, e.Target)
	if err != nil {
		return fmt.Errorf("%s: %w", e.Target, err)
	}
	if df.Terminal {
		term, err := terminal()
		if err != nil {
			return err
		}
		argv = append(term, argv...)
	}
	dir := df.Path
	if dir == "" {
		dir, _ = os.UserHomeDir()
	}
	return spawn(dir, argv[0], argv[1:]...)
}

// terminals are tried in order when an entry has Terminal=true; each
// carries the flag that starts the command after it.
var terminals = [][]string{
	{"xdg-terminal-exec"},
	{"x-terminal-emulator", "-e"},
	{"kgx", "--"},
	{"gnome-terminal", "--"},
	{"konsole", "-e"},
	{"ptyxis", "--"},
	{"xfce4-terminal", "-x"},
	{"tilix", "-e"},
	{"foot"},
	{"alacritty", "-e"},
	{"kitty"},
	{"wezterm", "start", "--"},
	{"xterm", "-e"},
}

func terminal() ([]string, error) {
	if t := os.Getenv("TERMINAL"); t != "" {
		if p, err := exec.LookPath(t); err == nil {
			return []string{p, "-e"}, nil
		}
	}
	for _, t := range terminals {
		if p, err := exec.LookPath(t[0]); err == nil {
			return append([]string{p}, t[1:]...), nil
		}
	}
	return nil, errors.New("Terminal=true but no terminal emulator found; set $TERMINAL")
}

// spawn starts a program in its own session, detached from ora's terminal.
func spawn(dir, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return err
	}
	go cmd.Wait()
	return nil
}

// runHelper runs a desktop helper and reports its failure if it exits
// within helperWait; one still running is taken as started.
func runHelper(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	var stderr limitedBuffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return err
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		if err != nil && stderr.Len() > 0 {
			return fmt.Errorf("%w: %s", err, stderr.String())
		}
		return err
	case <-time.After(helperWait):
		return nil
	}
}

type limitedBuffer struct{ b []byte }

func (l *limitedBuffer) Write(p []byte) (int, error) {
	if room := 512 - len(l.b); room > 0 {
		if len(p) < room {
			room = len(p)
		}
		l.b = append(l.b, p[:room]...)
	}
	return len(p), nil
}

func (l *limitedBuffer) Len() int { return len(l.b) }

func (l *limitedBuffer) String() string {
	b := l.b
	for len(b) > 0 && (b[len(b)-1] == '\n' || b[len(b)-1] == ' ') {
		b = b[:len(b)-1]
	}
	return string(b)
}

func openWithDefault(path string) error {
	if p, err := exec.LookPath("xdg-open"); err == nil {
		return runHelper(p, path)
	}
	if p, err := exec.LookPath("gio"); err == nil {
		return runHelper(p, "open", path)
	}
	return errors.New("neither xdg-open nor gio found")
}

// Reveal asks the file manager to select path (org.freedesktop.FileManager1,
// supported by Nautilus, Dolphin, Nemo, Thunar and others). Without it the
// containing folder is opened.
func Reveal(path string) error {
	uri := FileURI(path)
	if p, err := exec.LookPath("gdbus"); err == nil {
		if runHelper(p, "call", "--session",
			"--dest", "org.freedesktop.FileManager1",
			"--object-path", "/org/freedesktop/FileManager1",
			"--method", "org.freedesktop.FileManager1.ShowItems",
			"['"+uri+"']", "") == nil {
			return nil
		}
	}
	if p, err := exec.LookPath("dbus-send"); err == nil {
		if runHelper(p, "--session", "--print-reply", "--dest=org.freedesktop.FileManager1",
			"/org/freedesktop/FileManager1", "org.freedesktop.FileManager1.ShowItems",
			"array:string:"+uri, "string:") == nil {
			return nil
		}
	}
	return openWithDefault(filepath.Dir(path))
}

// OpenDir opens a folder in the default file manager.
func OpenDir(path string) error {
	return openWithDefault(path)
}
