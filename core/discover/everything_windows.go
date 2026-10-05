package discover

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"

	"ora/core/win"
)

var (
	user32                  = windows.NewLazySystemDLL("user32.dll")
	procFindWindowW         = user32.NewProc("FindWindowW")
	procRegisterClassExW    = user32.NewProc("RegisterClassExW")
	procCreateWindowExW     = user32.NewProc("CreateWindowExW")
	procDestroyWindow       = user32.NewProc("DestroyWindow")
	procDefWindowProcW      = user32.NewProc("DefWindowProcW")
	procGetMessageW         = user32.NewProc("GetMessageW")
	procDispatchMessageW    = user32.NewProc("DispatchMessageW")
	procSendMessageTimeoutW = user32.NewProc("SendMessageTimeoutW")
	procSetTimer            = user32.NewProc("SetTimer")
	procKillTimer           = user32.NewProc("KillTimer")
	procPostQuitMessage     = user32.NewProc("PostQuitMessage")
)

const (
	wmCopyData       = 0x004A
	wmTimer          = 0x0113
	hwndMessage      = ^uintptr(2) // (HWND)-3
	smtoAbortIfHung  = 0x0002
	copydataQuery2W  = 18
	ipcReplyID       = 0x0A7A0001
	ipcTimeoutMillis = 15000
	sendTimeoutMs    = 5000
)

var everythingClasses = []string{
	"EVERYTHING_TASKBAR_NOTIFICATION",
	"EVERYTHING_TASKBAR_NOTIFICATION_(1.5a)",
}

type copyDataStruct struct {
	dwData uintptr
	cbData uint32
	lpData unsafe.Pointer
}

type wndClassEx struct {
	cbSize        uint32
	style         uint32
	lpfnWndProc   uintptr
	cbClsExtra    int32
	cbWndExtra    int32
	hInstance     windows.Handle
	hIcon         uintptr
	hCursor       uintptr
	hbrBackground uintptr
	lpszMenuName  *uint16
	lpszClassName *uint16
	hIconSm       uintptr
}

type winMsg struct {
	hwnd    uintptr
	message uint32
	wParam  uintptr
	lParam  uintptr
	time    uint32
	ptX     int32
	ptY     int32
	private uint32
}

var (
	classOnce sync.Once
	classErr  error
	className = windows.StringToUTF16Ptr("ora_everything_reply")

	// Reply state, only touched on the thread running queryIPC.
	ipcReply   []byte
	ipcDone    bool
	ipcTimeout bool
)

func wndProc(hwnd, msg, wParam, lParam uintptr) uintptr {
	switch msg {
	case wmCopyData:
		cds := (*copyDataStruct)(win.PtrFromUintptr(lParam))
		if cds.dwData == ipcReplyID {
			ipcReply = bytes.Clone(unsafe.Slice((*byte)(cds.lpData), cds.cbData))
			ipcDone = true
			procPostQuitMessage.Call(0)
			return 1
		}
	case wmTimer:
		ipcTimeout = true
		procPostQuitMessage.Call(0)
		return 0
	}
	r, _, _ := procDefWindowProcW.Call(hwnd, msg, wParam, lParam)
	return r
}

func registerClass() error {
	classOnce.Do(func() {
		var hinst windows.Handle
		if err := windows.GetModuleHandleEx(0, nil, &hinst); err != nil {
			classErr = err
			return
		}
		wc := wndClassEx{
			lpfnWndProc:   windows.NewCallback(wndProc),
			hInstance:     hinst,
			lpszClassName: className,
		}
		wc.cbSize = uint32(unsafe.Sizeof(wc))
		if r, _, err := procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc))); r == 0 {
			classErr = fmt.Errorf("RegisterClassEx: %w", err)
		}
	})
	return classErr
}

func findEverything() uintptr {
	for _, c := range everythingClasses {
		p, _ := windows.UTF16PtrFromString(c)
		if h, _, _ := procFindWindowW.Call(uintptr(unsafe.Pointer(p)), 0); h != 0 {
			return h
		}
	}
	return 0
}

// queryIPC runs one EVERYTHING_IPC_QUERY2 against the running Everything.
func queryIPC(search string, maxResults, searchFlags uint32, keepFolders bool) ([]string, int, error) {
	target := findEverything()
	if target == 0 {
		return nil, 0, ErrEverythingNotRunning
	}

	type result struct {
		paths []string
		total int
		err   error
	}
	done := make(chan result, 1)
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		p, t, err := queryOnThread(target, search, maxResults, searchFlags, keepFolders)
		done <- result{p, t, err}
	}()
	r := <-done
	return r.paths, r.total, r.err
}

