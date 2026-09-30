//go:build windows

// Package win holds the minimal raw COM and Win32 helpers shared by discover
// and launch. Interfaces are called through their vtable; slot indices are
// taken from the Windows SDK headers.
package win

import (
	"fmt"
	"runtime"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	ole32                = windows.NewLazySystemDLL("ole32.dll")
	procCoCreateInstance = ole32.NewProc("CoCreateInstance")
	procPropVariantClear = ole32.NewProc("PropVariantClear")
)

const (
	CLSCTX_INPROC_SERVER = 0x1
	CLSCTX_LOCAL_SERVER  = 0x4
	CLSCTX_ALL           = 0x17

	coinitApartmentThreaded = 0x2
	rpcEChangedMode         = 0x80010106
)

// Obj is a COM interface pointer.
type Obj = unsafe.Pointer

func MustGUID(s string) windows.GUID {
	g, err := windows.GUIDFromString(s)
	if err != nil {
		panic(err)
	}
	return g
}

type HRESULT uint32

func (h HRESULT) Failed() bool { return int32(h) < 0 }

func (h HRESULT) Error() string {
	return fmt.Sprintf("HRESULT 0x%08X: %v", uint32(h), syscall.Errno(h))
}

// Err returns nil for success codes.
func (h HRESULT) Err() error {
	if h.Failed() {
		return h
	}
	return nil
}

// Call invokes vtable slot idx on obj.
func Call(obj Obj, idx int, args ...uintptr) HRESULT {
	vtbl := *(*unsafe.Pointer)(obj)
	fn := *(*uintptr)(unsafe.Add(vtbl, uintptr(idx)*unsafe.Sizeof(uintptr(0))))
	r, _, _ := syscall.SyscallN(fn, append([]uintptr{uintptr(obj)}, args...)...)
	return HRESULT(r)
}

func Release(obj Obj) {
	if obj != nil {
		Call(obj, 2)
	}
}

func QueryInterface(obj Obj, iid *windows.GUID) (Obj, error) {
	var out Obj
	if err := Call(obj, 0, uintptr(unsafe.Pointer(iid)), uintptr(unsafe.Pointer(&out))).Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func CreateInstance(clsid, iid *windows.GUID, ctx uint32) (Obj, error) {
	var out Obj
	r, _, _ := procCoCreateInstance.Call(
		uintptr(unsafe.Pointer(clsid)), 0, uintptr(ctx),
		uintptr(unsafe.Pointer(iid)), uintptr(unsafe.Pointer(&out)))
	if err := HRESULT(r).Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// WithCOM runs fn on a locked OS thread with an STA apartment.
func WithCOM(fn func() error) error {
	errc := make(chan error, 1)
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		err := windows.CoInitializeEx(0, coinitApartmentThreaded)
		initialized := true
		if err != nil {
			// S_FALSE (already initialized) still needs CoUninitialize.
			if errno, ok := err.(syscall.Errno); ok && errno == 1 {
				err = nil
			} else if ok && uint32(errno) == rpcEChangedMode {
				err, initialized = nil, false
			}
		}
		if err != nil {
			errc <- fmt.Errorf("CoInitializeEx: %w", err)
			return
		}
		if initialized {
			defer windows.CoUninitialize()
		}
		errc <- fn()
	}()
	return <-errc
}

// PROPVARIANT, amd64 layout (24 bytes).
type PropVariant struct {
	VT       uint16
	reserved [3]uint16
	Val      uintptr
	pad      uintptr
}

const VT_LPWSTR = 31

func (p *PropVariant) String() string {
	if p.VT != VT_LPWSTR || p.Val == 0 {
		return ""
	}
	return windows.UTF16PtrToString(*(**uint16)(unsafe.Pointer(&p.Val)))
}

func (p *PropVariant) Clear() {
	procPropVariantClear.Call(uintptr(unsafe.Pointer(p)))
}

type PropertyKey struct {
	FmtID windows.GUID
	PID   uint32
}

// PtrFromUintptr converts a pointer received as uintptr (lParam, callback
// arguments) without tripping go vet's unsafeptr check.
func PtrFromUintptr(u uintptr) unsafe.Pointer {
	return *(*unsafe.Pointer)(unsafe.Pointer(&u))
}

// CoTaskString copies and frees a CoTaskMemAlloc'd UTF-16 string.
func CoTaskString(p *uint16) string {
	if p == nil {
		return ""
	}
	s := windows.UTF16PtrToString(p)
	windows.CoTaskMemFree(unsafe.Pointer(p))
	return s
}
