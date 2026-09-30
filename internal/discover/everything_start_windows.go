package discover

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

const (
	everythingWMIPC        = 0x0400 // WM_USER
	everythingIsDBLoaded   = 401
	swShowMinNoActive      = 7
	everythingWindowWait   = 15 * time.Second
	everythingDBWait       = 60 * time.Second
	everythingPollInterval = 200 * time.Millisecond
)

var everythingExeNames = []string{"Everything.exe", "Everything64.exe"}

// everythingExe finds an installed Everything: App Paths, the uninstall
// entries (DisplayIcon / InstallLocation), the default install folders,
// Scoop and PATH.
func everythingExe() string {
	var candidates []string
	for _, n := range everythingExeNames {
		for _, root := range []registry.Key{registry.CURRENT_USER, registry.LOCAL_MACHINE} {
			if k, err := registry.OpenKey(root, `SOFTWARE\Microsoft\Windows\CurrentVersion\App Paths\`+n, registry.QUERY_VALUE); err == nil {
				candidates = append(candidates, strings.Trim(regString(k, ""), `"`))
				k.Close()
			}
		}
	}
	candidates = append(candidates, everythingFromUninstall()...)
	for _, env := range []string{"ProgramFiles", "ProgramW6432", "ProgramFiles(x86)", "LOCALAPPDATA"} {
		base := os.Getenv(env)
		if base == "" {
			continue
		}
		if env == "LOCALAPPDATA" {
			base = filepath.Join(base, "Programs")
		}
		for _, n := range everythingExeNames {
			candidates = append(candidates, filepath.Join(base, "Everything", n))
		}
	}
	if home, err := os.UserHomeDir(); err == nil {
		candidates = append(candidates, filepath.Join(home, `scoop\apps\everything\current\Everything.exe`))
	}
	for _, n := range everythingExeNames {
		if p, err := exec.LookPath(n); err == nil {
			candidates = append(candidates, p)
		}
	}
	for _, p := range candidates {
		if p != "" && fileExists(p) {
			return p
		}
	}
	return ""
}

func everythingFromUninstall() []string {
	var out []string
	for _, rt := range []struct {
		key  registry.Key
		view uint32
	}{
		{registry.LOCAL_MACHINE, registry.WOW64_64KEY},
		{registry.LOCAL_MACHINE, registry.WOW64_32KEY},
		{registry.CURRENT_USER, 0},
	} {
		k, err := registry.OpenKey(rt.key, uninstallKey, registry.ENUMERATE_SUB_KEYS|registry.READ|rt.view)
		if err != nil {
			continue
		}
		names, _ := k.ReadSubKeyNames(-1)
		for _, n := range names {
			if !strings.HasPrefix(strings.ToLower(n), "everything") {
				continue
			}
			sub, err := registry.OpenKey(k, n, registry.QUERY_VALUE|rt.view)
			if err != nil {
				continue
			}
			if icon := ParseDisplayIcon(regString(sub, "DisplayIcon")); isEverythingExe(icon) {
				out = append(out, icon)
			}
			if loc := strings.Trim(regString(sub, "InstallLocation"), `" `); loc != "" {
				for _, e := range everythingExeNames {
					out = append(out, filepath.Join(loc, e))
				}
			}
			sub.Close()
		}
		k.Close()
	}
	return out
}

func isEverythingExe(p string) bool {
	for _, n := range everythingExeNames {
		if strings.EqualFold(filepath.Base(p), n) {
			return true
		}
	}
	return false
}

// startEverything launches Everything in the background and waits until its
// IPC window exists and the database is loaded. ShellExecute (not
// CreateProcess) so an Everything configured to run as admin can prompt.
func startEverything(report Reporter) error {
	exe := everythingExe()
	if exe == "" {
		return errors.New("not running and not installed (https://www.voidtools.com)")
	}
	if err := windows.ShellExecute(0, windows.StringToUTF16Ptr("open"), windows.StringToUTF16Ptr(exe),
		windows.StringToUTF16Ptr("-startup"), windows.StringToUTF16Ptr(filepath.Dir(exe)), swShowMinNoActive); err != nil {
		return fmt.Errorf("not running, starting %s failed: %w", exe, err)
	}
	report("everything: was not running, started %s", exe)

	var hwnd uintptr
	for deadline := time.Now().Add(everythingWindowWait); hwnd == 0; hwnd = findEverything() {
		if time.Now().After(deadline) {
			return fmt.Errorf("started, but no Everything window after %s (UAC prompt declined?)", everythingWindowWait)
		}
		time.Sleep(everythingPollInterval)
	}
	told := false
	start := time.Now()
	for !everythingDBLoaded(hwnd) {
		if time.Since(start) > everythingDBWait {
			report("everything: database still loading after %s, results may be incomplete", everythingDBWait)
			return nil
		}
		if !told && time.Since(start) > time.Second {
			report("everything: waiting for the database to load...")
			told = true
		}
		time.Sleep(everythingPollInterval)
	}
	return nil
}

func everythingDBLoaded(hwnd uintptr) bool {
	var res uintptr
	r, _, _ := procSendMessageTimeoutW.Call(hwnd, everythingWMIPC, everythingIsDBLoaded, 0,
		smtoAbortIfHung, 1000, uintptr(unsafe.Pointer(&res)))
	return r != 0 && res == 1
}
