package launch

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"unsafe"

	"golang.org/x/sys/windows"

	"ora/internal/index"
	"ora/internal/win"
)

var (
	shell32            = windows.NewLazySystemDLL("shell32.dll")
	procShellExecuteEx = shell32.NewProc("ShellExecuteExW")

	clsidAppActivationManager = win.MustGUID("{45BA127D-10A8-46EA-8AB7-56EA9078943C}")
	iidAppActivationManager   = win.MustGUID("{2E941141-7F97-4756-BA1D-9DECDE894A3D}")
)

const (
	seeMaskNoAsync   = 0x00000100
	seeMaskFlagNoUI  = 0x00000400
	swShowNormal     = 1
	aamActivate      = 3 // IApplicationActivationManager::ActivateApplication
	errNoAssociation = windows.Errno(1155)
)

type shellExecuteInfo struct {
	cbSize       uint32
	fMask        uint32
	hwnd         uintptr
	lpVerb       *uint16
	lpFile       *uint16
	lpParameters *uint16
	lpDirectory  *uint16
	nShow        int32
	hInstApp     uintptr
	lpIDList     uintptr
	lpClass      *uint16
	hkeyClass    uintptr
	dwHotKey     uint32
	hIcon        uintptr
	hProcess     uintptr
}

type Shell struct {
	// FindAutoHotkey returns an AutoHotkey interpreter path, or "".
	FindAutoHotkey func() string
	Warn           func(format string, args ...any)
}

func (s Shell) Launch(e index.Entry, extra []string) error {
	cwd, _ := os.Getwd()
	p, warn := PlanFor(e, extra, cwd, windows.EscapeArg)
	if warn != "" && s.Warn != nil {
		s.Warn("%s", warn)
	}
	return win.WithCOM(func() error {
		if p.Store {
			return activate(p.File)
		}
		err := shellExecute(p.File, p.Params, p.Dir)
		if p.AHK && errors.Is(err, errNoAssociation) {
			ahk := ""
			if s.FindAutoHotkey != nil {
				ahk = s.FindAutoHotkey()
			}
			if ahk == "" {
				return fmt.Errorf("%s: no .ahk association and no AutoHotkey found", p.File)
			}
			params := windows.EscapeArg(p.File)
			if p.Params != "" {
				params += " " + p.Params
			}
			return shellExecute(ahk, params, p.Dir)
		}
		return err
	})
}

func shellExecute(file, params, dir string) error {
	f, err := windows.UTF16PtrFromString(file)
	if err != nil {
		return err
	}
	info := shellExecuteInfo{
		fMask:  seeMaskNoAsync | seeMaskFlagNoUI,
		lpFile: f,
		nShow:  swShowNormal,
	}
	info.cbSize = uint32(unsafe.Sizeof(info))
	if params != "" {
		if info.lpParameters, err = windows.UTF16PtrFromString(params); err != nil {
			return err
		}
	}
	if dir != "" {
		if info.lpDirectory, err = windows.UTF16PtrFromString(dir); err != nil {
			return err
		}
	}
	if r, _, e := procShellExecuteEx.Call(uintptr(unsafe.Pointer(&info))); r == 0 {
		return fmt.Errorf("ShellExecuteEx %s: %w", file, e)
	}
	return nil
}

func activate(aumid string) error {
	err := activateCOM(aumid)
	if err == nil {
		return nil
	}
	cmd := exec.Command("explorer.exe", `shell:AppsFolder\`+aumid)
	if err2 := cmd.Start(); err2 != nil {
		return fmt.Errorf("ActivateApplication: %v; explorer fallback: %w", err, err2)
	}
	go cmd.Wait()
	return nil
}

func activateCOM(aumid string) error {
	mgr, err := win.CreateInstance(&clsidAppActivationManager, &iidAppActivationManager, win.CLSCTX_LOCAL_SERVER)
	if err != nil {
		return err
	}
	defer win.Release(mgr)
	a, err := windows.UTF16PtrFromString(aumid)
	if err != nil {
		return err
	}
	var pid uint32
	return win.Call(mgr, aamActivate, uintptr(unsafe.Pointer(a)), 0, 0, uintptr(unsafe.Pointer(&pid))).Err()
}
