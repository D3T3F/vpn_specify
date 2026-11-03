//go:build windows

package platform

import (
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"
)

// ---------- "Open file" dialog ----------

type openFileName struct {
	lStructSize       uint32
	hwndOwner         uintptr
	hInstance         uintptr
	lpstrFilter       *uint16
	lpstrCustomFilter *uint16
	nMaxCustFilter    uint32
	nFilterIndex      uint32
	lpstrFile         *uint16
	nMaxFile          uint32
	lpstrFileTitle    *uint16
	nMaxFileTitle     uint32
	lpstrInitialDir   *uint16
	lpstrTitle        *uint16
	Flags             uint32
	nFileOffset       uint16
	nFileExtension    uint16
	lpstrDefExt       *uint16
	lCustData         uintptr
	lpfnHook          uintptr
	lpTemplateName    *uint16
	pvReserved        uintptr
	dwReserved        uint32
	FlagsEx           uint32
}

// OpenFileDialog shows the native dialog. filters: pairs "description", "*.ext;*.ext".
// noDeref: when a .lnk shortcut is chosen, return the .lnk itself.
func OpenFileDialog(owner uintptr, title, initialDir string, noDeref bool, filters ...string) (string, bool) {
	var f []uint16
	for _, s := range filters {
		f = append(f, utf16.Encode([]rune(s))...)
		f = append(f, 0)
	}
	f = append(f, 0)
	buf := make([]uint16, 4096)
	t, _ := windows.UTF16PtrFromString(title)
	ofn := openFileName{
		hwndOwner:   owner,
		lpstrFilter: &f[0],
		lpstrFile:   &buf[0],
		nMaxFile:    uint32(len(buf)),
		lpstrTitle:  t,
		Flags:       0x1000 | 0x800 | 0x8 | 0x80000, // FILEMUSTEXIST|PATHMUSTEXIST|NOCHANGEDIR|EXPLORER
	}
	ofn.lStructSize = uint32(unsafe.Sizeof(ofn))
	if initialDir != "" {
		ofn.lpstrInitialDir, _ = windows.UTF16PtrFromString(initialDir)
	}
	if noDeref {
		ofn.Flags |= 0x100000 // OFN_NODEREFERENCELINKS
	}
	r, _, _ := procGetOpenFileNameW.Call(uintptr(unsafe.Pointer(&ofn)))
	if r == 0 {
		return "", false
	}
	return windows.UTF16ToString(buf), true
}

// ---------- Messages and window ----------

func MessageBox(title, text string, flags uintptr) {
	t, _ := windows.UTF16PtrFromString(title)
	m, _ := windows.UTF16PtrFromString(text)
	_, _, _ = procMessageBoxW.Call(0, uintptr(unsafe.Pointer(m)), uintptr(unsafe.Pointer(t)), flags|0x00010000) // MB_SETFOREGROUND
}

// SetWindowIcons applies the embedded icon (resource 1) to the title bar and
// the taskbar, each at the right size for the current DPI.
func SetWindowIcons(hwnd uintptr) {
	var hinst windows.Handle
	if windows.GetModuleHandleEx(0, nil, &hinst) != nil {
		return
	}
	for _, s := range []struct{ which, cx, cy uintptr }{{0, 49, 50}, {1, 11, 12}} { // ICON_SMALL: SM_CXSMICON; ICON_BIG: SM_CXICON
		cx, _, _ := procGetSystemMetrics.Call(s.cx)
		cy, _, _ := procGetSystemMetrics.Call(s.cy)
		if h, _, _ := procLoadImageW.Call(uintptr(hinst), 1, 1 /*IMAGE_ICON*/, cx, cy, 0x8000 /*LR_SHARED*/); h != 0 {
			procSendMessageW.Call(hwnd, 0x0080 /*WM_SETICON*/, s.which, h)
		}
	}
}

func BringToFront(hwnd uintptr) {
	_, _, _ = procShowWindow.Call(hwnd, 9) // SW_RESTORE
	_, _, _ = procSetForegroundWindow.Call(hwnd)
}

// AllowForeground lets the running instance bring its window to the front.
func AllowForeground() { _, _, _ = procAllowSetForegroundWindow.Call(uintptr(^uint32(0))) }
