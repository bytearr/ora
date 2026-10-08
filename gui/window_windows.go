package gui

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"sync/atomic"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"

	"ora/core/config"
	"ora/core/engine"
	"ora/core/index"
	"ora/core/match"
	"ora/core/win"
)

var (
	user32   = windows.NewLazySystemDLL("user32.dll")
	gdi32    = windows.NewLazySystemDLL("gdi32.dll")
	shell32  = windows.NewLazySystemDLL("shell32.dll")
	kernel32 = windows.NewLazySystemDLL("kernel32.dll")
	gdiplus  = windows.NewLazySystemDLL("gdiplus.dll")
	dwmapi   = windows.NewLazySystemDLL("dwmapi.dll")

	procAllowSetForegroundWindow = user32.NewProc("AllowSetForegroundWindow")
	procAttachThreadInput        = user32.NewProc("AttachThreadInput")
	procBeginPaint               = user32.NewProc("BeginPaint")
	procBringWindowToTop         = user32.NewProc("BringWindowToTop")
	procCallNextHookEx           = user32.NewProc("CallNextHookEx")
	procCallWindowProcW          = user32.NewProc("CallWindowProcW")
	procCreateWindowExW          = user32.NewProc("CreateWindowExW")
	procDefWindowProcW           = user32.NewProc("DefWindowProcW")
	procDestroyIcon              = user32.NewProc("DestroyIcon")
	procDestroyWindow            = user32.NewProc("DestroyWindow")
	procDispatchMessageW         = user32.NewProc("DispatchMessageW")
	procDrawIconEx               = user32.NewProc("DrawIconEx")
	procDrawTextW                = user32.NewProc("DrawTextW")
	procEndPaint                 = user32.NewProc("EndPaint")
	procFillRect                 = user32.NewProc("FillRect")
	procFindWindowW              = user32.NewProc("FindWindowW")
	procGetClientRect            = user32.NewProc("GetClientRect")
	procGetCursorPos             = user32.NewProc("GetCursorPos")
	procGetDC                    = user32.NewProc("GetDC")
	procGetForegroundWindow      = user32.NewProc("GetForegroundWindow")
	procGetKeyState              = user32.NewProc("GetKeyState")
	procGetMessageW              = user32.NewProc("GetMessageW")
	procGetMonitorInfoW          = user32.NewProc("GetMonitorInfoW")
	procGetSystemMetrics         = user32.NewProc("GetSystemMetrics")
	procGetWindowRect            = user32.NewProc("GetWindowRect")
	procGetWindowTextLengthW     = user32.NewProc("GetWindowTextLengthW")
	procGetWindowTextW           = user32.NewProc("GetWindowTextW")
	procGetWindowThreadProcessId = user32.NewProc("GetWindowThreadProcessId")
	procInvalidateRect           = user32.NewProc("InvalidateRect")
	procIsWindowVisible          = user32.NewProc("IsWindowVisible")
	procKillTimer                = user32.NewProc("KillTimer")
	procLoadCursorW              = user32.NewProc("LoadCursorW")
	procMonitorFromPoint         = user32.NewProc("MonitorFromPoint")
	procPostMessageW             = user32.NewProc("PostMessageW")
	procPostQuitMessage          = user32.NewProc("PostQuitMessage")
	procRegisterClassExW         = user32.NewProc("RegisterClassExW")
	procReleaseDC                = user32.NewProc("ReleaseDC")
	procSendInput                = user32.NewProc("SendInput")
	procSendMessageW             = user32.NewProc("SendMessageW")
	procSetFocus                 = user32.NewProc("SetFocus")
	procSetForegroundWindow      = user32.NewProc("SetForegroundWindow")
	procSetProcessDPIAware       = user32.NewProc("SetProcessDPIAware")
	procSetTimer                 = user32.NewProc("SetTimer")
	procSetWindowLongPtrW        = user32.NewProc("SetWindowLongPtrW")
	procSetWindowPos             = user32.NewProc("SetWindowPos")
	procSetWindowTextW           = user32.NewProc("SetWindowTextW")
	procSetWindowsHookExW        = user32.NewProc("SetWindowsHookExW")
	procShowWindow               = user32.NewProc("ShowWindow")
	procTranslateMessage         = user32.NewProc("TranslateMessage")
	procUnhookWindowsHookEx      = user32.NewProc("UnhookWindowsHookEx")

	procBitBlt                 = gdi32.NewProc("BitBlt")
	procCreateCompatibleBitmap = gdi32.NewProc("CreateCompatibleBitmap")
	procCreateCompatibleDC     = gdi32.NewProc("CreateCompatibleDC")
	procCreateFontIndirectW    = gdi32.NewProc("CreateFontIndirectW")
	procCreateSolidBrush       = gdi32.NewProc("CreateSolidBrush")
	procDeleteDC               = gdi32.NewProc("DeleteDC")
	procDeleteObject           = gdi32.NewProc("DeleteObject")
	procGetDeviceCaps          = gdi32.NewProc("GetDeviceCaps")
	procGetTextFaceW           = gdi32.NewProc("GetTextFaceW")
	procSelectObject           = gdi32.NewProc("SelectObject")
	procSetBkColor             = gdi32.NewProc("SetBkColor")
	procSetBkMode              = gdi32.NewProc("SetBkMode")
	procSetTextColor           = gdi32.NewProc("SetTextColor")

	procSHGetFileInfoW     = shell32.NewProc("SHGetFileInfoW")
	procSHParseDisplayName = shell32.NewProc("SHParseDisplayName")

	procOpenMutexW = kernel32.NewProc("OpenMutexW")

	procGdiplusStartup         = gdiplus.NewProc("GdiplusStartup")
	procGdiplusShutdown        = gdiplus.NewProc("GdiplusShutdown")
	procGdipCreateFromHDC      = gdiplus.NewProc("GdipCreateFromHDC")
	procGdipDeleteGraphics     = gdiplus.NewProc("GdipDeleteGraphics")
	procGdipSetSmoothingMode   = gdiplus.NewProc("GdipSetSmoothingMode")
	procGdipSetPixelOffsetMode = gdiplus.NewProc("GdipSetPixelOffsetMode")
	procGdipCreateSolidFill    = gdiplus.NewProc("GdipCreateSolidFill")
	procGdipDeleteBrush        = gdiplus.NewProc("GdipDeleteBrush")
	procGdipFillEllipseI       = gdiplus.NewProc("GdipFillEllipseI")
	procGdipFillRectangleI     = gdiplus.NewProc("GdipFillRectangleI")

	procDwmSetWindowAttribute = dwmapi.NewProc("DwmSetWindowAttribute")
)

