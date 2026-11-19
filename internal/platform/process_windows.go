//go:build windows

package platform

import (
	"os"
	"path/filepath"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

// ProcessName returns the lower-case executable name (e.g. "discord.exe").
func ProcessName(pid uint32) string {
	if p := ProcessPath(pid); p != "" {
		return strings.ToLower(filepath.Base(p))
	}
	return ""
}

// ProcessPath returns the full path of the process executable ("" if it
// cannot be queried).
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

// Snapshot lists all the running processes with their parent PIDs and names.
func Snapshot() ProcessTable {
	t := ProcessTable{Parent: map[uint32]uint32{}, Name: map[uint32]string{}}
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return t
	}
	defer windows.CloseHandle(snap)
	var e windows.ProcessEntry32
	e.Size = uint32(unsafe.Sizeof(e))
	for err = windows.Process32First(snap, &e); err == nil; err = windows.Process32Next(snap, &e) {
		t.Parent[e.ProcessID] = e.ParentProcessID
		t.Name[e.ProcessID] = strings.ToLower(windows.UTF16ToString(e.ExeFile[:]))
	}
	return t
}

// RunningTreePIDs lists the PIDs of the processes in names and of all their
// descendants (see CapturedAncestor), with their names. vpn_specify itself and
// its descendants are never listed.
func RunningTreePIDs(names map[string]bool) map[uint32]string {
	out := map[uint32]string{}
	t := Snapshot()
	self := uint32(os.Getpid())
	for pid, name := range t.Name {
		if _, ok := CapturedAncestor(pid, t, names, self); ok {
			out[pid] = name
		}
	}
	return out
}

// RunningPIDs lists the PIDs whose executable is in names.
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
