package tunnel

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log"
	"net"
	"net/netip"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	"vpn_specify/internal/i18n"
	"vpn_specify/internal/platform"
)

// OpenVPN mode
//
// Runs the installed openvpn, but keeps it from touching the system routing:
//   - --route-nopull + pull-filter: ignore routes, redirect-gateway, DNS and
//     block-outside-dns pushed by the server;
//   - the same directives are removed from the .ovpn (in a temporary copy);
//   - only a default route through the tunnel with metric 9999 is added. The
//     computer's normal route has a much lower metric, so system traffic
//     keeps using the normal network.
//
// The proxy opens connections with the tunnel's local IP as source. Windows
// uses the "strong host" model when sending: a socket bound to the tunnel IP
// can only leave through the tunnel interface, using that metric-9999 route.
// So only the proxy's traffic goes through the VPN.
//
// That does not hold on Linux ("weak host"): there the sockets are also bound
// to the tunnel interface with SO_BINDTODEVICE (see bind_linux.go).

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
	ifName  string // tunnel interface; only used on Linux (SO_BINDTODEVICE)
	dns     []netip.Addr
	tmpDir  string
	exited  chan struct{}
	done    chan error
	once    sync.Once
	stopEv  func() // tells openvpn to exit (no-console mode)
	helper  bool   // Linux without root: openvpn runs via pkexec + openvpn-helper

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
	if runtime.GOOS != "windows" {
		for _, p := range []string{"/usr/sbin/openvpn", "/usr/local/sbin/openvpn"} { // sudo's PATH does not always have sbin
			if _, err := os.Stat(p); err == nil {
				return p, nil
			}
		}
		return "", ErrOpenVPNMissing
	}
	for _, c := range []string{`%ProgramFiles%\OpenVPN\bin\openvpn.exe`, `%ProgramFiles(x86)%\OpenVPN\bin\openvpn.exe`} {
		p := platform.ExpandEnv(c)
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	return "", ErrOpenVPNMissing
}

func openVPNMissing() error {
	if runtime.GOOS == "windows" {
		return i18n.Sentinel("openvpn.exe not found; install OpenVPN Community (https://openvpn.net/community-downloads/) or use -openvpn <path>")
	}
	return i18n.Sentinel("openvpn not found; install the openvpn package (e.g. sudo apt install openvpn) or use -openvpn <path>")
}

// installFailed: the automatic installation failed. It counts as
// ErrOpenVPNMissing, so callers do not retry and ask the user again.
type installFailed struct{ err error }

func (e installFailed) Error() string {
	return i18n.T("could not install OpenVPN automatically: {0}", e.err)
}
func (e installFailed) Is(target error) bool { return target == ErrOpenVPNMissing }

// locateOpenVPN finds openvpn, offering to install it when it is missing.
func locateOpenVPN(o Options) (string, error) {
	exe, err := findOpenVPN(o.OpenVPNPath)
	if !errors.Is(err, ErrOpenVPNMissing) || o.InstallOpenVPN == nil {
		return exe, err
	}
	progress := o.Progress
	if progress == nil {
		progress = func(string) {}
	}
	if ierr := o.InstallOpenVPN(func(s string) {
		log.Print(s)
		progress(s)
	}); ierr != nil {
		if errors.Is(ierr, ErrOpenVPNMissing) {
			return "", ierr
		}
		return "", installFailed{ierr}
	}
	progress("")
	exe, err = findOpenVPN(o.OpenVPNPath)
	if err != nil {
		return "", installFailed{err}
	}
	log.Printf("OpenVPN installed: %s", exe)
	return exe, nil
}

// sanitizeOVPN removes from the .ovpn everything that would change the system
// routes or DNS. It also returns locally defined DNS/ifconfig, as a fallback.
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
				b.WriteString("# [removed by vpn_specify] " + l + "\n")
				continue
			}
		}
		b.WriteString(line + "\n")
	}
	return b.Bytes(), dns, ifconfig
}

