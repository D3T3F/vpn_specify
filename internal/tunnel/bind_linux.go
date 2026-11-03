//go:build linux

package tunnel

import (
	"fmt"
	"log"
	"net"
	"os"
	"syscall"

	"golang.org/x/sys/unix"
)

// No Linux o envio segue o modelo "weak host": ligar o socket ao IP do túnel
// não basta, o kernel escolheria a rota padrão normal (de métrica menor) e o
// pacote sairia pela rede comum com o IP do túnel como origem. Por isso os
// sockets do proxy são presos à interface do túnel com SO_BINDTODEVICE
// (o modo OpenVPN já roda como root, que é o que essa opção exige).

// interfaceWithIP devolve o nome da interface que tem esse IP (ex.: tun0).
func interfaceWithIP(ip net.IP) (string, error) {
	ifs, err := net.Interfaces()
	if err != nil {
		return "", err
	}
	for _, ifc := range ifs {
		addrs, _ := ifc.Addrs()
		for _, a := range addrs {
			if n, ok := a.(*net.IPNet); ok && n.IP.Equal(ip) {
				return ifc.Name, nil
			}
		}
	}
	return "", fmt.Errorf("nenhuma interface com o IP %s", ip)
}

// prepareTunnelInterface descobre a interface do túnel e deixa o rp_filter
// dela em modo "loose". Em distribuições com rp_filter estrito (Fedora, RHEL)
// as respostas que chegam pelo túnel seriam descartadas, porque a melhor rota
// de volta para a internet é a da rede normal, não a do túnel.
func prepareTunnelInterface(ip net.IP) (string, error) {
	name, err := interfaceWithIP(ip)
	if err != nil {
		return "", err
	}
	if err := os.WriteFile("/proc/sys/net/ipv4/conf/"+name+"/rp_filter", []byte("2"), 0o644); err != nil {
		log.Printf("aviso: não consegui ajustar o rp_filter de %s (%v); se as respostas não chegarem, rode: sysctl -w net.ipv4.conf.%s.rp_filter=2", name, err, name)
	}
	return name, nil
}

// bindControl prende o socket à interface ifName.
func bindControl(ifName string) func(network, address string, c syscall.RawConn) error {
	if ifName == "" {
		return nil
	}
	return func(network, address string, c syscall.RawConn) error {
		var serr error
		if err := c.Control(func(fd uintptr) {
			serr = unix.BindToDevice(int(fd), ifName)
		}); err != nil {
			return err
		}
		if serr != nil {
			return fmt.Errorf("SO_BINDTODEVICE %s: %w", ifName, serr)
		}
		return nil
	}
}