const (
	wmDestroy      = 0x0002
	wmActivate     = 0x0006
	wmPaint        = 0x000F
	wmClose        = 0x0010
	wmEraseBkgnd   = 0x0014
	wmSetFont      = 0x0030
	wmKeyDown      = 0x0100
	wmChar         = 0x0102
	wmSysKeyDown   = 0x0104
	wmCommand      = 0x0111
	wmTimer        = 0x0113
	wmCtlColorEdit = 0x0133
	wmLButtonDown  = 0x0201
	wmMouseWheel   = 0x020A
	wmApp          = 0x8000

	wmHotkey   = wmApp + 1 // from the hook: toggle
	wmShow     = wmApp + 2 // from a second `ora`: show
	wmResult   = wmApp + 3
	wmIndex    = wmApp + 4
	wmLaunched = wmApp + 5
	wmIcon     = wmApp + 6

	wsPopup        = 0x80000000
	wsChild        = 0x40000000
	wsVisible      = 0x10000000
	wsClipChildren = 0x02000000
	wsExTopmost    = 0x00000008
	wsExToolWindow = 0x00000080
	esAutoHScroll  = 0x0080
	enChange       = 0x0300
	emGetSel       = 0x00B0
	emSetSel       = 0x00B1
	emReplaceSel   = 0x00C2
	emGetMargins   = 0x00D4
	csDropShadow   = 0x00020000

	swHide = 0
	swShow = 5

	swpNoSize     = 0x0001
	swpNoMove     = 0x0002
	swpNoZOrder   = 0x0004
	swpNoActivate = 0x0010
	swpShowWindow = 0x0040

	waInactive = 0
	mkShift    = 0x0004

	vkBack   = 0x08
	vkReturn = 0x0D
	vkShift  = 0x10
	vkEscape = 0x1B
	vkUp     = 0x26
	vkDown   = 0x28

	dtVCenter      = 0x0004
	dtSingleLine   = 0x0020
	dtCalcRect     = 0x0400
	dtNoPrefix     = 0x0800
	dtPathEllipsis = 0x4000
	dtEndEllipsis  = 0x8000

	smCxScreen         = 0
	smCyScreen         = 1
	logPixelsY         = 90
	transparent        = 1
	srcCopy            = 0x00CC0020
	diNormal           = 0x0003
	idcArrow           = 32512
	monitorPrimary     = 1
	monitorDefaultNear = 2
	shgfiIcon          = 0x0100
	shgfiPIDL          = 0x0008
	defaultCharset     = 1
	clearTypeQuality   = 5
	synchronize        = 0x00100000

	dwmaCornerPreference = 33
	dwmaBorderColor      = 34
	dwmCornerRound       = 2
	smoothingAntiAlias   = 4
	pixelOffsetHalf      = 4

	timerDebounce = 1
	className     = "ora"
	mutexName     = `Local\ora`
)

var (
	gwlpWndProc = ^uintptr(3) // -4
	hwndTopmost = ^uintptr(0) // -1
)

type rect struct{ left, top, right, bottom int32 }

type point struct{ x, y int32 }

type winMsg struct {
	hwnd    uintptr
	message uint32
	wParam  uintptr
	lParam  uintptr
	time    uint32
	pt      point
	private uint32
}

type wndClassEx struct {
	cbSize        uint32
	style         uint32
	lpfnWndProc   uintptr
	cbClsExtra    int32
	cbWndExtra    int32
	hInstance     uintptr
	hIcon         uintptr
	hCursor       uintptr
	hbrBackground uintptr
	lpszMenuName  *uint16
	lpszClassName *uint16
	hIconSm       uintptr
}

type paintStruct struct {
	hdc         uintptr
	fErase      int32
	rcPaint     rect
	fRestore    int32
	fIncUpdate  int32
	rgbReserved [32]byte
}

type logFont struct {
	height, width, escapement, orientation, weight int32
	italic, underline, strikeOut, charSet          byte
	outPrecision, clipPrecision, quality, pitch    byte
	faceName                                       [32]uint16
}

type monitorInfo struct {
	cbSize    uint32
	rcMonitor rect
	rcWork    rect
	dwFlags   uint32
}

type shFileInfo struct {
	hIcon         uintptr
	iIcon         int32
	dwAttributes  uint32
	szDisplayName [260]uint16
	szTypeName    [80]uint16
}

func loword(v uintptr) uint16 { return uint16(v) }
func hiword(v uintptr) uint16 { return uint16(v >> 16) }

func rgb(c uint32) uint32 { return (c>>16)&0xFF | c&0xFF00 | (c&0xFF)<<16 } // 0xRRGGBB to COLORREF

// palette: bg window, field search box, sel selected row, line separator
// and border, dim secondary text, accent the fallback when Windows has none.
type palette struct{ bg, field, sel, line, text, dim, accent uint32 }

var palettes = map[string]palette{
	config.ThemeDark: {bg: rgb(0x202020), field: rgb(0x2d2d2d), sel: rgb(0x333333), line: rgb(0x3a3a3a),
		text: rgb(0xffffff), dim: rgb(0x9e9e9e), accent: rgb(0x4cc2ff)},
	config.ThemeLight: {bg: rgb(0xf9f9f9), field: rgb(0xffffff), sel: rgb(0xeaeaea), line: rgb(0xe0e0e0),
		text: rgb(0x1a1a1a), dim: rgb(0x616161), accent: rgb(0x005fb8)},
}

// Sizes in logical pixels; px scales them to the primary monitor's DPI.
const (
	winWidth  = 720
	padPx     = 12 // around the search box
	boxPx     = 44 // search box height
	listGapPx = 8  // separator to first line, last line to bottom
	headerPx  = 30
	rowPx     = 52
	morePx    = 40
	notePx    = 32
	insetPx   = 8 // row highlight to window edge
	radiusPx  = 6
	iconPx    = 32
	fontPx    = 15
	smallPx   = 12
	glyphPx   = 16
	wheelRows = 3

	glyphSearch = "\ue721"
)

type searchReq struct {
	gen    uint64
	query  string
	recent bool
	ix     *index.Index
}

type iconReq struct {
	key   string
	entry index.Entry
}