// OpenVPNNeedsAuth reports whether the .ovpn asks for user/password without a credentials file.
func OpenVPNNeedsAuth(data []byte) bool {
	for _, line := range strings.Split(string(data), "\n") {
		f := strings.Fields(line)
		if len(f) == 1 && strings.EqualFold(strings.TrimPrefix(f[0], "--"), "auth-user-pass") {
			return true
		}
	}
	return false
}

// parsePushReply extracts the tunnel IP and DNS servers from the PUSH_REPLY
// message OpenVPN logs (before the filters are applied).
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
	// Linux, graphical interface without root: openvpn starts via pkexec (the
	// system asks for the administrator password, like UAC on Windows)
	// through "vpn_specify openvpn-helper" (see RunOpenVPNHelper).
	useHelper := runtime.GOOS == "linux" && o.NoConsole && !platform.IsElevated()
	if !platform.IsElevated() && !useHelper {
		if runtime.GOOS != "windows" {
			return nil, ErrNeedsAdmin
		}
		return nil, i18n.Error("OpenVPN mode needs a terminal running as Administrator (OpenVPN creates a network adapter). WireGuard mode does not")
	}
	exe, err := locateOpenVPN(o)
	if err != nil {
		return nil, err
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	sanitized, cfgDNS, cfgIP := sanitizeOVPN(data)
	if o.NoConsole && o.AuthUser == "" && OpenVPNNeedsAuth(data) {
		return nil, ErrNeedsCredentials
	}
	tmpDir, err := os.MkdirTemp("", "vpn_specify-")
	if err != nil {
		return nil, err
	}
	var authArgs []string
	if o.AuthUser != "" {
		// drop auth-user-pass from the .ovpn and pass a temporary file (deleted on disconnect)
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
		"--cd", filepath.Dir(absPath), // relative paths in the .ovpn (ca, cert, auth) keep working
		// The filters come before --config so they take precedence.
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
	var cmd *exec.Cmd
	switch {
	case useHelper:
		pk, err := exec.LookPath("pkexec")
		if err != nil {
			os.RemoveAll(tmpDir)
			return nil, i18n.Error("OpenVPN mode needs root: install polkit (the pkexec command) or open vpn_specify with sudo. WireGuard mode does not")
		}
		self, err := os.Executable()
		if err != nil {
			os.RemoveAll(tmpDir)
			return nil, err
		}
		cmd = exec.Command(pk, append([]string{self, "openvpn-helper", exe}, args...)...)
		// the helper stops openvpn when this input closes (on disconnect or
		// if vpn_specify dies); signals cannot reach a root process
		in, err := cmd.StdinPipe()
		if err != nil {
			os.RemoveAll(tmpDir)
			return nil, err
		}
		stopEvent = func() { in.Close() }
	case o.NoConsole && runtime.GOOS == "windows":
		// Without a console there is no Ctrl+C: openvpn exits cleanly
		// (removing its routes) when we signal this event.
		name, signal, err := platform.NewStopEvent()
		if err != nil {
			os.RemoveAll(tmpDir)
			return nil, err
		}
		args = append(args, "--service", name, "0")
		stopEvent = signal
		cmd = exec.Command(exe, args...)
	default:
		cmd = exec.Command(exe, args...)
	}
	platform.KillWithParent(cmd)
	if o.NoConsole {
		platform.HideConsole(cmd)
		cmd.Stderr = logWriter("[openvpn] ")
	} else {
		cmd.Stdin = os.Stdin // lets the user type user/password if the .ovpn asks
		cmd.Stderr = os.Stderr
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		os.RemoveAll(tmpDir)
		return nil, err
	}
	log.Printf("OpenVPN: starting %s", exe)
	if err := cmd.Start(); err != nil {
		os.RemoveAll(tmpDir)
		return nil, i18n.Wrap(err, "starting openvpn")
	}
	if err := platform.BindChildToUs(cmd.Process); err != nil {
		log.Printf("warning: could not tie openvpn to this process (%v); if vpn_specify is killed, end openvpn.exe manually", err)
	}

	t := &ovpnTunnel{
		cmd: cmd, tmpDir: tmpDir, stopEv: stopEvent, helper: useHelper,
		exited: make(chan struct{}), done: make(chan error, 1),
		cache: map[string]dnsCacheEntry{},
	}
	go func() {
		err := cmd.Wait()
		if err == nil {
			err = i18n.Error("the openvpn process ended")
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
			log.Printf("warning: OpenVPN could not create the tunnel route; connections may fail")
		}
		if strings.Contains(line, "AUTH_FAILED") {
			select {
			case failed <- i18n.Error("authentication refused by the server (AUTH_FAILED)"):
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
		var ee *exec.ExitError
		if useHelper && errors.As(err, &ee) && (ee.ExitCode() == 126 || ee.ExitCode() == 127) {
			return nil, ErrAdminDenied
		}
		return nil, i18n.Error("openvpn failed to connect: {0} (run with -v to see the full log)", err)
	case <-time.After(5 * time.Minute):
		t.kill()
		return nil, i18n.Error("timed out waiting for OpenVPN to connect")
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
		return nil, i18n.Error("could not find the tunnel IP (the server sent no ifconfig); run with -v and check the log")
	}
	if err := waitForLocalAddr(t.localIP, 30*time.Second); err != nil {
		t.kill()
		return nil, err
	}
	if t.ifName, err = prepareTunnelInterface(t.localIP); err != nil {
		t.kill()
		return nil, err
	}
	if len(t.dns) == 0 {
		log.Printf("warning: the VPN sent no DNS servers; names will be resolved by the normal DNS (use -dns to change it)")
	}
	log.Printf("OpenVPN: connected. Tunnel IP %s, DNS %v", t.localIP, t.dns)
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

// scanOutput splits the output into lines. A chunk without a line break that
// ends in ":" (e.g. "Enter Auth Username:") is shown right away, so the user
// sees the prompt.
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
	return i18n.Error("the tunnel IP {0} did not appear on any interface", ip)
}

func (t *ovpnTunnel) dialer(network string) *net.Dialer {
	d := &net.Dialer{Control: bindControl(t.ifName)}
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
			return nil, i18n.Error("{0}: IPv6 is not supported in OpenVPN mode", host)
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

// ListenUDP binds the socket to the tunnel IP; for the same reason as TCP
// (strong host on Windows, SO_BINDTODEVICE on Linux), datagrams only leave
// through the VPN interface.
func (t *ovpnTunnel) ListenUDP() (net.PacketConn, error) {
	lc := net.ListenConfig{Control: bindControl(t.ifName)}
	return lc.ListenPacket(context.Background(), "udp4", net.JoinHostPort(t.localIP.String(), "0"))
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
		if t.stopEv != nil {
			t.stopEv()
		}
		_ = t.cmd.Process.Kill() // does not work in helper mode (root process); stopEv covers it
		t.waitExit(15 * time.Second)
		os.RemoveAll(t.tmpDir)
	})
}

func (t *ovpnTunnel) waitExit(d time.Duration) {
	select {
	case <-t.exited:
	case <-time.After(d):
		log.Printf("warning: openvpn did not exit; end it manually")
	}
}

// Close waits for openvpn to exit by itself (it also gets the console Ctrl+C
// and removes the routes it created); if it does not, it is killed.
func (t *ovpnTunnel) Close() error {
	t.once.Do(func() {
		wait := 5 * time.Second
		if t.stopEv != nil {
			t.stopEv()
			wait = 10 * time.Second
		} else if runtime.GOOS != "windows" {
			// outside Windows SIGTERM makes openvpn exit cleanly (removing the
			// route); also covers the case where it got no Ctrl+C
			_ = t.cmd.Process.Signal(syscall.SIGTERM)
		}
		select {
		case <-t.exited:
		case <-time.After(wait):
			_ = t.cmd.Process.Kill()
			t.waitExit(5 * time.Second)
		}
		os.RemoveAll(t.tmpDir)
	})
	return nil
}
