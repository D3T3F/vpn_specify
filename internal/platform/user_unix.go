//go:build unix

package platform

import (
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

// SudoUser is whoever called sudo, when vpn_specify runs as root through
// sudo (OpenVPN mode needs root). Apps opened by "up -- program" run as that
// user, never as root.
type SudoUser struct {
	UID, GID uint32
	Name     string
	Home     string
}

// InvokingUser returns the user who called sudo, or nil when not running as
// root through sudo.
func InvokingUser() *SudoUser {
	if os.Geteuid() != 0 {
		return nil
	}
	uid, err1 := strconv.ParseUint(os.Getenv("SUDO_UID"), 10, 32)
	gid, err2 := strconv.ParseUint(os.Getenv("SUDO_GID"), 10, 32)
	if err1 != nil || err2 != nil || uid == 0 {
		return nil
	}
	u, err := user.LookupId(strconv.FormatUint(uid, 10))
	if err != nil {
		return nil
	}
	return &SudoUser{UID: uint32(uid), GID: uint32(gid), Name: u.Username, Home: u.HomeDir}
}

// MkdirAllOwned creates dir (like os.MkdirAll) and gives the created folders
// to u, so the user can use them later.
func (u *SudoUser) MkdirAllOwned(dir string, perm os.FileMode) error {
	var created []string
	for d := dir; ; d = filepath.Dir(d) {
		if _, err := os.Stat(d); err == nil || d == filepath.Dir(d) {
			break
		}
		created = append(created, d)
	}
	if err := os.MkdirAll(dir, perm); err != nil {
		return err
	}
	for _, d := range created {
		_ = os.Chown(d, int(u.UID), int(u.GID))
	}
	return nil
}

// Chown gives the file to u.
func (u *SudoUser) Chown(path string) error { return os.Chown(path, int(u.UID), int(u.GID)) }

// Apply makes cmd run as u, with the user's HOME/USER (not root's).
func (u *SudoUser) Apply(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Credential = &syscall.Credential{Uid: u.UID, Gid: u.GID}
	set := map[string]string{"HOME": u.Home, "USER": u.Name, "LOGNAME": u.Name}
	if rt := "/run/user/" + strconv.FormatUint(uint64(u.UID), 10); dirExists(rt) {
		set["XDG_RUNTIME_DIR"] = rt // Wayland, PulseAudio/PipeWire, session D-Bus
	}
	env := cmd.Env
	if env == nil {
		env = os.Environ()
	}
	out := env[:0:0]
	for _, kv := range env {
		k, _, _ := strings.Cut(kv, "=")
		if _, ok := set[k]; !ok {
			out = append(out, kv)
		}
	}
	for k, v := range set {
		out = append(out, k+"="+v)
	}
	cmd.Env = out
}

func dirExists(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.IsDir()
}
