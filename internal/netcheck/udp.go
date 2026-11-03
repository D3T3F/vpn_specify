package netcheck

import (
	"encoding/binary"
	"errors"
	"io"
	"math/rand/v2"
	"net"
	"strconv"
	"strings"
	"time"

	"golang.org/x/net/dns/dnsmessage"

	"vpn_specify/internal/i18n"
	"vpn_specify/internal/proxy"
)

// SocksUDPExchange sends a datagram to dst ("host:port") through SOCKS5 UDP
// ASSOCIATE and returns the first reply.
func SocksUDPExchange(socksAddr, dst string, payload []byte, timeout time.Duration) ([]byte, error) {
	ctrl, err := net.DialTimeout("tcp", socksAddr, timeout)
	if err != nil {
		return nil, err
	}
	defer ctrl.Close()
	_ = ctrl.SetDeadline(time.Now().Add(timeout))
	if _, err := ctrl.Write([]byte{5, 1, 0}); err != nil {
		return nil, err
	}
	var m [2]byte
	if _, err := io.ReadFull(ctrl, m[:]); err != nil || m[1] != 0 {
		return nil, i18n.Error("SOCKS handshake failed")
	}
	if _, err := ctrl.Write([]byte{5, 3, 0, 1, 0, 0, 0, 0, 0, 0}); err != nil {
		return nil, err
	}
	var rep [10]byte
	if _, err := io.ReadFull(ctrl, rep[:]); err != nil {
		return nil, err
	}
	if rep[1] != 0 || rep[3] != 1 {
		return nil, i18n.Error("UDP ASSOCIATE refused (code {0})", rep[1])
	}
	relay := &net.UDPAddr{IP: net.IP(rep[4:8]), Port: int(binary.BigEndian.Uint16(rep[8:10]))}

	uc, err := net.DialUDP("udp", nil, relay)
	if err != nil {
		return nil, err
	}
	defer uc.Close()
	_ = uc.SetDeadline(time.Now().Add(timeout))

	host, portStr, err := net.SplitHostPort(dst)
	if err != nil {
		return nil, err
	}
	port, _ := strconv.Atoi(portStr)
	var hdr []byte
	if ip := net.ParseIP(host); ip != nil {
		hdr = proxy.SocksUDPHeader(&net.UDPAddr{IP: ip, Port: port})
	} else {
		hdr = append([]byte{0, 0, 0, 3, byte(len(host))}, host...)
		hdr = binary.BigEndian.AppendUint16(hdr, uint16(port))
	}
	if _, err := uc.Write(append(hdr, payload...)); err != nil {
		return nil, err
	}
	buf := make([]byte, 64*1024)
	n, err := uc.Read(buf)
	if err != nil {
		return nil, err
	}
	_, _, data, err := proxy.ParseSocksUDP(buf[:n])
	return data, err
}

// Public IP seen over UDP: a TXT query for o-o.myaddr.l.google.com sent
// straight to Google's authoritative server, which answers with the packet's
// source IP.
const myIPDNSServer = "216.239.32.10:53" // ns1.google.com

func myIPQuery() (id uint16, pkt []byte) {
	id = uint16(rand.Uint32())
	q := dnsmessage.Message{
		Header: dnsmessage.Header{ID: id},
		Questions: []dnsmessage.Question{{
			Name: dnsmessage.MustNewName("o-o.myaddr.l.google.com."), Type: dnsmessage.TypeTXT, Class: dnsmessage.ClassINET,
		}},
	}
	pkt, _ = q.Pack()
	return id, pkt
}

func parseMyIPAnswer(id uint16, b []byte) (string, error) {
	var m dnsmessage.Message
	if err := m.Unpack(b); err != nil {
		return "", err
	}
	if m.ID != id {
		return "", errors.New("reply with the wrong ID")
	}
	for _, a := range m.Answers {
		if t, ok := a.Body.(*dnsmessage.TXTResource); ok && len(t.TXT) > 0 {
			return strings.TrimSpace(t.TXT[0]), nil
		}
	}
	return "", errors.New("reply without TXT")
}

func UDPDirect() (string, error) {
	id, pkt := myIPQuery()
	c, err := net.DialTimeout("udp", myIPDNSServer, 5*time.Second)
	if err != nil {
		return "", err
	}
	defer c.Close()
	_ = c.SetDeadline(time.Now().Add(5 * time.Second))
	if _, err := c.Write(pkt); err != nil {
		return "", err
	}
	buf := make([]byte, 4096)
	n, err := c.Read(buf)
	if err != nil {
		return "", err
	}
	return parseMyIPAnswer(id, buf[:n])
}

func UDPViaSocks(socksAddr string) (string, error) {
	id, pkt := myIPQuery()
	resp, err := SocksUDPExchange(socksAddr, myIPDNSServer, pkt, 8*time.Second)
	if err != nil {
		return "", err
	}
	return parseMyIPAnswer(id, resp)
}
