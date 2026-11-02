package tunnel

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/netip"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"vpn_specify/internal/platform"
)

// Modo OpenVPN
//
// Usa o openvpn.exe instalado, mas impede que ele mexa no roteamento do
// Windows:
//   - --route-nopull + pull-filter: ignora rotas, redirect-gateway, DNS e
//     block-outside-dns enviados pelo servidor;
//   - as mesmas diretivas são removidas do .ovpn (numa cópia temporária);
//   - adiciona só uma rota padrão pelo túnel com métrica 9999. Como a rota
//     normal do PC tem métrica bem menor, o tráfego do sistema continua indo
//     pela rede normal.
//
// O proxy abre as conexões com o IP local do túnel como origem. O Windows
// usa o modelo "strong host" no envio: um socket ligado ao IP do túnel só
// pode sair pela interface do túnel, e usa aquela rota de métrica 9999.
// Assim, só o tráfego do proxy passa pela VPN.

var ovpnStripDirectives = map[string]bool{
	"redirect-gateway":  true,
	"redirect-private":  true,
	"block-outside-dns": true,
	"dhcp-option":       true,
	"route":             true,
	"route-ipv6":        true,
	"register-dns":      true,
	"route-nopull":      true,
}

type ovpnTunnel struct {
	cmd     *exec.Cmd
	localIP net.IP
	dns     []netip.Addr
	tmpDir  string
	exited  chan struct{}
	done    chan error
	once    sync.Once
	stopEv  func() // sinaliza o openvpn para sair (modo sem console)

	cacheMu sync.Mutex
	cache   map[string]dnsCacheEntry
}

type dnsCacheEntry struct {
	ips     []net.IP
	expires time.Time
}

func findOpenVPN(explicit string) (string, error) {
	if explicit != "" {
		return explicit, nil
	}
	if p, err := exec.LookPath("openvpn"); err == nil {
		return p, nil
	}
	for _, c := range []string{`%ProgramFiles%\OpenVPN\bin\openvpn.exe`, `%ProgramFiles(x86)%\OpenVPN\bin\openvpn.exe`} {
		p := platform.ExpandEnv(c)
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	return "", errors.New("openvpn.exe não encontrado; instale o OpenVPN Community (https://openvpn.net/community-downloads/) ou use -openvpn <caminho>")
}

// sanitizeOVPN remove do .ovpn tudo que mexeria nas rotas ou no DNS do
// sistema. Também devolve DNS/ifconfig definidos localmente, como reserva.
func sanitizeOVPN(data []byte) (out []byte, dns []netip.Addr, ifconfig net.IP) {
	var b bytes.Buffer
	inBlock := ""
	for _, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimRight(raw, "\r")
		l := strings.TrimSpace(line)
		if inBlock != "" {
			if strings.EqualFold(l, "</"+inBlock+">") {
				inBlock = ""
			}
			b.WriteString(line + "\n")
			continue
		}
		if strings.HasPrefix(l, "<") && strings.HasSuffix(l, ">") && !strings.HasPrefix(l, "</") {
			inBlock = strings.Trim(l, "<>")
			b.WriteString(line + "\n")
			continue
		}
		f := strings.Fields(l)
		if len(f) > 0 && !strings.HasPrefix(f[0], "#") && !strings.HasPrefix(f[0], ";") {
			d := strings.ToLower(strings.TrimPrefix(f[0], "--"))
			if d == "dhcp-option" && len(f) >= 3 && strings.EqualFold(f[1], "DNS") {
				if a, err := netip.ParseAddr(f[2]); err == nil && a.Is4() {
					dns = append(dns, a)
				}
			}
			if d == "ifconfig" && len(f) >= 2 {
				ifconfig = net.ParseIP(f[1]).To4()
			}
			if ovpnStripDirectives[d] {
				b.WriteString("# [vpn_specify removeu] " + l + "\n")
				continue
			}
		}
		b.WriteString(line + "\n")
	}
	return b.Bytes(), dns, ifconfig
}

// OpenVPNNeedsAuth diz se o .ovpn pede usuário/senha sem arquivo de credenciais.
func OpenVPNNeedsAuth(data []byte) bool {
	for _, line := range strings.Split(string(data), "\n") {
		f := strings.Fields(line)
		if len(f) == 1 && strings.EqualFold(strings.TrimPrefix(f[0], "--"), "auth-user-pass") {
			return true
		}
	}
	return false
}

