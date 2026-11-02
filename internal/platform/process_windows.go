//go:build windows

package platform

import (
	"path/filepath"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

// ProcessName devolve o nome do executável em minúsculas (ex.: "discord.exe").
func ProcessName(pid uint32) string {
	if p := ProcessPath(pid); p != "" {
		return strings.ToLower(filepath.Base(p))
	}
	return ""
}

// ProcessPath devolve o caminho completo do executável do processo ("" se
// não der para consultar).
func ProcessPath(pid uint32) string {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return ""
	}
	defer windows.CloseHandle(h)
	buf := make([]uint16, 32768)
	n := uint32(len(buf))
	if err := windows.QueryFullProcessImageName(h, 0, &buf[0], &n); err != nil {
		return ""
	}
	return windows.UTF16ToString(buf[:n])
}

// RunningPIDs lista os PIDs cujo executável está em names.
func RunningPIDs(names map[string]bool) map[uint32]string {
	out := map[uint32]string{}
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return out
	}
	defer windows.CloseHandle(snap)
	var e windows.ProcessEntry32
	e.Size = uint32(unsafe.Sizeof(e))
	for err = windows.Process32First(snap, &e); err == nil; err = windows.Process32Next(snap, &e) {
		name := strings.ToLower(windows.UTF16ToString(e.ExeFile[:]))
		if names[name] {
			out[e.ProcessID] = name
		}
	}
	return out
}
