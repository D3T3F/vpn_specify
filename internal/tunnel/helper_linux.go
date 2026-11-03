//go:build linux

package tunnel

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"syscall"
	"time"

	"vpn_specify/internal/platform"
)

// RunOpenVPNHelper é o "vpn_specify openvpn-helper <openvpn> args...": roda
// como root (via pkexec, aberto pela interface gráfica sem root) e só:
//   - inicia o openvpn com os argumentos recebidos, repassando a saída;
//   - deixa o rp_filter da interface tun em modo loose quando ela aparece
//     (o agente, sem root, não consegue);
//   - encerra o openvpn (SIGTERM, limpo) quando a entrada padrão fecha, ou
//     seja, quando o agente desconecta ou morre. Um processo comum não pode
//     mandar sinais para um processo root, por isso o aviso vem pela entrada.
func RunOpenVPNHelper(args []string) error {
	if len(args) < 1 {
		return errors.New("uso: vpn_specify openvpn-helper <openvpn> [args...]")
	}
	if os.Geteuid() != 0 {
		return errors.New("openvpn-helper precisa rodar como root")
	}
	if filepath.Base(args[0]) != "openvpn" {
		return fmt.Errorf("openvpn-helper só executa o openvpn, não %q", args[0])
	}
	cmd := exec.Command(args[0], args[1:]...)
	platform.KillWithParent(cmd)
	cmd.Stderr = os.Stderr
	out, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}

	stop := func() {
		_ = cmd.Process.Signal(syscall.SIGTERM)
		time.AfterFunc(10*time.Second, func() { _ = cmd.Process.Kill() })
	}
	go func() {
		_, _ = io.Copy(io.Discard, os.Stdin)
		stop()
	}()
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGTERM, syscall.SIGINT, syscall.SIGHUP)
	go func() {
		<-sig
		stop()
	}()

	sc := bufio.NewScanner(out)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Text()
		fmt.Println(line)
		if m := tunOpenedRe.FindStringSubmatch(line); m != nil {
			setLooseRPFilter(m[1])
		}
	}
	return cmd.Wait()
}

var tunOpenedRe = regexp.MustCompile(`(?:TUN/TAP|DCO) device (\S+) opened`)