// parsePushReply extrai o IP do túnel e os DNS da mensagem PUSH_REPLY que o
// OpenVPN registra no log (antes dos filtros serem aplicados).
func parsePushReply(line string) (ip net.IP, dns []netip.Addr) {
	i := strings.Index(line, "PUSH_REPLY,")
	if i < 0 {
		return nil, nil
	}
	s := strings.TrimRight(line[i+len("PUSH_REPLY,"):], "' \r")
	for _, opt := range strings.Split(s, ",") {
		f := strings.Fields(opt)
		switch {
		case len(f) >= 2 && f[0] == "ifconfig":
			ip = net.ParseIP(f[1]).To4()
		case len(f) >= 3 && f[0] == "dhcp-option" && strings.EqualFold(f[1], "DNS"):
			if a, err := netip.ParseAddr(f[2]); err == nil && a.Is4() {
				dns = append(dns, a)
			}
		}
	}
	return ip, dns
}

func openOpenVPN(path string, data []byte, o Options) (Tunnel, error) {
	exe, err := findOpenVPN(o.OpenVPNPath)
	if err != nil {
		return nil, err
	}
	if !platform.IsElevated() {
		return nil, errors.New("o modo OpenVPN precisa de um terminal aberto como Administrador (o OpenVPN cria um adaptador de rede). " +
			"O modo WireGuard não precisa")
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	sanitized, cfgDNS, cfgIP := sanitizeOVPN(data)
	if o.NoConsole && o.AuthUser == "" && OpenVPNNeedsAuth(data) {
		return nil, errors.New("esta VPN pede usuário e senha: preencha as credenciais da configuração")
	}
	tmpDir, err := os.MkdirTemp("", "vpn_specify-")
	if err != nil {
		return nil, err
	}
	var authArgs []string
	if o.AuthUser != "" {
		// remove o auth-user-pass do .ovpn e passa um arquivo temporário (apagado ao desconectar)
		var b strings.Builder
		for _, line := range strings.Split(string(sanitized), "\n") {
			if f := strings.Fields(line); len(f) > 0 && strings.EqualFold(strings.TrimPrefix(f[0], "--"), "auth-user-pass") {
				continue
			}
			b.WriteString(line + "\n")
		}
		sanitized = []byte(b.String())
		authFile := filepath.Join(tmpDir, "auth.txt")
		if err := os.WriteFile(authFile, []byte(o.AuthUser+"\n"+o.AuthPass+"\n"), 0o600); err != nil {
			os.RemoveAll(tmpDir)
			return nil, err
		}
		authArgs = []string{"--auth-user-pass", authFile}
	}
	tmpConf := filepath.Join(tmpDir, "config.ovpn")
	if err := os.WriteFile(tmpConf, sanitized, 0o600); err != nil {
		os.RemoveAll(tmpDir)
		return nil, err
	}

	args := []string{
		"--cd", filepath.Dir(absPath), // caminhos relativos do .ovpn (ca, cert, auth) continuam funcionando
		// Os filtros vêm antes do --config para terem prioridade.
		"--pull-filter", "ignore", "redirect-gateway",
		"--pull-filter", "ignore", "block-outside-dns",
		"--pull-filter", "ignore", "dhcp-option",
		"--pull-filter", "ignore", "register-dns",
		"--config", tmpConf,
		"--route-nopull",
		"--route", "0.0.0.0", "0.0.0.0", "vpn_gateway", "9999",
		"--verb", "3",
	}
	args = append(args, authArgs...)
	var stopEvent func()
	if o.NoConsole {
		// Sem console não há Ctrl+C: o openvpn sai de forma limpa (removendo
		// as rotas) quando sinalizamos este evento.
		name, signal, err := platform.NewStopEvent()
		if err != nil {
			os.RemoveAll(tmpDir)
			return nil, err
		}
		args = append(args, "--service", name, "0")
		stopEvent = signal
	}
	cmd := exec.Command(exe, args...)
	if o.NoConsole {
		platform.HideConsole(cmd)
		cmd.Stderr = logWriter("[openvpn] ")
	} else {
		cmd.Stdin = os.Stdin // permite digitar usuário/senha se o .ovpn pedir
		cmd.Stderr = os.Stderr
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		os.RemoveAll(tmpDir)
		return nil, err
	}
	log.Printf("OpenVPN: iniciando %s", exe)
	if err := cmd.Start(); err != nil {
		os.RemoveAll(tmpDir)
		return nil, fmt.Errorf("iniciando openvpn: %w", err)
	}
	if err := platform.BindChildToUs(cmd.Process); err != nil {
		log.Printf("aviso: não consegui vincular o openvpn a este processo (%v); se o vpn_specify for morto, encerre o openvpn.exe manualmente", err)
	}

	t := &ovpnTunnel{
		cmd: cmd, tmpDir: tmpDir, stopEv: stopEvent,
		exited: make(chan struct{}), done: make(chan error, 1),
		cache: map[string]dnsCacheEntry{},
	}
	go func() {
		err := cmd.Wait()
		if err == nil {
			err = errors.New("o processo openvpn terminou")
		}
		close(t.exited)
		t.done <- err
	}()

	var (
		mu        sync.Mutex
		pushedIP  net.IP
		pushedDNS []netip.Addr
		readyOnce sync.Once
		ready     = make(chan struct{})
		failed    = make(chan error, 1)
	)
	go scanOutput(stdout, func(line string) {
		if o.Verbose || isImportantOVPNLine(line) {
			log.Printf("[openvpn] %s", line)
		}
		if strings.Contains(line, "PUSH_REPLY,") {
			ip, dns := parsePushReply(line)
			mu.Lock()
			if ip != nil {
				pushedIP = ip
			}
			pushedDNS = append(pushedDNS, dns...)
			mu.Unlock()
		}
		if strings.Contains(line, "needs a gateway parameter") {
			log.Printf("aviso: o OpenVPN não conseguiu criar a rota do túnel; as conexões podem falhar")
		}
		if strings.Contains(line, "AUTH_FAILED") {
			select {
			case failed <- errors.New("autenticação recusada pelo servidor (AUTH_FAILED)"):
			default:
			}
		}
		if strings.Contains(line, "Initialization Sequence Completed") {
			readyOnce.Do(func() { close(ready) })
		}
	})

	select {
	case <-ready:
	case err := <-failed:
		t.kill()
		return nil, err
	case err := <-t.done:
		t.kill()
		return nil, fmt.Errorf("openvpn falhou ao conectar: %w (rode com -v para ver o log completo)", err)
	case <-time.After(5 * time.Minute):
		t.kill()
		return nil, errors.New("tempo esgotado esperando o OpenVPN conectar")
	}

	mu.Lock()
	t.localIP, t.dns = pushedIP, pushedDNS
	mu.Unlock()
	if t.localIP == nil {
		t.localIP = cfgIP
	}
	if len(t.dns) == 0 {
		t.dns = cfgDNS
	}
	if len(o.DNS) > 0 {
		t.dns = o.DNS
	}
	if t.localIP == nil {
		t.kill()
		return nil, errors.New("não consegui descobrir o IP do túnel (o servidor não enviou ifconfig); rode com -v e me mande o log")
	}
	if err := waitForLocalAddr(t.localIP, 30*time.Second); err != nil {
		t.kill()
		return nil, err
	}
	if len(t.dns) == 0 {
		log.Printf("aviso: a VPN não enviou servidores DNS; nomes serão resolvidos pelo DNS normal do PC (use -dns para mudar)")
	}
	log.Printf("OpenVPN: conectado. IP do túnel %s, DNS %v", t.localIP, t.dns)
	return t, nil
}

func isImportantOVPNLine(l string) bool {
	for _, k := range []string{"ERROR", "Error", "error:", "AUTH_FAILED", "Exiting", "WARNING", "Options error", "Initialization Sequence Completed", "Enter Auth", "Enter Private Key"} {
		if strings.Contains(l, k) {
			return true
		}
	}
	return false
}

// scanOutput separa a saída em linhas. Um pedaço sem quebra de linha que
// termina em ":" (ex.: "Enter Auth Username:") é mostrado na hora, para o
// usuário ver o prompt.
func scanOutput(r io.Reader, onLine func(string)) {
	buf := make([]byte, 4096)
	var pending []byte
	for {
		n, err := r.Read(buf)
		if n > 0 {
			pending = append(pending, buf[:n]...)
			for {
				i := bytes.IndexByte(pending, '\n')
				if i < 0 {
					break
				}
				onLine(strings.TrimRight(string(pending[:i]), "\r"))
				pending = pending[i+1:]
			}
			if p := bytes.TrimSpace(pending); len(p) > 0 && p[len(p)-1] == ':' {
				os.Stderr.Write(pending)
				pending = nil
			}
		}
		if err != nil {
			if len(pending) > 0 {
				onLine(string(pending))
			}
			return
		}
	}
}

func waitForLocalAddr(ip net.IP, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		addrs, _ := net.InterfaceAddrs()
		for _, a := range addrs {
			if n, ok := a.(*net.IPNet); ok && n.IP.Equal(ip) {
				return nil
			}
		}
		time.Sleep(300 * time.Millisecond)
	}
	return fmt.Errorf("o IP do túnel %s não apareceu em nenhuma interface", ip)
}

