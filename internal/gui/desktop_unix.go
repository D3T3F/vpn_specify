//go:build linux || darwin

package gui

import (
	"os"
	"os/exec"
	"strings"
	"syscall"

	"vpn_specify/internal/platform"
)

// Helpers shared by the Unix builds (Linux and macOS): the arguments of the
// apps, the installed apps and the programs started outside the session.

// ---------- arguments ----------

// splitArgs splits a command line like the shell would (single and double
// quotes, backslash), without expanding variables.
func splitArgs(s string) []string {
	var out []string
	var cur strings.Builder
	inArg, quote := false, rune(0)
	rs := []rune(s)
	for i := 0; i < len(rs); i++ {
		c := rs[i]
		switch {
		case quote == '\'':
			if c == '\'' {
				quote = 0
			} else {
				cur.WriteRune(c)
			}
		case quote == '"':
			if c == '"' {
				quote = 0
			} else if c == '\\' && i+1 < len(rs) && strings.ContainsRune("\"\\$`", rs[i+1]) {
				i++
				cur.WriteRune(rs[i])
			} else {
				cur.WriteRune(c)
			}
		case c == '\'' || c == '"':
			quote, inArg = c, true
		case c == '\\' && i+1 < len(rs):
			i++
			cur.WriteRune(rs[i])
			inArg = true
		case c == ' ' || c == '\t' || c == '\n':
			if inArg {
				out = append(out, cur.String())
				cur.Reset()
				inArg = false
			}
		default:
			cur.WriteRune(c)
			inArg = true
		}
	}
	if inArg {
		out = append(out, cur.String())
	}
	return out
}

// joinArgs is the inverse of splitArgs (to show and save the arguments).
func joinArgs(args []string) string {
	var parts []string
	for _, a := range args {
		if a != "" && !strings.ContainsAny(a, " \t\n'\"\\$`;&|<>()*?[]{}~#") {
			parts = append(parts, a)
			continue
		}
		parts = append(parts, "'"+strings.ReplaceAll(a, "'", `'\''`)+"'")
	}
	return strings.Join(parts, " ")
}

type installedApp struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Comment string `json:"comment"`
	Path    string `json:"path"`
	Args    string `json:"args"`
	WorkDir string `json:"workDir"`
	Icon    string `json:"icon"`     // icon name or path (goes into the shortcut)
	IconURL string `json:"iconFile"` // icon file, for the window (empty if not found)
}

// ---------- processes ----------

// startDetached starts a program outside the vpn_specify session (it stays
// open if vpn_specify exits). When running through sudo, it opens as the user.
func startDetached(path string, args []string, dir string, env []string) (int, error) {
	cmd := exec.Command(path, args...)
	cmd.Dir, cmd.Env = dir, env
	if cmd.Dir == "" {
		cmd.Dir, _ = os.UserHomeDir()
	}
	if fi, err := os.Stat(cmd.Dir); err != nil || !fi.IsDir() {
		cmd.Dir = ""
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true} // its own process group (pgid = pid)
	if su := platform.InvokingUser(); su != nil {
		su.Apply(cmd)
	}
	if err := cmd.Start(); err != nil {
		return 0, err
	}
	go cmd.Wait() // avoid a zombie process
	return cmd.Process.Pid, nil
}