type iconRes struct {
	key  string
	icon uintptr
}

type window struct {
	eng      *engine.Engine
	settings config.Settings
	hk       config.Hotkey
	log      *logger

	hinst, hwnd, edit, editProc uintptr
	font, small, head, glyph    uintptr
	bgBrush, fieldBrush         uintptr
	gdipToken                   uintptr
	rounded                     bool // DWM rounds and frames the window (Windows 11)
	dpi                         int32
	colors                      palette
	accent                      uint32
	work                        rect // work area of the monitor the window is on
	x, top, lineH, smallH       int32

	m           Model
	latestGen   atomic.Uint64
	ix          *index.Index
	ixErr       string
	status      []string
	settingText bool
	busy        bool
	startHidden bool // autostart: wait for the hotkey

	reqs     chan searchReq
	results  chan Result
	indexed  chan indexDone
	launched chan error
	iconReqs chan iconReq
	iconRess chan iconRes
	icons    map[string]uintptr
	asked    map[string]bool

	panicVal   any
	panicStack []byte
}

type indexDone struct {
	ix  *index.Index
	err error
}

// theWindow is the one window; the Win32 callbacks reach it here.
var theWindow *window

var (
	wndProcCB  = syscall.NewCallback(wndProc)
	editProcCB = syscall.NewCallback(editProc)
)

func (w *window) px(v int32) int32 { return (v*w.dpi + 48) / 96 }

func utf16(s string) *uint16 {
	p, _ := windows.UTF16PtrFromString(s)
	return p
}

func (w *window) create() error {
	var h windows.Handle
	if err := windows.GetModuleHandleEx(0, nil, &h); err != nil {
		return err
	}
	w.hinst = uintptr(h)
	dc, _, _ := procGetDC.Call(0)
	d, _, _ := procGetDeviceCaps.Call(dc, logPixelsY)
	procReleaseDC.Call(0, dc)
	w.dpi = int32(d)
	if w.dpi <= 0 {
		w.dpi = 96
	}

	w.colors = palettes[w.settings.Theme]
	w.accent = accentColor(w.settings.Theme, w.colors.accent)
	w.bgBrush, _, _ = procCreateSolidBrush.Call(uintptr(w.colors.bg))
	w.fieldBrush, _, _ = procCreateSolidBrush.Call(uintptr(w.colors.field))
	w.font = newFont("Segoe UI", w.px(fontPx), 400)
	w.small = newFont("Segoe UI", w.px(smallPx), 400)
	w.head = newFont("Segoe UI", w.px(smallPx), 600)
	if w.font == 0 || w.small == 0 || w.head == 0 {
		return fmt.Errorf("CreateFontIndirect failed")
	}
	w.glyph = iconFont(w.px(glyphPx))

	input := struct {
		version          uint32
		debugCallback    uintptr
		noBackground     int32
		noExternalCodecs int32
	}{version: 1}
	if procGdiplusStartup.Find() == nil {
		if st, _, _ := procGdiplusStartup.Call(uintptr(unsafe.Pointer(&w.gdipToken)), uintptr(unsafe.Pointer(&input)), 0); st != 0 {
			w.gdipToken = 0
		}
	}

	cursor, _, _ := procLoadCursorW.Call(0, idcArrow)
	wc := wndClassEx{
		style:         csDropShadow,
		lpfnWndProc:   wndProcCB,
		hInstance:     w.hinst,
		hCursor:       cursor,
		lpszClassName: utf16(className),
	}
	wc.cbSize = uint32(unsafe.Sizeof(wc))
	if r, _, err := procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc))); r == 0 {
		return fmt.Errorf("RegisterClassEx: %w", err)
	}
	theWindow = w
	hwnd, _, err := procCreateWindowExW.Call(wsExToolWindow|wsExTopmost, uintptr(unsafe.Pointer(utf16(className))),
		uintptr(unsafe.Pointer(utf16("ora"))), wsPopup|wsClipChildren,
		0, 0, uintptr(w.px(winWidth)), uintptr(w.px(2*padPx+boxPx)), 0, 0, w.hinst, 0)
	if hwnd == 0 {
		return fmt.Errorf("CreateWindowEx: %w", err)
	}
	w.hwnd = hwnd
	w.frame()

	w.lineH = w.textHeight(w.font)
	w.smallH = w.textHeight(w.small)
	box := w.searchBox()
	editX := box.left + w.px(44)
	edit, _, err := procCreateWindowExW.Call(0, uintptr(unsafe.Pointer(utf16("EDIT"))), 0,
		wsChild|wsVisible|esAutoHScroll,
		uintptr(editX), uintptr(box.top+(box.bottom-box.top-w.lineH)/2),
		uintptr(box.right-w.px(padPx)-editX), uintptr(w.lineH), hwnd, 1, w.hinst, 0)
	if edit == 0 {
		return fmt.Errorf("CreateWindowEx EDIT: %w", err)
	}
	w.edit = edit
	procSendMessageW.Call(edit, wmSetFont, w.font, 1)
	w.editProc, _, _ = procSetWindowLongPtrW.Call(edit, gwlpWndProc, editProcCB)
	return nil
}

func newFont(face string, px, weight int32) uintptr {
	lf := logFont{height: -px, weight: weight, charSet: defaultCharset, quality: clearTypeQuality}
	copy(lf.faceName[:], windows.StringToUTF16(face))
	f, _, _ := procCreateFontIndirectW.Call(uintptr(unsafe.Pointer(&lf)))
	return f
}

// iconFont is the Windows 11 icon font, else the Windows 10 one, else 0.
// GDI substitutes a missing face silently, so the selected face is checked.
func iconFont(px int32) uintptr {
	dc, _, _ := procGetDC.Call(0)
	defer procReleaseDC.Call(0, dc)
	for _, face := range []string{"Segoe Fluent Icons", "Segoe MDL2 Assets"} {
		f := newFont(face, px, 400)
		if f == 0 {
			continue
		}
		old, _, _ := procSelectObject.Call(dc, f)
		var buf [32]uint16
		procGetTextFaceW.Call(dc, uintptr(len(buf)), uintptr(unsafe.Pointer(&buf[0])))
		procSelectObject.Call(dc, old)
		if windows.UTF16ToString(buf[:]) == face {
			return f
		}
		procDeleteObject.Call(f)
	}
	return 0
}

