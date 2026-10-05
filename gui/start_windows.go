package gui

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"ora/core/config"
	"ora/core/engine"
)

// envChild marks the detached process that owns the window. Without it,
// `ora` would start itself again and again. "1" shows the window at start,
// childHidden only installs the hotkey.
const (
	envChild    = "ORA_GUI"
	childHidden = "hidden"
)

const frontTimeout = 2 * time.Second

// Run is `ora` without arguments. From a terminal it hands over to a
// running window or starts a detached one and returns; in that detached
// child it runs the window until Ctrl+Q. hidden is the autostart: start
// the window without showing it, and leave a running one alone.
func Run(hidden bool) error {
	if v := os.Getenv(envChild); v == "1" || v == childHidden {
		os.Unsetenv(envChild) // apps started from the window must not inherit it
		return runWindow(v == childHidden)
	}
	if h := findWindow(); h != 0 {
		if hidden {
			return nil
		}
		return showRunning(h)
	}
	if mutexExists() {
		if hidden {
			return nil
		}
		h := waitWindow(frontTimeout)
		if h == 0 {
			return errors.New("ora is starting, but its window did not appear within 2 s")
		}
		return showRunning(h)
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
	// it would keep that directory from being renamed or deleted.
	if home, err := os.UserHomeDir(); err == nil {
		cmd.Dir = home
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.DETACHED_PROCESS | windows.CREATE_NEW_PROCESS_GROUP}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start the window: %w", err)
	}
	procAllowSetForegroundWindow.Call(uintptr(cmd.Process.Pid))
	return cmd.Process.Release()
}

func findWindow() uintptr {
	h, _, _ := procFindWindowW.Call(uintptr(unsafe.Pointer(utf16(className))), 0)
	return h
}

func waitWindow(d time.Duration) uintptr {
	for end := time.Now().Add(d); ; time.Sleep(50 * time.Millisecond) {
		if h := findWindow(); h != 0 || time.Now().After(end) {
			return h
		}
	}
}

func mutexExists() bool {
	h, _, _ := procOpenMutexW.Call(synchronize, 0, uintptr(unsafe.Pointer(utf16(mutexName))))
	if h == 0 {
		return false
	}
	windows.CloseHandle(windows.Handle(h))
	return true
}

// showRunning lets the running window take the foreground and asks it to
// show itself. It waits until it is in front, not just in the taskbar.
func showRunning(h uintptr) error {
	var pid uint32
	procGetWindowThreadProcessId.Call(h, uintptr(unsafe.Pointer(&pid)))
	procAllowSetForegroundWindow.Call(uintptr(pid))
	procPostMessageW.Call(h, wmShow, 0, 0)
	for end := time.Now().Add(frontTimeout); time.Now().Before(end); time.Sleep(50 * time.Millisecond) {
		fg, _, _ := procGetForegroundWindow.Call()
		if v, _, _ := procIsWindowVisible.Call(h); v != 0 && fg == h {
			return nil
		}
	}
	return errors.New("the ora window did not come to the front within 2 s")
}

func runWindow(hidden bool) (err error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	log := newLogger()

	mutex, merr := windows.CreateMutex(nil, false, utf16(mutexName))
	if errors.Is(merr, windows.ERROR_ALREADY_EXISTS) {
		windows.CloseHandle(mutex)
		if h := waitWindow(frontTimeout); h != 0 && !hidden {
			procPostMessageW.Call(h, wmShow, 0, 0)
		}
		return nil
	}
	if merr != nil {
		return fatal(log, fmt.Errorf("mutex %s: %w", mutexName, merr))
	}
	// The mutex stays open until the process ends.

	procSetProcessDPIAware.Call()
	w := &window{log: log, startHidden: hidden}
	defer func() {
		if r := recover(); r != nil {
			uninstallHook()
			err = crash(log, r, debug.Stack())
		}
	}()
	return w.run()
}

func (w *window) run() error {
	var warns []string
	warn := func(format string, args ...any) {
		s := fmt.Sprintf(format, args...)
		warns = append(warns, s)
		w.log.printf("warning: %s", s)
	}
	if path, created, err := config.EnsureConfigFile(); err != nil {
		warn("config: %v", err)
	} else if created {
		w.log.printf("created %s", path)
	}
	eng, err := engine.Open(func(format string, args ...any) { w.log.printf(format, args...) })
	if err != nil {
		return fatal(w.log, err)
	}
	w.eng = eng
	w.settings, w.hk = config.CheckSettings(eng.Cfg.Settings, warn)
	if err := syncAutostart(w.settings.Autostart); err != nil {
		warn("autostart: %v", err)
	}
	w.status = warns

	if err := w.create(); err != nil {
		return fatal(w.log, err)
	}
	if err := installHook(w.hwnd, w.hk, w.hinst); err != nil {
		procDestroyWindow.Call(w.hwnd)
		return fatal(w.log, fmt.Errorf("keyboard hook: %w", err))
	}
	defer uninstallHook()
	w.startWorkers()
	if !w.startHidden {
		w.show()
	}
	w.loop()
	w.freeIcons()
	if w.panicVal != nil {
		uninstallHook()
		return crash(w.log, w.panicVal, w.panicStack)
	}
	return nil
}

// fatal reports a start error: log and a message box, there is no
// console and maybe no window yet.
func fatal(log *logger, err error) error {
	log.printf("start failed: %v", err)
	windows.MessageBox(0, utf16(fmt.Sprintf("ora could not start:\n\n%v\n\nDetails: %s", err, log.path())), utf16("ora"), windows.MB_OK|windows.MB_ICONERROR)
	return err
}

func crash(log *logger, r any, stack []byte) error {
	log.printf("panic: %v\n%s", r, stack)
	windows.MessageBox(0, utf16(fmt.Sprintf("ora stopped after an internal error:\n\n%v\n\nDetails: %s", r, log.path())), utf16("ora"), windows.MB_OK|windows.MB_ICONERROR)
	return fmt.Errorf("panic: %v", r)
}

// logger appends to %LOCALAPPDATA%\ora\gui.log. Safe for the workers.
type logger struct {
	mu   sync.Mutex
	file string
}

func newLogger() *logger {
	l := &logger{}
	if d, err := config.CacheDir(); err == nil {
		l.file = filepath.Join(d, "gui.log")
	}
	return l
}

func (l *logger) path() string {
	if l.file == "" {
		return "%LOCALAPPDATA%\\ora\\gui.log"
	}
	return l.file
}

func (l *logger) printf(format string, args ...any) {
	if l.file == "" {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if err := os.MkdirAll(filepath.Dir(l.file), 0o755); err != nil {
		return
	}
	f, err := os.OpenFile(l.file, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "%s %s\n", time.Now().Format("2006-01-02 15:04:05"), fmt.Sprintf(format, args...))
}