func (t *ovpnTunnel) dialer(network string) *net.Dialer {
	d := &net.Dialer{}
	if strings.HasPrefix(network, "udp") {
		d.LocalAddr = &net.UDPAddr{IP: t.localIP}
	} else {
		d.LocalAddr = &net.TCPAddr{IP: t.localIP}
	}
	return d
}

func (t *ovpnTunnel) resolve(ctx context.Context, host string) ([]net.IP, error) {
	if ip := net.ParseIP(host); ip != nil {
		if ip.To4() == nil {
			return nil, fmt.Errorf("%s: IPv6 não é suportado no modo OpenVPN", host)
		}
		return []net.IP{ip}, nil
	}
	t.cacheMu.Lock()
	e, ok := t.cache[host]
	t.cacheMu.Unlock()
	if ok && time.Now().Before(e.expires) {
		return e.ips, nil
	}
	var ips []net.IP
	var err error
	if len(t.dns) == 0 {
		ips, err = net.DefaultResolver.LookupIP(ctx, "ip4", host)
	} else {
		ips, err = lookupA(ctx, t.dns, host, func(ctx context.Context, network, server string) (net.Conn, error) {
			return t.dialer(network).DialContext(ctx, network+"4", server)
		})
	}
	if err != nil {
		return nil, err
	}
	t.cacheMu.Lock()
	t.cache[host] = dnsCacheEntry{ips: ips, expires: time.Now().Add(60 * time.Second)}
	t.cacheMu.Unlock()
	return ips, nil
}

