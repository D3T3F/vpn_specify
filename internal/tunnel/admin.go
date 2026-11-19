package tunnel

import (
	"fmt"
	"os/exec"
	"strings"
)

// macOS has no pkexec: the root command runs through osascript, which shows
// the system's administrator password dialog ("do shell script ... with
// administrator privileges"). The shell command is built from quoted words
// and then made an AppleScript string literal, so a quote, a backslash, a
// $(...) or a backtick in an argument cannot end the literal or run anything.

// shellQuote quotes s as one word for /bin/sh: inside single quotes nothing is
// expanded, and a single quote itself is written as '\”.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// appleScriptString returns s as an AppleScript string literal. The backslash,
// the double quote and the line breaks/tabs are escaped; any other control
// character is refused.
func appleScriptString(s string) (string, error) {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch {
		case r == '\\':
			b.WriteString(`\\`)
		case r == '"':
			b.WriteString(`\"`)
		case r == '\n':
			b.WriteString(`\n`)
		case r == '\r':
			b.WriteString(`\r`)
		case r == '\t':
			b.WriteString(`\t`)
		case r < 0x20 || r == 0x7f:
			return "", fmt.Errorf("control character %U not allowed in a command", r)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String(), nil
}

// helperShellCommand is the command the administrator runs as root:
// "<self> openvpn-helper <helperArgs...>". The helper opens its own log file
// (see helper_darwin.go); the shell does no redirection.
func helperShellCommand(self string, helperArgs []string) string {
	words := append([]string{self, "openvpn-helper"}, helperArgs...)
	for i, w := range words {
		words[i] = shellQuote(w)
	}
	return strings.Join(words, " ")
}

// adminCommand returns the command that runs shellCmd as root through the
// administrator password dialog. It returns an error only for a control
// character in shellCmd.
func adminCommand(shellCmd string) (*exec.Cmd, error) {
	lit, err := appleScriptString(shellCmd)
	if err != nil {
		return nil, err
	}
	return exec.Command("/usr/bin/osascript", "-e", "do shell script "+lit+" with administrator privileges"), nil
}
