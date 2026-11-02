//go:build windows

package capture

import (
	"encoding/binary"
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Descobre qual processo é dono de uma porta local, pelas tabelas de
// conexões do Windows (as mesmas que o netstat -ano usa).

var (
	iphlpapi        = windows.NewLazySystemDLL("iphlpapi.dll")
	procExtTCPTable = iphlpapi.NewProc("GetExtendedTcpTable")
	procExtUDPTable = iphlpapi.NewProc("GetExtendedUdpTable")
	procSetTcpEntry = iphlpapi.NewProc("SetTcpEntry")
)

const (
	afInet           = 2
	afInet6          = 23
	tcpTableOwnerAll = 5 // TCP_TABLE_OWNER_PID_ALL
	udpTableOwner    = 1 // UDP_TABLE_OWNER_PID
)

func ipTable(proc *windows.LazyProc, af, class uintptr) ([]byte, error) {
	size := uint32(32 * 1024)
	for i := 0; i < 8; i++ {
		buf := make([]byte, size)
		r, _, _ := proc.Call(uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&size)), 0, af, class, 0)
		switch r {
		case 0:
			return buf, nil
		case 122: // ERROR_INSUFFICIENT_BUFFER: size já foi atualizado
			size += 4096
		default:
			return nil, fmt.Errorf("%s: erro %d", proc.Name, r)
		}
	}
	return nil, fmt.Errorf("%s: tabela mudando rápido demais", proc.Name)
}

// Layout das linhas (bytes): porta local em ordem de rede nos 2 primeiros
// bytes do DWORD; PID no último DWORD.
type tableLayout struct {
	rowSize, portOff, pidOff int
}

var (
	layoutTCP4 = tableLayout{24, 8, 20}  // MIB_TCPROW_OWNER_PID: state, laddr, lport, raddr, rport, pid
	layoutTCP6 = tableLayout{56, 20, 52} // MIB_TCP6ROW_OWNER_PID
	layoutUDP4 = tableLayout{12, 4, 8}   // MIB_UDPROW_OWNER_PID
	layoutUDP6 = tableLayout{28, 20, 24} // MIB_UDP6ROW_OWNER_PID
)

// portOwner devolve o PID dono da porta local (0 se não achar).
func portOwner(tcp, v6 bool, port uint16) uint32 {
	proc, class, af, lay := procExtUDPTable, uintptr(udpTableOwner), uintptr(afInet), layoutUDP4
	switch {
	case tcp && !v6:
		proc, class, lay = procExtTCPTable, tcpTableOwnerAll, layoutTCP4
	case tcp && v6:
		proc, class, af, lay = procExtTCPTable, tcpTableOwnerAll, afInet6, layoutTCP6
	case !tcp && v6:
		af, lay = afInet6, layoutUDP6
	}
	b, err := ipTable(proc, af, class)
	if err != nil {
		return 0
	}
	n := int(binary.LittleEndian.Uint32(b[:4]))
	for i := 0; i < n; i++ {
		off := 4 + i*lay.rowSize
		if off+lay.rowSize > len(b) {
			break
		}
		row := b[off : off+lay.rowSize]
		if binary.BigEndian.Uint16(row[lay.portOff:]) == port {
			return binary.LittleEndian.Uint32(row[lay.pidOff:])
		}
	}
	return 0
}

// resetTCPConnections derruba as conexões TCP IPv4 estabelecidas dos PIDs
// dados, para o app reconectar (agora capturado). Precisa de Administrador.
func resetTCPConnections(pids map[uint32]string) int {
	b, err := ipTable(procExtTCPTable, afInet, tcpTableOwnerAll)
	if err != nil {
		return 0
	}
	n := int(binary.LittleEndian.Uint32(b[:4]))
	count := 0
	for i := 0; i < n; i++ {
		off := 4 + i*layoutTCP4.rowSize
		if off+layoutTCP4.rowSize > len(b) {
			break
		}
		row := b[off : off+layoutTCP4.rowSize]
		state := binary.LittleEndian.Uint32(row[0:])
		pid := binary.LittleEndian.Uint32(row[20:])
		if _, ok := pids[pid]; !ok || state != 5 { // 5 = ESTABLISHED
			continue
		}
		var mib [20]byte // MIB_TCPROW: mesmos 5 primeiros DWORDs
		copy(mib[:], row[:20])
		binary.LittleEndian.PutUint32(mib[0:], 12) // MIB_TCP_STATE_DELETE_TCB
		if r, _, _ := procSetTcpEntry.Call(uintptr(unsafe.Pointer(&mib[0]))); r == 0 {
			count++
		}
	}
	return count
}
