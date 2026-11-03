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

// RunOpenVPNHelper is "vpn_specify openvpn-helper <openvpn> args...": it runs
// as root (via pkexec, started by the graphical interface without root) and
// only:
//   - starts openvpn with the given arguments, passing its output through;
//   - sets the tun interface's rp_filter to loose when it appears (the
//     unprivileged agent cannot);
//   - stops openvpn (SIGTERM, cleanly) when standard input closes, that is,
//     when the agent disconnects or dies. A normal process cannot signal a
//     root process, so the notice comes through the input.
func RunOpenVPNHelper(args []string) error {
	if len(args) < 1 {
		return errors.New("usage: vpn_specify openvpn-helper <openvpn> [args...]")
	}
	if os.Geteuid() != 0 {
		return errors.New("openvpn-helper must run as root")
	}
	if filepath.Base(args[0]) != "openvpn" {
		return fmt.Errorf("openvpn-helper only runs openvpn, not %q", args[0])
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
