//go:build darwin

package tunnel

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

// RunOpenVPNHelper is "vpn_specify openvpn-helper <stop file> <owner pid>
// <log file> <openvpn> args..." on macOS. It runs as root, started by the
// administrator prompt (see admin.go), and only:
//   - starts openvpn with the given arguments, its output going to the log
//     file that the unprivileged vpn_specify reads;
//   - stops openvpn (SIGTERM, cleanly) when the stop file appears, or when the
//     vpn_specify process that started it is gone. A normal process cannot
//     signal a root process, so these notices are files and polling instead
//     of standard input as on Linux.
func RunOpenVPNHelper(args []string) error {
	if os.Geteuid() != 0 {
		return errors.New("openvpn-helper must run as root")
	}
	h, err := parseDarwinHelperArgs(args)
	if err != nil {
		return err
	}
	if err := checkOpenVPNBinary(h.openvpn, osFileOwnerStat{}); err != nil {
		return err
	}
	if h.configFile != "" {
		// a symbolic link here could make root read another file
		if fi, err := os.Lstat(h.configFile); err != nil || !fi.Mode().IsRegular() {
			return fmt.Errorf("openvpn-helper: %s is not a regular file", h.configFile)
		}
	}
	logF, err := openHelperLog(h.logFile)
	if err != nil {
		return err
	}
	defer logF.Close()

	cmd := exec.Command(h.openvpn, h.args...)
	cmd.Stdout = logF
	cmd.Stderr = logF
	if err := cmd.Start(); err != nil {
		return err
	}

	var once sync.Once
	stop := func() {
		once.Do(func() {
			_ = cmd.Process.Signal(syscall.SIGTERM)
			time.AfterFunc(10*time.Second, func() { _ = cmd.Process.Kill() })
		})
	}
	go func() {
		t := time.NewTicker(500 * time.Millisecond)
		defer t.Stop()
		for range t.C {
			if _, err := os.Stat(h.stopFile); err == nil || !processAlive(h.owner) {
				stop()
				return
			}
		}
	}()
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGTERM, syscall.SIGINT, syscall.SIGHUP)
	go func() {
		<-sig
		stop()
	}()
	return cmd.Wait()
}

// openHelperLog opens the log for appending. The folder belongs to the user
// (and so can be changed by the user), so nothing is followed: the folder must
// be a real folder, the file is opened with O_NOFOLLOW and must be a regular
// file owned by the folder's owner, who must not be root.
func openHelperLog(path string) (*os.File, error) {
	dir := filepath.Dir(path)
	dfi, err := os.Lstat(dir)
	if err != nil || !dfi.IsDir() {
		return nil, fmt.Errorf("openvpn-helper: %s is not a folder", dir)
	}
	dirOwner, ok := ownerOfInfo(dfi)
	if !ok || dirOwner == 0 {
		return nil, fmt.Errorf("openvpn-helper: %s must belong to a user, not root", dir)
	}
	fd, err := unix.Open(path, unix.O_WRONLY|unix.O_APPEND|unix.O_NOFOLLOW, 0)
	if err != nil {
		return nil, fmt.Errorf("openvpn-helper: cannot open the log %s: %w", path, err)
	}
	var st unix.Stat_t
	if err := unix.Fstat(fd, &st); err != nil || st.Mode&unix.S_IFMT != unix.S_IFREG || st.Uid != dirOwner {
		unix.Close(fd)
		return nil, fmt.Errorf("openvpn-helper: the log %s must be a regular file owned by the user", path)
	}
	return os.NewFile(uintptr(fd), path), nil
}

// ownerOfInfo returns the owner uid of a file, when the system reports it.
func ownerOfInfo(fi fs.FileInfo) (uint32, bool) {
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, false
	}
	return st.Uid, true
}

// osFileOwnerStat is the fileOwnerStat of the real files.
type osFileOwnerStat struct{}

func (osFileOwnerStat) OwnerAndMode(path string) (uint32, fs.FileMode, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return 0, 0, err
	}
	uid, ok := ownerOfInfo(fi)
	if !ok {
		return 0, 0, errors.New("owner not known")
	}
	return uid, fi.Mode().Perm(), nil
}

// processAlive reports whether the process exists (EPERM means it does, but
// belongs to someone else).
func processAlive(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}
