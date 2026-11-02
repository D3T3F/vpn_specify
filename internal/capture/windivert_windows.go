//go:build windows

package capture

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Binding mínimo do WinDivert 2.x (https://reqrypt.org/windivert.html),
// carregado de WinDivert.dll ao lado do executável (ou em .\windivert\).

// divertAddr espelha WINDIVERT_ADDRESS (80 bytes).
type divertAddr struct {
	Timestamp int64
	Flags     uint32 // Layer:8 Event:8 Sniffed:1 Outbound:1 Loopback:1 Impostor:1 IPv6:1 ...
	_         uint32
	IfIdx     uint32
	SubIfIdx  uint32
	_         [56]byte
}

const (
	wdOutbound = 1 << 17
	wdIPv6     = 1 << 20
)

func (a *divertAddr) setOutbound(out bool) {
	if out {
		a.Flags |= wdOutbound
	} else {
		a.Flags &^= wdOutbound
	}
}

var (
	wdDLL                                                    *windows.DLL
	wdOpen, wdRecv, wdSend, wdClose, wdShutdown, wdChecksums *windows.Proc
)

func loadWinDivert() error {
	if wdDLL != nil {
		return nil
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	dir := filepath.Dir(exe)
	var found string
	for _, d := range []string{dir, filepath.Join(dir, "windivert")} {
		_, e1 := os.Stat(filepath.Join(d, "WinDivert.dll"))
		_, e2 := os.Stat(filepath.Join(d, "WinDivert64.sys"))
		if e1 == nil && e2 == nil {
			found = d
			break
		}
	}
	if found == "" {
		return fmt.Errorf("WinDivert.dll e WinDivert64.sys não encontrados em %s nem em %s", dir, filepath.Join(dir, "windivert"))
	}
	dll, err := windows.LoadDLL(filepath.Join(found, "WinDivert.dll"))
	if err != nil {
		return fmt.Errorf("carregando WinDivert.dll: %w", err)
	}
	procs := []**windows.Proc{&wdOpen, &wdRecv, &wdSend, &wdClose, &wdShutdown, &wdChecksums}
	names := []string{"WinDivertOpen", "WinDivertRecv", "WinDivertSend", "WinDivertClose", "WinDivertShutdown", "WinDivertHelperCalcChecksums"}
	for i, n := range names {
		p, err := dll.FindProc(n)
		if err != nil {
			return err
		}
		*procs[i] = p
	}
	wdDLL = dll
	return nil
}

var errNoData error = windows.ERROR_NO_DATA // devolvido pelo Recv depois do Shutdown

type divertHandle uintptr

func divertOpen(filter string) (divertHandle, error) {
	if err := loadWinDivert(); err != nil {
		return 0, err
	}
	f, err := windows.BytePtrFromString(filter)
	if err != nil {
		return 0, err
	}
	// layer 0 = WINDIVERT_LAYER_NETWORK, prioridade 0, flags 0
	r, _, callErr := wdOpen.Call(uintptr(unsafe.Pointer(f)), 0, 0, 0)
	if windows.Handle(r) == windows.InvalidHandle {
		var en windows.Errno
		if errors.As(callErr, &en) {
			switch en {
			case windows.ERROR_ACCESS_DENIED:
				return 0, errors.New("WinDivert: acesso negado (rode como Administrador)")
			case windows.ERROR_FILE_NOT_FOUND:
				return 0, errors.New("WinDivert: driver WinDivert64.sys não encontrado")
			case 577, 1275: // ERROR_INVALID_IMAGE_HASH, ERROR_DRIVER_BLOCKED
				return 0, fmt.Errorf("WinDivert: o Windows bloqueou o driver (%v); verifique o antivírus/Integridade de Memória", en)
			}
		}
		return 0, fmt.Errorf("WinDivertOpen: %w", callErr)
	}
	return divertHandle(r), nil
}

func (h divertHandle) recv(buf []byte, addr *divertAddr) (int, error) {
	var n uint32
	r, _, err := wdRecv.Call(uintptr(h), uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)), uintptr(unsafe.Pointer(&n)), uintptr(unsafe.Pointer(addr)))
	if r == 0 {
		return 0, err
	}
	return int(n), nil
}

func (h divertHandle) send(pkt []byte, addr *divertAddr) error {
	var n uint32
	r, _, err := wdSend.Call(uintptr(h), uintptr(unsafe.Pointer(&pkt[0])), uintptr(len(pkt)), uintptr(unsafe.Pointer(&n)), uintptr(unsafe.Pointer(addr)))
	if r == 0 {
		return err
	}
	return nil
}

func divertChecksums(pkt []byte, addr *divertAddr) {
	_, _, _ = wdChecksums.Call(uintptr(unsafe.Pointer(&pkt[0])), uintptr(len(pkt)), uintptr(unsafe.Pointer(addr)), 0)
}

func (h divertHandle) close() {
	_, _, _ = wdShutdown.Call(uintptr(h), 3) // WINDIVERT_SHUTDOWN_BOTH
	_, _, _ = wdClose.Call(uintptr(h))
}
