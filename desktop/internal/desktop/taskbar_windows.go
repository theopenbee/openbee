package desktop

import (
	"errors"
	"fmt"
	"syscall"
	"unsafe"

	winapi "golang.org/x/sys/windows"
)

const (
	clsctxInprocServer = 1
	pngIconVersion     = 0x00030000
	taskbarRelease     = 2
	taskbarHrInit      = 3
	taskbarSetOverlay  = 18
)

var (
	ole32                        = winapi.NewLazySystemDLL("ole32.dll")
	user32                       = winapi.NewLazySystemDLL("user32.dll")
	procCoCreateInstance         = ole32.NewProc("CoCreateInstance")
	procCreateIconFromResourceEx = user32.NewProc("CreateIconFromResourceEx")
	procDestroyIcon              = user32.NewProc("DestroyIcon")
	clsidTaskbarList             = winapi.GUID{Data1: 0x56fdf344, Data2: 0xfd6d, Data3: 0x11d0, Data4: [8]byte{0x95, 0x8a, 0x00, 0x60, 0x97, 0xc9, 0xa0, 0x90}}
	iidTaskbarList3              = winapi.GUID{Data1: 0xea1afb91, Data2: 0x9e28, Data3: 0x4b86, Data4: [8]byte{0x90, 0xe9, 0x9e, 0x9f, 0x8a, 0x5e, 0xef, 0xaf}}
	taskbarList                  uintptr
)

func native(addr uintptr) unsafe.Pointer {
	return *(*unsafe.Pointer)(unsafe.Pointer(&addr))
}

func comCall(obj uintptr, index int, args ...uintptr) uintptr {
	vtable := *(*uintptr)(native(obj))
	method := *(*uintptr)(unsafe.Add(native(vtable), index*int(unsafe.Sizeof(uintptr(0)))))
	r, _, _ := syscall.SyscallN(method, append([]uintptr{obj}, args...)...)
	return r
}

func failed(hr uintptr) bool {
	return int32(uint32(hr)) < 0
}

func taskbar() (uintptr, error) {
	if taskbarList != 0 {
		return taskbarList, nil
	}
	var tl uintptr
	hr, _, _ := procCoCreateInstance.Call(uintptr(unsafe.Pointer(&clsidTaskbarList)), 0, clsctxInprocServer,
		uintptr(unsafe.Pointer(&iidTaskbarList3)), uintptr(unsafe.Pointer(&tl)))
	if failed(hr) || tl == 0 {
		return 0, fmt.Errorf("taskbar overlay: %#x", hr)
	}
	if hr := comCall(tl, taskbarHrInit); failed(hr) {
		comCall(tl, taskbarRelease)
		return 0, fmt.Errorf("taskbar overlay: %#x", hr)
	}
	taskbarList = tl
	return tl, nil
}

func setOverlay(hwnd uintptr, png []byte, description string) error {
	if hwnd == 0 {
		return nil
	}
	tl, err := taskbar()
	if err != nil {
		return err
	}
	var icon uintptr
	if len(png) > 0 {
		icon, _, _ = procCreateIconFromResourceEx.Call(uintptr(unsafe.Pointer(&png[0])), uintptr(len(png)), 1, pngIconVersion, 0, 0, 0)
		if icon == 0 {
			return errors.New("taskbar overlay: cannot create an icon from the image")
		}
		defer procDestroyIcon.Call(icon)
	}
	text, err := winapi.UTF16PtrFromString(description)
	if err != nil {
		return err
	}
	if hr := comCall(tl, taskbarSetOverlay, hwnd, icon, uintptr(unsafe.Pointer(text))); failed(hr) {
		return fmt.Errorf("taskbar overlay: %#x", hr)
	}
	return nil
}