func queryOnThread(target uintptr, search string, maxResults, searchFlags uint32, keepFolders bool) ([]string, int, error) {
	if err := registerClass(); err != nil {
		return nil, 0, err
	}
	hwnd, _, err := procCreateWindowExW.Call(0, uintptr(unsafe.Pointer(className)), 0, 0,
		0, 0, 0, 0, hwndMessage, 0, 0, 0)
	if hwnd == 0 {
		return nil, 0, fmt.Errorf("CreateWindowEx: %w", err)
	}
	defer procDestroyWindow.Call(hwnd)

	ipcReply, ipcDone, ipcTimeout = nil, false, false
	q := buildQuery2(uint32(hwnd), ipcReplyID, searchFlags, maxResults, search)
	cds := copyDataStruct{dwData: copydataQuery2W, cbData: uint32(len(q)), lpData: unsafe.Pointer(&q[0])}
	var accepted uintptr
	r, _, err := procSendMessageTimeoutW.Call(target, wmCopyData, hwnd, uintptr(unsafe.Pointer(&cds)),
		smtoAbortIfHung, sendTimeoutMs, uintptr(unsafe.Pointer(&accepted)))
	runtime.KeepAlive(q)
	if r == 0 {
		return nil, 0, fmt.Errorf("SendMessageTimeout: %w (Everything elevated or hung?)", err)
	}
	if accepted == 0 {
		return nil, 0, errors.New("Everything rejected the query (needs Everything 1.4.1+)")
	}

	procSetTimer.Call(hwnd, 1, ipcTimeoutMillis, 0)
	defer procKillTimer.Call(hwnd, 1)
	var m winMsg
	for !ipcDone && !ipcTimeout {
		g, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if int32(g) <= 0 {
			break
		}
		procDispatchMessageW.Call(uintptr(unsafe.Pointer(&m)))
	}
	if !ipcDone {
		return nil, 0, fmt.Errorf("no reply from Everything within %d s", ipcTimeoutMillis/1000)
	}
	return parseList2(ipcReply, keepFolders)
}

func findES() (string, error) {
	if p, err := exec.LookPath("es.exe"); err == nil {
		return p, nil
	}
	var candidates []string
	if la := os.Getenv("LOCALAPPDATA"); la != "" {
		candidates = append(candidates, filepath.Join(la, `Microsoft\WinGet\Links\es.exe`))
		if m, _ := filepath.Glob(filepath.Join(la, `Microsoft\WinGet\Packages\voidtools.Everything.Cli_*\es.exe`)); len(m) > 0 {
			candidates = append(candidates, m...)
		}
	}
	for _, env := range []string{"ProgramFiles", "ProgramFiles(x86)"} {
		if pf := os.Getenv(env); pf != "" {
			candidates = append(candidates, filepath.Join(pf, `Everything\es.exe`))
		}
	}
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	return "", errors.New("es.exe not found")
}

// queryES is the fallback when IPC fails. The command line is passed raw so
// the quoted regex terms reach es.exe unchanged.
func queryES(search string, maxResults int, matchPath, keepFolders bool) ([]string, int, error) {
	es, err := findES()
	if err != nil {
		return nil, 0, err
	}
	if matchPath {
		search = "-match-path " + search
	}
	run := func(extra string) ([]byte, error) {
		cmd := exec.Command(es)
		cmd.SysProcAttr = &syscall.SysProcAttr{
			HideWindow: true,
			CmdLine:    windows.EscapeArg(es) + " " + extra + " " + search,
		}
		return cmd.Output()
	}
	// Console output uses the ANSI code page; only the export file is UTF-8.
	tmp, err := os.CreateTemp("", "ora-es-*.txt")
	if err != nil {
		return nil, 0, err
	}
	tmpPath := tmp.Name()
	tmp.Close()
	defer os.Remove(tmpPath)
	if _, err := run("-export-txt " + windows.EscapeArg(tmpPath) + " -utf8-bom -n " + strconv.Itoa(maxResults)); err != nil {
		return nil, 0, err
	}
	out, err := os.ReadFile(tmpPath)
	if err != nil {
		return nil, 0, err
	}
	out = bytes.TrimPrefix(out, []byte("\xef\xbb\xbf"))
	var paths []string
	sc := bufio.NewScanner(bytes.NewReader(out))
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		if line := strings.TrimSpace(sc.Text()); line != "" {
			paths = append(paths, line)
		}
	}
	total := len(paths)
	if cnt, err := run("-get-result-count"); err == nil {
		if n, err := strconv.Atoi(strings.TrimSpace(string(bytes.TrimPrefix(cnt, []byte("\xef\xbb\xbf"))))); err == nil {
			total = n
		}
	}
	if !keepFolders {
		paths = dropDirs(paths)
	}
	return paths, total, nil
}

func dropDirs(paths []string) []string {
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		st, err := os.Stat(p)
		if err == nil && st.IsDir() {
			continue
		}
		out = append(out, p)
	}
	return out
}
