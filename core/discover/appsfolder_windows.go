package discover

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"

	"ora/core/index"
	"ora/core/win"
)

var (
	shell32                         = windows.NewLazySystemDLL("shell32.dll")
	procSHCreateItemFromParsingName = shell32.NewProc("SHCreateItemFromParsingName")

	iidShellItem      = win.MustGUID("{43826D1E-E718-42EE-BC55-A1E261C37BFE}")
	iidEnumShellItems = win.MustGUID("{70629033-E363-4A28-A567-0DB78006E6D7}")
	bhidEnumItems     = win.MustGUID("{94F60519-2850-4924-AA5A-D15E84868039}")
)

// IShellItem / IEnumShellItems slots.
const (
	siBindToHandler  = 3
	siGetDisplayName = 5
	esiNext          = 3

	sigdnNormalDisplay         = 0x00000000
	sigdnParentRelativeParsing = 0x80018001
)

type AppsFolder struct{}

func (AppsFolder) Name() index.Source { return index.SourceAppsFolder }

func (a AppsFolder) Discover(report Reporter) ([]index.Entry, error) {
	var out []index.Entry
	err := win.WithCOM(func() error {
		var err error
		out, err = enumAppsFolder()
		return err
	})
	if err == nil {
		return out, nil
	}
	report("appsfolder: COM failed (%v), using Get-StartApps fallback", err)
	return startAppsFallback()
}

func enumAppsFolder() ([]index.Entry, error) {
	p16, _ := windows.UTF16PtrFromString("shell:AppsFolder")
	var folder win.Obj
	r, _, _ := procSHCreateItemFromParsingName.Call(uintptr(unsafe.Pointer(p16)), 0,
		uintptr(unsafe.Pointer(&iidShellItem)), uintptr(unsafe.Pointer(&folder)))
	if err := win.HRESULT(r).Err(); err != nil {
		return nil, fmt.Errorf("SHCreateItemFromParsingName: %w", err)
	}
	defer win.Release(folder)

	var enum win.Obj
	if err := win.Call(folder, siBindToHandler, 0, uintptr(unsafe.Pointer(&bhidEnumItems)),
		uintptr(unsafe.Pointer(&iidEnumShellItems)), uintptr(unsafe.Pointer(&enum))).Err(); err != nil {
		return nil, fmt.Errorf("BindToHandler: %w", err)
	}
	defer win.Release(enum)

	var out []index.Entry
	for {
		var item win.Obj
		var fetched uint32
		hr := win.Call(enum, esiNext, 1, uintptr(unsafe.Pointer(&item)), uintptr(unsafe.Pointer(&fetched)))
		if hr != 0 || fetched == 0 || item == nil {
			if hr.Failed() {
				return nil, fmt.Errorf("IEnumShellItems.Next: %w", hr)
			}
			break
		}
		name := itemName(item, sigdnNormalDisplay)
		aumid := itemName(item, sigdnParentRelativeParsing)
		win.Release(item)
		if name != "" && IsPackagedAUMID(aumid) {
			out = append(out, storeEntry(name, aumid))
		}
	}
	return out, nil
}

func itemName(item win.Obj, sigdn uint32) string {
	var p *uint16
	if win.Call(item, siGetDisplayName, uintptr(sigdn), uintptr(unsafe.Pointer(&p))).Failed() {
		return ""
	}
	return win.CoTaskString(p)
}

func storeEntry(name, aumid string) index.Entry {
	return index.Entry{Name: name, Kind: index.KindStore, Source: index.SourceAppsFolder, Target: aumid, AUMID: aumid}
}

type startApp struct {
	Name  string `json:"Name"`
	AppID string `json:"AppID"`
}

func startAppsFallback() ([]index.Entry, error) {
	cmd := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-Command",
		"[Console]::OutputEncoding=[Text.Encoding]::UTF8; Get-StartApps | ConvertTo-Json -Compress")
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	data, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("Get-StartApps: %w", err)
	}
	apps, err := parseStartApps(data)
	if err != nil {
		return nil, err
	}
	var out []index.Entry
	for _, a := range apps {
		if a.Name != "" && IsPackagedAUMID(a.AppID) {
			out = append(out, storeEntry(a.Name, a.AppID))
		}
	}
	return out, nil
}

// parseStartApps accepts both a JSON array and a single object, which is
// what ConvertTo-Json emits for one result.
func parseStartApps(data []byte) ([]startApp, error) {
	s := strings.TrimSpace(strings.TrimPrefix(string(data), "\ufeff"))
	if s == "" {
		return nil, nil
	}
	var apps []startApp
	if strings.HasPrefix(s, "{") {
		var one startApp
		if err := json.Unmarshal([]byte(s), &one); err != nil {
			return nil, err
		}
		return []startApp{one}, nil
	}
	if err := json.Unmarshal([]byte(s), &apps); err != nil {
		return nil, err
	}
	return apps, nil
}