// accentColor is the Windows accent in the shade Windows uses for the
// theme: Light2 on dark, Dark1 on light.
func accentColor(theme string, fallback uint32) uint32 {
	shade := 1
	if theme == config.ThemeLight {
		shade = 4
	}
	k, err := registry.OpenKey(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\Explorer\Accent`, registry.QUERY_VALUE)
	if err != nil {
		return fallback
	}
	defer k.Close()
	b, _, err := k.GetBinaryValue("AccentPalette")
	if err != nil || len(b) < 32 {
		return fallback
	}
	c := b[shade*4:]
	return uint32(c[0]) | uint32(c[1])<<8 | uint32(c[2])<<16
}

// frame asks DWM for round corners and a theme border. Before Windows 11
// that fails and paint draws a square border itself.
func (w *window) frame() {
	if procDwmSetWindowAttribute.Find() != nil {
		return
	}
	pref := uint32(dwmCornerRound)
	hr, _, _ := procDwmSetWindowAttribute.Call(w.hwnd, dwmaCornerPreference, uintptr(unsafe.Pointer(&pref)), 4)
	w.rounded = hr == 0
	if w.rounded {
		c := w.colors.line
		procDwmSetWindowAttribute.Call(w.hwnd, dwmaBorderColor, uintptr(unsafe.Pointer(&c)), 4)
	}
}

func (w *window) textHeight(font uintptr) int32 {
	dc, _, _ := procGetDC.Call(w.hwnd)
	old, _, _ := procSelectObject.Call(dc, font)
	r := rect{}
	s := windows.StringToUTF16("Ag")
	procDrawTextW.Call(dc, uintptr(unsafe.Pointer(&s[0])), uintptr(len(s)-1), uintptr(unsafe.Pointer(&r)), dtCalcRect|dtSingleLine|dtNoPrefix)
	procSelectObject.Call(dc, old)
	procReleaseDC.Call(w.hwnd, dc)
	return r.bottom - r.top + w.px(2)
}

func (w *window) startWorkers() {
	w.reqs = make(chan searchReq, 1)
	w.results = make(chan Result, 8)
	w.indexed = make(chan indexDone, 1)
	w.launched = make(chan error, 1)
	w.iconReqs = make(chan iconReq, 64)
	w.iconRess = make(chan iconRes, 64)
	w.icons = map[string]uintptr{}
	w.asked = map[string]bool{}
	go w.loadIndex()
	go w.searchLoop()
	go w.iconLoop()
}

func (w *window) post(msg uintptr) { procPostMessageW.Call(w.hwnd, msg, 0, 0) }

func (w *window) recoverWorker(what string) {
	if r := recover(); r != nil {
		w.log.printf("panic in %s: %v\n%s", what, r, debug.Stack())
	}
}

func (w *window) loadIndex() {
	defer w.recoverWorker("index")
	ix, err := w.eng.Index(false)
	w.indexed <- indexDone{ix, err}
	w.post(wmIndex)
}

func (w *window) searchLoop() {
	for req := range w.reqs {
		if req.gen != w.latestGen.Load() {
			continue
		}
		w.search(req)
	}
}

func (w *window) deliver(r Result) {
	w.results <- r
	w.post(wmResult)
}

type fileResult struct {
	ix     *index.Index
	ranked []match.Result
	err    error
}

// search delivers the answer to req. A text query is answered twice: the
// programs as soon as they are ranked, the full list when Everything has
// replied. Both searches run at the same time, and search returns after the
// programs so the next query need not wait for Everything; a file result
// that is no longer the latest query is dropped.
func (w *window) search(req searchReq) {
	defer func() {
		if p := recover(); p != nil {
			w.log.printf("panic in search: %v\n%s", p, debug.Stack())
			w.deliver(Result{Gen: req.gen, Note: fmt.Sprintf("search failed: %v", p)})
		}
	}()
	if req.recent {
		ids, err := w.eng.RecentIDs()
		if err != nil {
			w.log.printf("recent: %v", err)
		}
		w.deliver(Result{Gen: req.gen, Rows: DropMissing(RecentRows(ids, req.ix, os.Stat), os.Stat)})
		return
	}
	files := make(chan fileResult, 1)
	go func() {
		defer func() {
			if p := recover(); p != nil {
				w.log.printf("panic in file search: %v\n%s", p, debug.Stack())
				files <- fileResult{err: fmt.Errorf("file search failed: %v", p)}
			}
		}()
		ix, ranked, err := w.eng.SearchPaths(req.query, true)
		files <- fileResult{ix, ranked, err}
	}()
	programs := DropMissing(ProgramRows(req.ix, w.eng.Search(req.ix, req.query)), os.Stat)
	var typed *index.Entry
	if e, ok := TypedPath(req.query); ok {
		typed = &e
	}
	first := SearchResult(req.gen, programs, typed, nil, nil, nil)
	first.Partial = true
	w.deliver(first)
	go func() {
		defer w.recoverWorker("file search")
		f := <-files
		if req.gen != w.latestGen.Load() {
			return
		}
		w.deliver(SearchResult(req.gen, programs, typed, f.ix, f.ranked, f.err))
	}()
}

func (w *window) submit(req searchReq) {
	select {
	case <-w.reqs:
	default:
	}
	w.reqs <- req
}

func (w *window) iconLoop() {
	runtime.LockOSThread()
	if err := windows.CoInitializeEx(0, windows.COINIT_APARTMENTTHREADED); err == nil {
		defer windows.CoUninitialize()
	}
	for req := range w.iconReqs {
		w.iconRess <- iconRes{req.key, loadIcon(req.entry)}
		w.post(wmIcon)
	}
}

// loadIcon returns the shell icon of what the entry starts: the shortcut
// target if it exists, else the file itself; a Store app by its AUMID.
func loadIcon(e index.Entry) uintptr {
	var sfi shFileInfo
	if e.Kind == index.KindStore {
		var pidl uintptr
		name := utf16(`shell:AppsFolder\` + e.AUMID)
		if hr, _, _ := procSHParseDisplayName.Call(uintptr(unsafe.Pointer(name)), 0, uintptr(unsafe.Pointer(&pidl)), 0, 0); hr != 0 || pidl == 0 {
			return 0
		}
		procSHGetFileInfoW.Call(pidl, 0, uintptr(unsafe.Pointer(&sfi)), unsafe.Sizeof(sfi), shgfiPIDL|shgfiIcon)
		windows.CoTaskMemFree(win.PtrFromUintptr(pidl))
		return sfi.hIcon
	}
	p := e.Target
	if e.Resolved != "" {
		if _, err := os.Stat(e.Resolved); err == nil {
			p = e.Resolved
		}
	}
	procSHGetFileInfoW.Call(uintptr(unsafe.Pointer(utf16(p))), 0, uintptr(unsafe.Pointer(&sfi)), unsafe.Sizeof(sfi), shgfiIcon)
	return sfi.hIcon
}

func (w *window) icon(e index.Entry) uintptr {
	key := e.ID()
	if h, ok := w.icons[key]; ok {
		return h
	}
	if !w.asked[key] {
		select {
		case w.iconReqs <- iconReq{key, e}:
			w.asked[key] = true
		default:
		}
	}
	return 0
}

func (w *window) drainIcons() {
	for {
		select {
		case r := <-w.iconRess:
			w.icons[r.key] = r.icon
		default:
			procInvalidateRect.Call(w.hwnd, 0, 0)
			return
		}
	}
}

func (w *window) freeIcons() {
	for _, h := range w.icons {
		if h != 0 {
			procDestroyIcon.Call(h)
		}
	}
	w.icons = nil
	if w.gdipToken != 0 {
		procGdiplusShutdown.Call(w.gdipToken)
		w.gdipToken = 0
	}
}

func (w *window) visible() bool {
	r, _, _ := procIsWindowVisible.Call(w.hwnd)
	return r != 0
}

func (w *window) notes() []string {
	var n []string
	switch {
	case w.ixErr != "":
		n = append(n, w.ixErr)
	case w.ix == nil:
		n = append(n, "index loading")
	}
	n = append(n, w.status...)
	if w.m.Note != "" {
		n = append(n, w.m.Note)
	}
	return n
}

type lineKind int

const (
	lineHeader lineKind = iota
	lineRow
	lineMore
	lineNote
)

// line is one band under the search box. item indexes Model.Visible.
type line struct {
	kind lineKind
	item int
	text string
	y, h int32
}

func (w *window) searchBox() rect {
	p := w.px(padPx)
	return rect{p, p, w.px(winWidth) - p, p + w.px(boxPx)}
}

// group is the section header above a row.
func (w *window) group(r Row) string {
	switch {
	case QueryMode(w.m.Query) == ModeRecent:
		return "Recent"
	case r.IsPath():
		return "Files & folders"
	default:
		return "Apps"
	}
}

// lines lays out everything under the search box; paint, clicks and the
// window height all use it. A header starts each group on screen.
func (w *window) lines() (ls []line, height int32) {
	y := w.px(2*padPx + boxPx)
	_, items := w.m.Visible()
	notes := w.notes()
	if len(items)+len(notes) == 0 {
		return nil, y
	}
	y += w.px(listGapPx)
	add := func(l line, h int32) {
		l.y, l.h = y, w.px(h)
		ls = append(ls, l)
		y += l.h
	}
	group := ""
	for i, it := range items {
		if it.More {
			add(line{kind: lineMore, item: i}, morePx)
			continue
		}
		if g := w.group(w.m.Rows[it.Row]); g != group {
			group = g
			add(line{kind: lineHeader, text: g}, headerPx)
		}
		add(line{kind: lineRow, item: i}, rowPx)
	}
	for _, n := range notes {
		add(line{kind: lineNote, text: n}, notePx)
	}
	return ls, y + w.px(listGapPx)
}

// collapsedHeight is the window with a full first page: header, the
// visible rows and Show more. present centers that height, so a short
// list hangs from the same top and typing does not move the search box.
func (w *window) collapsedHeight() int32 {
	return w.px(2*padPx + boxPx + 2*listGapPx + headerPx + visibleRows*rowPx + morePx)
}

func (w *window) layout() {
	_, h := w.lines()
	y := w.top
	if w.m.Expanded {
		y = w.work.top + (w.work.bottom-w.work.top-h)/2
	}
	y = max(w.work.top, min(y, w.work.bottom-h))
	procSetWindowPos.Call(w.hwnd, 0, uintptr(w.x), uintptr(y), uintptr(w.px(winWidth)), uintptr(h), swpNoZOrder|swpNoActivate)
	procInvalidateRect.Call(w.hwnd, 0, 0)
}

// monitorRect is the work area (without taskbar) of the configured
// monitor. (0,0) always lies on the primary one.
func (w *window) monitorRect() rect {
	var pt point
	flag := uintptr(monitorPrimary)
	if w.settings.Monitor == config.MonitorCursor {
		procGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
		flag = monitorDefaultNear
	}
	mon, _, _ := procMonitorFromPoint.Call(uintptr(uint32(pt.x))|uintptr(uint32(pt.y))<<32, flag)
	mi := monitorInfo{}
	mi.cbSize = uint32(unsafe.Sizeof(mi))
	if r, _, _ := procGetMonitorInfoW.Call(mon, uintptr(unsafe.Pointer(&mi))); r != 0 {
		return mi.rcWork
	}
	cx, _, _ := procGetSystemMetrics.Call(smCxScreen)
	cy, _, _ := procGetSystemMetrics.Call(smCyScreen)
	return rect{0, 0, int32(cx), int32(cy)}
}

// reset empties the line and starts the recent list.
func (w *window) reset() {
	procKillTimer.Call(w.hwnd, timerDebounce)
	w.settingText = true
	procSetWindowTextW.Call(w.edit, uintptr(unsafe.Pointer(utf16(""))))
	w.settingText = false
	w.m.SetQuery("", false)
	w.latestGen.Store(w.m.Gen)
}

func (w *window) show() {
	if w.visible() {
		w.present()
		return
	}
	w.reset()
	w.requestRecent()
	w.present()
	go w.refreshAutostart()
}

// present places the window on the configured monitor and brings it to
// the front without touching the query.
func (w *window) present() {
	w.work = w.monitorRect()
	w.x = w.work.left + (w.work.right-w.work.left-w.px(winWidth))/2
	w.top = w.work.top + (w.work.bottom-w.work.top-w.collapsedHeight())/2
	w.layout()
	procSetWindowPos.Call(w.hwnd, hwndTopmost, 0, 0, 0, 0, swpNoSize|swpNoMove|swpShowWindow)
	procShowWindow.Call(w.hwnd, swShow)
	bringToFront(w.hwnd)
	if fg, _, _ := procGetForegroundWindow.Call(); fg != w.hwnd {
		w.log.printf("window shown without the foreground; it belongs to %s", windowOwner(fg))
	}
	procSetFocus.Call(w.edit)
}

// windowOwner names the process of hwnd for the log.
func windowOwner(hwnd uintptr) string {
	if hwnd == 0 {
		return "no window"
	}
	var pid uint32
	procGetWindowThreadProcessId.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return fmt.Sprintf("pid %d", pid)
	}
	defer windows.CloseHandle(h)
	buf := make([]uint16, windows.MAX_PATH)
	n := uint32(len(buf))
	if err := windows.QueryFullProcessImageName(h, 0, &buf[0], &n); err != nil {
		return fmt.Sprintf("pid %d", pid)
	}
	return fmt.Sprintf("%s (pid %d)", filepath.Base(windows.UTF16ToString(buf[:n])), pid)
}

func (w *window) hide() {
	procShowWindow.Call(w.hwnd, swHide)
	w.reset()
	w.status = nil
}

// toggle hides the window only when it has the focus. Visible behind
// another app, or shown without the foreground, the hotkey brings it up.
func (w *window) toggle() {
	if fg, _, _ := procGetForegroundWindow.Call(); w.visible() && fg == w.hwnd {
		w.hide()
		return
	}
	w.show()
}

// bringToFront is SetForegroundWindow from the owning thread. If Windows
// refuses, it retries attached to the foreground thread's input.
func bringToFront(hwnd uintptr) {
	procSetForegroundWindow.Call(hwnd)
	fg, _, _ := procGetForegroundWindow.Call()
	if fg == hwnd || fg == 0 {
		return
	}
	other, _, _ := procGetWindowThreadProcessId.Call(fg, 0)
	me := uintptr(windows.GetCurrentThreadId())
	if other == 0 || other == me {
		return
	}
	procAttachThreadInput.Call(me, other, 1)
	procSetForegroundWindow.Call(hwnd)
	procBringWindowToTop.Call(hwnd)
	procAttachThreadInput.Call(me, other, 0)
}

func (w *window) quit() {
	uninstallHook()
	procDestroyWindow.Call(w.hwnd)
}

func (w *window) onEdit() {
	procInvalidateRect.Call(w.edit, 0, 1) // drop or bring back the placeholder
	if w.settingText {
		return
	}
	n, _, _ := procGetWindowTextLengthW.Call(w.edit)
	buf := make([]uint16, n+1)
	procGetWindowTextW.Call(w.edit, uintptr(unsafe.Pointer(&buf[0])), n+1)
	w.status = nil
	q := windows.UTF16ToString(buf)
	mode := w.m.SetQuery(q, QueryMode(q) == ModeSearch && w.ix != nil)
	w.latestGen.Store(w.m.Gen)
	procKillTimer.Call(w.hwnd, timerDebounce)
	switch mode {
	case ModeRecent:
		w.requestRecent()
	case ModeSearch:
		if w.ix != nil {
			procSetTimer.Call(w.hwnd, timerDebounce, uintptr(debounce.Milliseconds()), 0)
		}
	}
	w.layout()
}

func (w *window) requestRecent() {
	if w.ix != nil {
		w.submit(searchReq{gen: w.m.Gen, recent: true, ix: w.ix})
	}
}

func (w *window) runSearch() {
	if w.ix == nil || QueryMode(w.m.Query) != ModeSearch {
		return
	}
	w.submit(searchReq{gen: w.m.Gen, query: w.m.Query, ix: w.ix})
}

func (w *window) onIndex() {
	d := <-w.indexed
	if d.err != nil {
		w.ixErr = "index: " + d.err.Error()
		w.log.printf("%s", w.ixErr)
	} else {
		w.ix = d.ix
	}
	switch QueryMode(w.m.Query) {
	case ModeRecent:
		w.requestRecent()
	case ModeSearch:
		procSetTimer.Call(w.hwnd, timerDebounce, uintptr(debounce.Milliseconds()), 0)
	}
	w.layout()
}

func (w *window) drainResults() {
	changed := false
	for {
		select {
		case r := <-w.results:
			changed = w.m.Apply(r) || changed
		default:
			if changed {
				w.layout()
				w.do(w.m.TakePending())
			}
			return
		}
	}
}

func (w *window) do(a Action) {
	switch a.Kind {
	case ActExpand:
		w.layout()
	case ActLaunch, ActReveal:
		if w.busy {
			return
		}
		w.busy = true
		ix := w.ix
		go func() {
			defer w.recoverWorker("launch")
			e := a.Entry
			var err error
			if a.Kind == ActLaunch {
				if err = w.eng.Launch(ix, e, nil); err == nil {
					w.eng.Remember(e)
				}
			} else {
				if e.Kind == index.KindFolder {
					e.Kind = index.KindFile // Shift+Enter highlights the folder in its parent
				}
				err = w.eng.Reveal(e)
			}
			w.launched <- err
			w.post(wmLaunched)
		}()
	}
}

func (w *window) onLaunched() {
	err := <-w.launched
	w.busy = false
	if err == nil {
		w.hide()
		return
	}
	w.log.printf("%v", err)
	w.status = []string{err.Error()}
	if !w.visible() {
		w.present()
	}
	w.layout()
}

func keyDown(vk uintptr) bool {
	r, _, _ := procGetKeyState.Call(vk)
	return int16(r) < 0
}

func (w *window) onDeactivate() {
	if !w.visible() {
		return
	}
	var pt point
	var r rect
	procGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
	procGetWindowRect.Call(w.hwnd, uintptr(unsafe.Pointer(&r)))
	if pt.x < r.left || pt.x >= r.right || pt.y < r.top || pt.y >= r.bottom {
		w.hide()
	}
}

func (w *window) catch(ret *uintptr) {
	if r := recover(); r != nil {
		if w.panicVal == nil {
			w.panicVal, w.panicStack = r, debug.Stack()
		}
		procPostQuitMessage.Call(4)
		*ret = 0
	}
}

func wndProc(hwnd, msg, wParam, lParam uintptr) (ret uintptr) {
	w := theWindow
	if w == nil || w.hwnd == 0 {
		r, _, _ := procDefWindowProcW.Call(hwnd, msg, wParam, lParam)
		return r
	}
	defer w.catch(&ret)
	switch msg {
	case wmPaint:
		w.paint(hwnd)
		return 0
	case wmEraseBkgnd:
		return 1
	case wmCtlColorEdit:
		procSetTextColor.Call(wParam, uintptr(w.colors.text))
		procSetBkColor.Call(wParam, uintptr(w.colors.field))
		return w.fieldBrush
	case wmCommand:
		if hiword(wParam) == enChange && lParam == w.edit {
			w.onEdit()
		}
		return 0
	case wmTimer:
		if wParam == timerDebounce {
			procKillTimer.Call(hwnd, timerDebounce)
			w.runSearch()
		}
		return 0
	case wmLButtonDown:
		y := int32(int16(hiword(lParam)))
		ls, _ := w.lines()
		for _, l := range ls {
			if (l.kind == lineRow || l.kind == lineMore) && y >= l.y && y < l.y+l.h {
				w.do(w.m.Click(l.item, wParam&mkShift != 0))
				break
			}
		}
		return 0
	case wmMouseWheel:
		steps := -int(int16(hiword(wParam))) * wheelRows / 120
		if steps == 0 {
			steps = 1
			if int16(hiword(wParam)) > 0 {
				steps = -1
			}
		}
		w.m.Scroll(steps)
		w.layout()
		return 0
	case wmActivate:
		if loword(wParam) == waInactive {
			w.onDeactivate()
		}
	case wmHotkey:
		w.toggle()
		return 0
	case wmShow:
		w.show()
		return 0
	case wmResult:
		w.drainResults()
		return 0
	case wmIndex:
		w.onIndex()
		return 0
	case wmLaunched:
		w.onLaunched()
		return 0
	case wmIcon:
		w.drainIcons()
		return 0
	case wmClose:
		w.hide() // Ctrl+Q quits; closing must not drop the hotkey
		return 0
	case wmDestroy:
		procPostQuitMessage.Call(0)
		return 0
	}
	r, _, _ := procDefWindowProcW.Call(hwnd, msg, wParam, lParam)
	return r
}

func editProc(hwnd, msg, wParam, lParam uintptr) (ret uintptr) {
	w := theWindow
	defer w.catch(&ret)
	switch msg {
	case wmKeyDown:
		switch wParam {
		case vkUp:
			w.m.Move(-1)
			w.layout()
			return 0
		case vkDown:
			w.m.Move(1)
			w.layout()
			return 0
		case vkReturn:
			w.do(w.m.Activate(keyDown(vkShift)))
			return 0
		case vkEscape:
			w.hide()
			return 0
		case 'Q':
			if keyDown(vkControl) {
				w.quit()
				return 0
			}
		case 'A':
			if keyDown(vkControl) {
				procSendMessageW.Call(hwnd, emSetSel, 0, ^uintptr(0))
				return 0
			}
		case vkBack:
			if keyDown(vkControl) {
				deleteWordBack(hwnd)
				return 0
			}
		}
	case wmChar:
		switch wParam {
		case '\r', '\n', 0x1B, 0x11, 0x01, 0x7F: // Enter, Esc, Ctrl+Q, Ctrl+A, Ctrl+Backspace
			return 0
		}
	case wmMouseWheel:
		r, _, _ := procSendMessageW.Call(w.hwnd, msg, wParam, lParam)
		return r
	case wmPaint:
		r, _, _ := procCallWindowProcW.Call(w.editProc, hwnd, msg, wParam, lParam)
		w.placeholder(hwnd)
		return r
	}
	r, _, _ := procCallWindowProcW.Call(w.editProc, hwnd, msg, wParam, lParam)
	return r
}

// deleteWordBack is Ctrl+Backspace, which the plain EDIT does not know:
// it deletes the selection, else the word before the caret.
func deleteWordBack(edit uintptr) {
	var start, end uint32
	procSendMessageW.Call(edit, emGetSel, uintptr(unsafe.Pointer(&start)), uintptr(unsafe.Pointer(&end)))
	if start == end {
		n, _, _ := procGetWindowTextLengthW.Call(edit)
		buf := make([]uint16, n+1)
		procGetWindowTextW.Call(edit, uintptr(unsafe.Pointer(&buf[0])), n+1)
		start = uint32(wordStart(buf[:n], int(min(uint32(n), start))))
		procSendMessageW.Call(edit, emSetSel, uintptr(start), uintptr(end))
	}
	procSendMessageW.Call(edit, emReplaceSel, 1, uintptr(unsafe.Pointer(utf16(""))))
}

// placeholder draws the hint over the empty field. The EDIT here has no
// cue banner: that needs comctl32 v6, and ora has no manifest.
func (w *window) placeholder(edit uintptr) {
	if n, _, _ := procGetWindowTextLengthW.Call(edit); n != 0 {
		return
	}
	var rc rect
	procGetClientRect.Call(edit, uintptr(unsafe.Pointer(&rc)))
	m, _, _ := procSendMessageW.Call(edit, emGetMargins, 0, 0)
	rc.left += int32(loword(m)) + w.px(1)
	dc, _, _ := procGetDC.Call(edit)
	procSetBkMode.Call(dc, transparent)
	w.text(dc, "Type to search", w.font, rc, w.colors.dim, dtEndEllipsis)
	procReleaseDC.Call(edit, dc)
}

func (w *window) paint(hwnd uintptr) {
	var ps paintStruct
	hdc, _, _ := procBeginPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
	defer procEndPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
	var rc rect
	procGetClientRect.Call(hwnd, uintptr(unsafe.Pointer(&rc)))
	mem, _, _ := procCreateCompatibleDC.Call(hdc)
	bmp, _, _ := procCreateCompatibleBitmap.Call(hdc, uintptr(rc.right), uintptr(rc.bottom))
	oldBmp, _, _ := procSelectObject.Call(mem, bmp)
	oldFont, _, _ := procSelectObject.Call(mem, w.font)
	procFillRect.Call(mem, uintptr(unsafe.Pointer(&rc)), w.bgBrush)
	procSetBkMode.Call(mem, transparent)

	box := w.searchBox()
	w.fillRound(mem, box, w.px(radiusPx), w.colors.field)
	r := w.px(radiusPx)
	fill(mem, rect{box.left + r, box.bottom - w.px(2), box.right - r, box.bottom}, w.accent)
	if w.glyph != 0 {
		w.text(mem, glyphSearch, w.glyph, rect{box.left + w.px(14), box.top, box.left + w.px(14+glyphPx+4), box.bottom}, w.colors.dim, 0)
	}

	ls, _ := w.lines()
	if len(ls) > 0 {
		sepY := w.px(2*padPx + boxPx)
		fill(mem, rect{0, sepY, rc.right, sepY + max(1, w.px(1))}, w.colors.line)
	}
	first, items := w.m.Visible()
	inset := w.px(insetPx)
	labelX := inset + w.px(12)
	textX := labelX + w.px(iconPx+12)
	right := rc.right - inset - w.px(12)
	for _, l := range ls {
		band := rect{inset, l.y, rc.right - inset, l.y + l.h}
		if (l.kind == lineRow || l.kind == lineMore) && first+l.item == w.m.Sel {
			w.fillRound(mem, band, w.px(radiusPx), w.colors.sel)
			barH := w.px(16)
			bar := rect{band.left, l.y + (l.h-barH)/2, band.left + w.px(3), l.y + (l.h+barH)/2}
			w.fillRound(mem, bar, (bar.right-bar.left)/2, w.accent)
		}
		switch l.kind {
		case lineHeader:
			w.text(mem, l.text, w.head, rect{labelX, l.y, right, l.y + l.h}, w.colors.dim, dtEndEllipsis)
		case lineNote:
			w.text(mem, l.text, w.small, rect{labelX, l.y, right, l.y + l.h}, w.colors.dim, dtEndEllipsis)
		case lineMore:
			w.text(mem, "Show more", w.font, rect{labelX, l.y, right, l.y + l.h}, w.accent, dtEndEllipsis)
		case lineRow:
			row := w.m.Rows[items[l.item].Row]
			size := w.px(iconPx)
			if h := w.icon(row.Entry); h != 0 {
				procDrawIconEx.Call(mem, uintptr(labelX), uintptr(l.y+(l.h-size)/2), h, uintptr(size), uintptr(size), 0, 0, diNormal)
			}
			name, sub, flags := rowText(row)
			top := l.y + (l.h-w.lineH-w.smallH)/2
			nameColor := w.colors.text
			if row.Blocked {
				nameColor = w.colors.dim
			}
			w.text(mem, name, w.font, rect{textX, top, right, top + w.lineH}, nameColor, dtEndEllipsis)
			w.text(mem, sub, w.small, rect{textX, top + w.lineH, right, top + w.lineH + w.smallH}, w.colors.dim, flags)
		}
	}
	if !w.rounded {
		for _, e := range []rect{{0, 0, rc.right, 1}, {0, rc.bottom - 1, rc.right, rc.bottom}, {0, 0, 1, rc.bottom}, {rc.right - 1, 0, rc.right, rc.bottom}} {
			fill(mem, e, w.colors.line)
		}
	}

	procBitBlt.Call(hdc, 0, 0, uintptr(rc.right), uintptr(rc.bottom), mem, 0, 0, srcCopy)
	procSelectObject.Call(mem, oldFont)
	procSelectObject.Call(mem, oldBmp)
	procDeleteObject.Call(bmp)
	procDeleteDC.Call(mem)
}

// rowText is the name and the second line of a row: the folder of a file
// or folder, else what kind of program it is.
func rowText(r Row) (name, sub string, flags uintptr) {
	name, dir := r.Label()
	switch {
	case dir != "":
		return name, dir, dtPathEllipsis
	case r.Entry.Kind == index.KindStore:
		return name, "Store app", dtEndEllipsis
	default:
		return name, "App", dtEndEllipsis
	}
}

// text draws s on one line, vertically centered in r.
func (w *window) text(dc uintptr, s string, font uintptr, r rect, color uint32, flags uintptr) {
	u := windows.StringToUTF16(s)
	n := uintptr(len(u) - 1)
	if n == 0 || r.left >= r.right {
		return
	}
	old, _, _ := procSelectObject.Call(dc, font)
	procSetTextColor.Call(dc, uintptr(color))
	procDrawTextW.Call(dc, uintptr(unsafe.Pointer(&u[0])), n, uintptr(unsafe.Pointer(&r)), dtSingleLine|dtVCenter|dtNoPrefix|flags)
	procSelectObject.Call(dc, old)
}

func fill(dc uintptr, r rect, color uint32) {
	b, _, _ := procCreateSolidBrush.Call(uintptr(color))
	procFillRect.Call(dc, uintptr(unsafe.Pointer(&r)), b)
	procDeleteObject.Call(b)
}

// fillRound fills r with corners of radius rad, antialiased by GDI+.
// The shape is four corner circles and two crossing rectangles, so only
// integer GDI+ calls are needed. Without GDI+ the corners stay square.
func (w *window) fillRound(dc uintptr, r rect, rad int32, color uint32) {
	width, height := r.right-r.left, r.bottom-r.top
	rad = min(rad, width/2, height/2)
	var g uintptr
	if w.gdipToken == 0 || rad <= 0 {
		fill(dc, r, color)
		return
	}
	if st, _, _ := procGdipCreateFromHDC.Call(dc, uintptr(unsafe.Pointer(&g))); st != 0 {
		fill(dc, r, color)
		return
	}
	defer procGdipDeleteGraphics.Call(g)
	procGdipSetSmoothingMode.Call(g, smoothingAntiAlias)
	procGdipSetPixelOffsetMode.Call(g, pixelOffsetHalf)
	argb := 0xFF000000 | (color&0xFF)<<16 | color&0xFF00 | (color>>16)&0xFF
	var b uintptr
	if st, _, _ := procGdipCreateSolidFill.Call(uintptr(argb), uintptr(unsafe.Pointer(&b))); st != 0 {
		return
	}
	defer procGdipDeleteBrush.Call(b)
	d := 2 * rad
	for _, c := range [][2]int32{{r.left, r.top}, {r.right - d, r.top}, {r.left, r.bottom - d}, {r.right - d, r.bottom - d}} {
		procGdipFillEllipseI.Call(g, b, uintptr(c[0]), uintptr(c[1]), uintptr(d), uintptr(d))
	}
	procGdipFillRectangleI.Call(g, b, uintptr(r.left+rad), uintptr(r.top), uintptr(width-d), uintptr(height))
	procGdipFillRectangleI.Call(g, b, uintptr(r.left), uintptr(r.top+rad), uintptr(width), uintptr(height-d))
}

// loop runs the message loop until WM_QUIT.
func (w *window) loop() {
	var m winMsg
	for {
		r, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if int32(r) <= 0 {
			return
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
		procDispatchMessageW.Call(uintptr(unsafe.Pointer(&m)))
	}
}
