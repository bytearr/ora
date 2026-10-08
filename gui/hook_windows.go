package gui

import (
	"sync/atomic"
	"syscall"
	"unsafe"

	"ora/core/config"
	"ora/core/win"
)

const (
	whKeyboardLL  = 13
	hcAction      = 0
	llkhfInjected = 0x10
	inputKeyboard = 1
	keyeventfUp   = 0x0002
	vkControl     = 0x11
)

type kbdLLHook struct {
	vkCode      uint32
	scanCode    uint32
	flags       uint32
	time        uint32
	dwExtraInfo uintptr
}

// keyInput is INPUT with a KEYBDINPUT, amd64 layout (40 bytes).
type keyInput struct {
	typ         uint32
	_           uint32
	wVk         uint16
	wScan       uint16
	dwFlags     uint32
	time        uint32
	_           uint32
	dwExtraInfo uintptr
	_           [8]byte
}

// hotkeyState is the hook's state machine. The hook callback runs on the
// window thread inside GetMessage; it only touches these atomics, posts one
// message and, for a Win hotkey, injects the Ctrl mask.
type hotkeyState struct {
	hk   config.Hotkey
	down [4]atomic.Uint64 // physical key state by virtual-key code

	aDown, bDown atomic.Bool // form 2: the two keys
	clean        atomic.Bool // form 2: nothing else went down since the first key
	armed        atomic.Bool // form 2: both held, clean
	maskWin      atomic.Bool // fired with Win involved; mask the next Win release

	fire func()
	mask func()
}

func (s *hotkeyState) isDown(vk uint32) bool {
	return vk < 256 && s.down[vk/64].Load()&(1<<(vk%64)) != 0
}

func (s *hotkeyState) setDown(vk uint32, d bool) {
	if vk >= 256 {
		return
	}
	w, bit := &s.down[vk/64], uint64(1)<<(vk%64)
	for {
		old := w.Load()
		nw := old &^ bit
		if d {
			nw = old | bit
		}
		if w.CompareAndSwap(old, nw) {
			return
		}
	}
}

func isWinVK(vk uint32) bool { return vk == config.VKLWin || vk == config.VKRWin }

// key handles one physical key event. Nothing is swallowed; the caller
// passes every event on.
func (s *hotkeyState) key(vk uint32, down bool) {
	was := s.isDown(vk)
	s.setDown(vk, down)
	if s.hk.IsCombo() {
		s.combo(vk, down, was)
	} else if down && !was && vk == s.hk.Trigger && s.modsMatch() {
		s.trigger()
	}
	if !down && isWinVK(vk) && s.maskWin.Load() {
		s.maskWin.Store(false)
		s.mask()
	}
}

func (s *hotkeyState) trigger() {
	if s.hk.HasWin() {
		s.maskWin.Store(true)
	}
	s.fire()
}

func (s *hotkeyState) combo(vk uint32, down, was bool) {
	a, b := s.hk.Combo[0].Has(vk), s.hk.Combo[1].Has(vk)
	if down {
		if !a && !b {
			if s.aDown.Load() || s.bDown.Load() {
				s.clean.Store(false)
				s.armed.Store(false)
			}
			return
		}
		if was {
			return // auto-repeat
		}
		if !s.aDown.Load() && !s.bDown.Load() {
			s.clean.Store(true)
		}
		if a {
			s.aDown.Store(true)
		} else {
			s.bDown.Store(true)
		}
		if s.aDown.Load() && s.bDown.Load() && s.clean.Load() {
			s.armed.Store(true)
		}
		return
	}
	if !a && !b {
		return
	}
	// clean stays set: tapping the released key again while the other is
	// held fires again.
	if s.armed.Load() {
		s.armed.Store(false)
		s.trigger()
	}
	if a {
		s.aDown.Store(false)
	} else {
		s.bDown.Store(false)
	}
}

func sideOK(side config.Side, l, r bool) bool {
	switch side {
	case config.SideNone:
		return !l && !r
	case config.SideLeft:
		return l
	case config.SideRight:
		return r
	default:
		return l || r
	}
}

func (s *hotkeyState) modsMatch() bool {
	h := s.hk
	return sideOK(h.Win, s.isDown(config.VKLWin), s.isDown(config.VKRWin)) &&
		sideOK(h.Alt, s.isDown(config.VKLAlt), s.isDown(config.VKRAlt)) &&
		sideOK(h.Ctrl, s.isDown(config.VKLControl), s.isDown(config.VKRControl)) &&
		sideOK(h.Shift, s.isDown(config.VKLShift), s.isDown(config.VKRShift))
}

var (
	hook       hotkeyState
	hookHandle uintptr
	hookProcCB = syscall.NewCallback(hookProc)

	// Resolved before the hook is installed so the callback calls nothing
	// that can fail or allocate.
	addrCallNextHookEx uintptr
	addrPostMessage    uintptr
	addrSendInput      uintptr
	hookHwnd           uintptr
	maskInputs         = [2]keyInput{
		{typ: inputKeyboard, wVk: vkControl},
		{typ: inputKeyboard, wVk: vkControl, dwFlags: keyeventfUp},
	}
)

func hookProc(code, wParam, lParam uintptr) uintptr {
	if int32(code) == hcAction {
		k := (*kbdLLHook)(win.PtrFromUintptr(lParam))
		if k.flags&llkhfInjected == 0 {
			hook.key(k.vkCode, wParam == wmKeyDown || wParam == wmSysKeyDown)
		}
	}
	r, _, _ := syscall.SyscallN(addrCallNextHookEx, 0, code, wParam, lParam)
	return r
}

func postHotkey() {
	syscall.SyscallN(addrPostMessage, hookHwnd, wmHotkey, 0, 0)
}

func sendMask() {
	syscall.SyscallN(addrSendInput, uintptr(len(maskInputs)), uintptr(unsafe.Pointer(&maskInputs[0])), unsafe.Sizeof(maskInputs[0]))
}

// installHook registers WH_KEYBOARD_LL on the calling thread, which must
// be the window thread running the message loop.
func installHook(hwnd uintptr, hk config.Hotkey, hinst uintptr) error {
	for _, p := range []interface{ Find() error }{procCallNextHookEx, procPostMessageW, procSendInput} {
		if err := p.Find(); err != nil {
			return err
		}
	}
	addrCallNextHookEx = procCallNextHookEx.Addr()
	addrPostMessage = procPostMessageW.Addr()
	addrSendInput = procSendInput.Addr()
	hookHwnd = hwnd
	hook.hk, hook.fire, hook.mask = hk, postHotkey, sendMask
	h, _, err := procSetWindowsHookExW.Call(whKeyboardLL, hookProcCB, hinst, 0)
	if h == 0 {
		return err
	}
	hookHandle = h
	return nil
}

func uninstallHook() {
	if hookHandle != 0 {
		procUnhookWindowsHookEx.Call(hookHandle)
		hookHandle = 0
	}
}
