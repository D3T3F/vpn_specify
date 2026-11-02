//go:build windows

package platform

import (
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

// ---------- Diálogo "Escolher pasta" e perguntas ----------

var (
	shell32                  = windows.NewLazySystemDLL("shell32.dll")
	procSHBrowseForFolderW   = shell32.NewProc("SHBrowseForFolderW")
	procSHGetPathFromIDListW = shell32.NewProc("SHGetPathFromIDListW")
	procCoTaskMemFree        = windows.NewLazySystemDLL("ole32.dll").NewProc("CoTaskMemFree")
	browseCallbackOnce       sync.Once
	browseCallback           uintptr
)

type browseInfo struct {
	hwndOwner      uintptr
	pidlRoot       uintptr
	pszDisplayName *uint16
	lpszTitle      *uint16
	ulFlags        uint32
	lpfn           uintptr
	lParam         uintptr
	iImage         int32
}

// BrowseFolder mostra o diálogo nativo de escolha de pasta, começando em initial.
func BrowseFolder(owner uintptr, title, initial string) (string, bool) {
	browseCallbackOnce.Do(func() {
		// BFFM_INITIALIZED: seleciona a pasta inicial (lpData aponta para o caminho UTF-16)
		browseCallback = windows.NewCallback(func(hwnd, msg, lParam, lpData uintptr) uintptr {
			if msg == 1 && lpData != 0 {
				procSendMessageW.Call(hwnd, 0x0400+103 /*BFFM_SETSELECTIONW*/, 1, lpData)
			}
			return 0
		})
	})
	_ = windows.CoInitializeEx(0, windows.COINIT_APARTMENTTHREADED)
	t, _ := windows.UTF16PtrFromString(title)
	init, _ := windows.UTF16PtrFromString(initial)
	name := make([]uint16, windows.MAX_PATH)
	bi := browseInfo{
		hwndOwner:      owner,
		pszDisplayName: &name[0],
		lpszTitle:      t,
		ulFlags:        0x0001 | 0x0040, // BIF_RETURNONLYFSDIRS | BIF_NEWDIALOGSTYLE
		lpfn:           browseCallback,
		lParam:         uintptr(unsafe.Pointer(init)),
	}
	pidl, _, _ := procSHBrowseForFolderW.Call(uintptr(unsafe.Pointer(&bi)))
	if pidl == 0 {
		return "", false
	}
	defer procCoTaskMemFree.Call(pidl)
	buf := make([]uint16, 32768)
	if r, _, _ := procSHGetPathFromIDListW.Call(pidl, uintptr(unsafe.Pointer(&buf[0]))); r == 0 {
		return "", false
	}
	return windows.UTF16ToString(buf), true
}

// Respostas de Ask.
const (
	AnswerCancel = 2
	AnswerYes    = 6
	AnswerNo     = 7
)

// Ask faz uma pergunta Sim/Não (ou Sim/Não/Cancelar) e devolve AnswerYes,
// AnswerNo ou AnswerCancel.
func Ask(title, text string, withCancel bool) int {
	flags := uintptr(0x4 | 0x20) // MB_YESNO | MB_ICONQUESTION
	if withCancel {
		flags = 0x3 | 0x20 // MB_YESNOCANCEL
	}
	t, _ := windows.UTF16PtrFromString(title)
	m, _ := windows.UTF16PtrFromString(text)
	r, _, _ := procMessageBoxW.Call(0, uintptr(unsafe.Pointer(m)), uintptr(unsafe.Pointer(t)), flags|0x00010000)
	return int(r)
}
