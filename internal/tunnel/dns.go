package tunnel

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net"
	"net/netip"
	"strings"
	"time"

	"golang.org/x/net/dns/dnsmessage"
)

// Cliente DNS mínimo (registros A) cujas consultas saem por um dial
// customizado, ou seja, por dentro da VPN. Evita vazar os nomes acessados
// para o DNS normal do computador.

type dialFunc func(ctx context.Context, network, address string) (net.Conn, error)

func lookupA(ctx context.Context, servers []netip.Addr, name string, dial dialFunc) ([]net.IP, error) {
	var lastErr error
	for _, s := range servers {
		ips, err := queryA(ctx, netip.AddrPortFrom(s, 53).String(), name, dial)
		if err == nil && len(ips) > 0 {
			return ips, nil
		}
		if err == nil {
			err = fmt.Errorf("%s: sem endereço IPv4", name)
		}
		lastErr = err
	}
	if lastErr == nil {
		lastErr = errors.New("nenhum servidor DNS configurado")
	}
	return nil, lastErr
}

func queryA(ctx context.Context, server, name string, dial dialFunc) ([]net.IP, error) {
	fq := name
	if !strings.HasSuffix(fq, ".") {
		fq += "."
	}
	n, err := dnsmessage.NewName(fq)
	if err != nil {
		return nil, fmt.Errorf("nome inválido %q", name)
	}
	id := uint16(rand.Uint32())
	q := dnsmessage.Message{
		Header:    dnsmessage.Header{ID: id, RecursionDesired: true},
		Questions: []dnsmessage.Question{{Name: n, Type: dnsmessage.TypeA, Class: dnsmessage.ClassINET}},
	}
	pkt, err := q.Pack()
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	resp, err := dnsExchange(ctx, "udp", server, pkt, dial)
	if err == nil && resp.Truncated {
		resp, err = dnsExchange(ctx, "tcp", server, pkt, dial)
	}
	if err != nil {
		return nil, fmt.Errorf("DNS %s: %w", server, err)
	}
	if resp.ID != id {
		return nil, errors.New("resposta DNS com ID errado")
	}
	switch resp.RCode {
	case dnsmessage.RCodeSuccess:
	case dnsmessage.RCodeNameError:
		return nil, fmt.Errorf("%s: domínio não encontrado", name)
	default:
		return nil, fmt.Errorf("%s: erro DNS %v", name, resp.RCode)
	}
	var ips []net.IP
	for _, a := range resp.Answers {
		if r, ok := a.Body.(*dnsmessage.AResource); ok {
			ips = append(ips, net.IPv4(r.A[0], r.A[1], r.A[2], r.A[3]))
		}
	}
	return ips, nil
}

func dnsExchange(ctx context.Context, network, server string, pkt []byte, dial dialFunc) (*dnsmessage.Message, error) {
	c, err := dial(ctx, network, server)
	if err != nil {
		return nil, err
	}
	defer c.Close()
	if dl, ok := ctx.Deadline(); ok {
		_ = c.SetDeadline(dl)
	}
	var buf []byte
	if network == "tcp" {
		framed := make([]byte, 2+len(pkt))
		binary.BigEndian.PutUint16(framed, uint16(len(pkt)))
		copy(framed[2:], pkt)
		if _, err := c.Write(framed); err != nil {
			return nil, err
		}
		var lb [2]byte
		if _, err := io.ReadFull(c, lb[:]); err != nil {
			return nil, err
		}
		buf = make([]byte, binary.BigEndian.Uint16(lb[:]))
		if _, err := io.ReadFull(c, buf); err != nil {
			return nil, err
		}
	} else {
		if _, err := c.Write(pkt); err != nil {
			return nil, err
		}
		buf = make([]byte, 65535)
		n, err := c.Read(buf)
		if err != nil {
			return nil, err
		}
		buf = buf[:n]
	}
	var m dnsmessage.Message
	if err := m.Unpack(buf); err != nil {
		return nil, err
	}
	return &m, nil
}