func (t *ovpnTunnel) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}
	ips, err := t.resolve(ctx, host)
	if err != nil {
		return nil, err
	}
	var lastErr error
	for _, ip := range ips {
		c, err := t.dialer("tcp").DialContext(ctx, "tcp4", net.JoinHostPort(ip.String(), port))
		if err == nil {
			return c, nil
		}
		lastErr = err
	}
	return nil, lastErr
}

// ListenUDP liga o socket ao IP do túnel; pelo mesmo motivo do TCP (strong
// host), os datagramas só saem pela interface da VPN.
func (t *ovpnTunnel) ListenUDP() (net.PacketConn, error) {
	return net.ListenUDP("udp4", &net.UDPAddr{IP: t.localIP})
}

func (t *ovpnTunnel) LookupIP(ctx context.Context, host string) (net.IP, error) {
	ips, err := t.resolve(ctx, host)
	if err != nil {
		return nil, err
	}
	return ips[0], nil
}

func (t *ovpnTunnel) Done() <-chan error { return t.done }

func (t *ovpnTunnel) kill() {
	t.once.Do(func() {
		_ = t.cmd.Process.Kill()
		<-t.exited
		os.RemoveAll(t.tmpDir)
	})
}

// Close espera o openvpn sair sozinho (ele também recebe o Ctrl+C do console
// e remove as rotas que criou); se não sair, encerra à força.
func (t *ovpnTunnel) Close() error {
	t.once.Do(func() {
		wait := 5 * time.Second
		if t.stopEv != nil {
			t.stopEv()
			wait = 10 * time.Second
		}
		select {
		case <-t.exited:
		case <-time.After(wait):
			_ = t.cmd.Process.Kill()
			<-t.exited
		}
		os.RemoveAll(t.tmpDir)
	})
	return nil
}
