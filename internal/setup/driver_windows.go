//go:build windows

package setup

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"

	"vpn_specify/internal/i18n"
)

const (
	windivertService    = "WinDivert"
	windivertDriverFile = "windivert64.sys"
	windivertStopWait   = 5 * time.Second
	// rights needed to read the configuration, query the state and stop the
	// service; nothing that changes its configuration
	serviceStopRights = windows.SERVICE_STOP | windows.SERVICE_QUERY_STATUS | windows.SERVICE_QUERY_CONFIG
)

// renameFile is os.Rename; tests replace it to force the fallback path.
var renameFile = os.Rename

// isWinDivertDriver reports whether dst is the WinDivert kernel driver image,
// which Windows keeps locked while its service is running.
func isWinDivertDriver(dst string) bool {
	return strings.EqualFold(filepath.Base(dst), windivertDriverFile)
}

// stopWinDivertService asks the WinDivert service to stop so Windows releases
// the driver image at dst. It only stops the service when its configured
// image is dst (another vpn_specify copy or WinDivert tool is left alone). It
// never changes the service configuration. It reports whether the image at
// dst is no longer loaded by the service.
// This talks to the Service Control Manager, so it cannot be unit tested; it
// is checked by hand on Windows.
func stopWinDivertService(dst string) bool {
	scm, err := windows.OpenSCManager(nil, nil, windows.SC_MANAGER_CONNECT)
	if err != nil {
		return false
	}
	defer windows.CloseServiceHandle(scm)
	name, err := windows.UTF16PtrFromString(windivertService)
	if err != nil {
		return false
	}
	h, err := windows.OpenService(scm, name, serviceStopRights)
	if errors.Is(err, windows.ERROR_SERVICE_DOES_NOT_EXIST) {
		return true // no driver service, so nothing is loaded
	}
	if err != nil {
		return false
	}
	s := &mgr.Service{Name: windivertService, Handle: h}
	defer s.Close()
	cfg, err := s.Config()
	if err != nil {
		return false
	}
	if !serviceBinaryMatches(cfg.BinaryPathName, dst) {
		return true // the service loads another file; ours is not in use
	}
	// The error is ignored on purpose (ERROR_SERVICE_NOT_ACTIVE, or a stop
	// already pending): the state polled below says whether it worked.
	_, _ = s.Control(svc.Stop)
	deadline := time.Now().Add(windivertStopWait)
	for {
		st, err := s.Query()
		if err != nil {
			return false
		}
		if st.State == svc.Stopped {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(200 * time.Millisecond)
	}
}

// isFileInUse reports whether err is the "file is locked" kind of failure
// (access denied or sharing violation) that a loaded driver image produces.
func isFileInUse(err error) bool {
	return errors.Is(err, windows.ERROR_ACCESS_DENIED) || errors.Is(err, windows.ERROR_SHARING_VIOLATION)
}

// replaceAside moves the existing dst out of the way to dst+".old", puts tmp
// in its place and removes the old file (or schedules its removal for the
// next restart if it is still in use). If tmp cannot be put in place, the old
// file is moved back; if that fails too, the old file stays at dst+".old" and
// the error names it.
func replaceAside(tmp, dst string) error {
	old := dst + ".old"
	_ = os.Remove(old)
	if err := renameFile(dst, old); err != nil {
		return i18n.Wrap(err, "could not replace {0} (is it in use?)", dst)
	}
	if err := renameFile(tmp, dst); err != nil {
		if rerr := renameFile(old, dst); rerr != nil {
			return i18n.Error("could not replace {0}; the old file is kept at {1} (replace: {2}; restore: {3})", dst, old, err, rerr)
		}
		return i18n.Wrap(err, "could not replace {0} (is it in use?)", dst)
	}
	if err := os.Remove(old); err != nil && !errors.Is(err, os.ErrNotExist) {
		removeOrDelay(old)
	}
	return nil
}
