package tunnel

import (
	"strings"
	"testing"
)

// appleScriptUnquote reverses appleScriptString the way AppleScript reads a
// string literal (the escapes it uses: \\ \" \n \r \t).
func appleScriptUnquote(t *testing.T, lit string) string {
	t.Helper()
	if len(lit) < 2 || lit[0] != '"' || lit[len(lit)-1] != '"' {
		t.Fatalf("not a string literal: %s", lit)
	}
	var b strings.Builder
	body := lit[1 : len(lit)-1]
	for i := 0; i < len(body); i++ {
		c := body[i]
		if c == '"' {
			t.Fatalf("unescaped quote inside the literal: %s", lit)
		}
		if c != '\\' {
			b.WriteByte(c)
			continue
		}
		i++
		switch body[i] {
		case '\\':
			b.WriteByte('\\')
		case '"':
			b.WriteByte('"')
		case 'n':
			b.WriteByte('\n')
		case 'r':
			b.WriteByte('\r')
		case 't':
			b.WriteByte('\t')
		default:
			t.Fatalf("unknown escape \\%c", body[i])
		}
	}
	return b.String()
}

// shellSplit reads back the words /bin/sh would see from a command built with
// shellQuote (single-quoted words only, with '\” for a quote).
func shellSplit(t *testing.T, s string) []string {
	t.Helper()
	var words []string
	var cur strings.Builder
	inWord, inQuote := false, false
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case inQuote:
			if c == '\'' {
				inQuote = false
			} else {
				cur.WriteByte(c)
			}
		case c == '\'':
			inQuote, inWord = true, true
		case c == '\\' && i+1 < len(s) && s[i+1] == '\'': // an escaped quote: '\''
			cur.WriteByte('\'')
			i++
		case c == ' ':
			if inWord {
				words = append(words, cur.String())
				cur.Reset()
				inWord = false
			}
		default:
			t.Fatalf("unexpected unquoted character %q in %s", c, s)
		}
	}
	if inQuote {
		t.Fatalf("unterminated quote in %s", s)
	}
	if inWord {
		words = append(words, cur.String())
	}
	return words
}

var hostile = []string{
	`plain`,
	`with "double" quotes`,
	`with 'single' quotes`,
	`back\slash\\`,
	"new\nline and\r\ttab",
	`$(touch /tmp/pwned)`,
	"`touch /tmp/pwned`",
	`"; do shell script "rm -rf /"; "`,
	`\"} & (do shell script "id") & {\"`,
	`a'$(id)'"b`,
}

func TestShellQuoteRoundTrip(t *testing.T) {
	for _, s := range hostile {
		if got := shellSplit(t, shellQuote(s)); len(got) != 1 || got[0] != s {
			t.Errorf("shellQuote(%q) reads back as %q", s, got)
		}
	}
}

func TestAppleScriptStringRoundTrip(t *testing.T) {
	for _, s := range hostile {
		lit, err := appleScriptString(s)
		if err != nil {
			t.Fatalf("appleScriptString(%q): %v", s, err)
		}
		if got := appleScriptUnquote(t, lit); got != s {
			t.Errorf("appleScriptString(%q) reads back as %q", s, got)
		}
	}
}

func TestAppleScriptStringRefusesControlCharacters(t *testing.T) {
	for _, s := range []string{"a\x00b", "a\x07b", "a\x1bb", "a\x7fb"} {
		if _, err := appleScriptString(s); err == nil {
			t.Errorf("appleScriptString(%q) should fail", s)
		}
	}
}

// The whole chain: the root command, as the shell sees it after osascript
// reads the literal, has the same words as the arguments.
func TestHelperShellCommandKeepsArguments(t *testing.T) {
	args := []string{"/tmp/stop file", "123", "/tmp/log's file", "/opt/homebrew/sbin/openvpn", "--config", `/Users/x/it's "a" $(id).ovpn`}
	cmd := helperShellCommand("/Applications/vpn_specify", args)
	lit, err := appleScriptString(cmd)
	if err != nil {
		t.Fatal(err)
	}
	got := shellSplit(t, appleScriptUnquote(t, lit))
	want := append([]string{"/Applications/vpn_specify", "openvpn-helper"}, args...)
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("words = %q; want %q", got, want)
	}
}
