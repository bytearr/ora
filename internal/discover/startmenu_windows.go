package discover

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"

	"ora/internal/index"
	"ora/internal/win"
)

var (
	clsidShellLink    = win.MustGUID("{00021401-0000-0000-C000-000000000046}")
	iidShellLinkW     = win.MustGUID("{000214F9-0000-0000-C000-000000000046}")
	iidPersistFile    = win.MustGUID("{0000010B-0000-0000-C000-000000000046}")
	iidPropertyStore  = win.MustGUID("{886D8EEB-8CF2-4446-8D02-CDBA1DBDCF99}")
	pkeyAppUserModeID = win.PropertyKey{FmtID: win.MustGUID("{9F4C2855-9F79-4B39-A8D0-E1D42DE1D5F3}"), PID: 5}
)

// IShellLinkW slots.
const (
	slGetPath             = 3
	slGetWorkingDirectory = 8
	slGetArguments        = 10
)

// IPersistFile / IPropertyStore slots.
const (
	pfLoad     = 5
	psGetValue = 5
)

type StartMenu struct{}

func (StartMenu) Name() index.Source { return index.SourceStartMenu }

func (StartMenu) Discover(report Reporter) ([]index.Entry, error) {
	// Known folders follow redirection (GPO, roaming profiles); the env
	// paths are the fallback.
	var roots []string
	for _, f := range []struct {
		id  *windows.KNOWNFOLDERID
		env string
	}{
		{windows.FOLDERID_CommonPrograms, "ProgramData"},
		{windows.FOLDERID_Programs, "APPDATA"},
	} {
		if p, err := windows.KnownFolderPath(f.id, 0); err == nil && p != "" {
			roots = append(roots, p)
		} else if base := os.Getenv(f.env); base != "" {
			roots = append(roots, filepath.Join(base, `Microsoft\Windows\Start Menu\Programs`))
		}
	}
	var lnks []string
	for _, root := range roots {
		filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if err == nil && !d.IsDir() && strings.EqualFold(filepath.Ext(p), ".lnk") {
				lnks = append(lnks, p)
			}
			return nil
		})
	}

	var out []index.Entry
	failed := 0
	var firstErr error
	err := win.WithCOM(func() error {
		for _, p := range lnks {
			e, err := readShortcut(p)
			if err != nil {
				if failed == 0 {
					firstErr = fmt.Errorf("%s: %w", p, err)
				}
				failed++
				continue
			}
			out = append(out, e)
		}
		return nil
	})
	if failed > 0 {
		report("startmenu: %d of %d shortcuts unreadable (first: %v)", failed, len(lnks), firstErr)
	}
	return out, err
}

func readShortcut(path string) (index.Entry, error) {
	link, err := win.CreateInstance(&clsidShellLink, &iidShellLinkW, win.CLSCTX_INPROC_SERVER)
	if err != nil {
		return index.Entry{}, err
	}
	defer win.Release(link)

	pf, err := win.QueryInterface(link, &iidPersistFile)
	if err != nil {
		return index.Entry{}, err
	}
	defer win.Release(pf)
	p16, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return index.Entry{}, err
	}
	if err := win.Call(pf, pfLoad, uintptr(unsafe.Pointer(p16)), 0 /* STGM_READ */).Err(); err != nil {
		return index.Entry{}, err
	}

	target := linkString(link, slGetPath, true)
	args := linkString(link, slGetArguments, false)
	wd := linkString(link, slGetWorkingDirectory, false)

	var aumid string
	if ps, err := win.QueryInterface(link, &iidPropertyStore); err == nil {
		var pv win.PropVariant
		if win.Call(ps, psGetValue, uintptr(unsafe.Pointer(&pkeyAppUserModeID)), uintptr(unsafe.Pointer(&pv))).Err() == nil {
			aumid = pv.String()
			pv.Clear()
		}
		win.Release(ps)
	}
	return ClassifyShortcut(path, target, args, wd, aumid), nil
}

func linkString(link win.Obj, slot int, isPath bool) string {
	buf := make([]uint16, 1024)
	var hr win.HRESULT
	if isPath {
		// GetPath(pszFile, cch, pfd=NULL, fFlags=0)
		hr = win.Call(link, slot, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)), 0, 0)
	} else {
		hr = win.Call(link, slot, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	}
	if hr != 0 { // S_FALSE means no value
		return ""
	}
	return windows.UTF16ToString(buf)
}
