//go:build !windows

package platform

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"

	"vpn_specify/internal/i18n"
)

// Protect/Unprotect store passwords outside Windows (DPAPI is used there).
//
// The password is encrypted (AES-256-GCM) with a random per-user key in
// ~/.local/share/vpn_specify/secret.key (mode 0600), kept apart from
// settings.json in ~/.config. Copying only settings.json does not reveal the
// password; the key is protected like SSH keys are: by file permissions.

const secretPrefix = "v1:"

func secretKeyPath() (string, error) {
	base := os.Getenv("XDG_DATA_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		base = filepath.Join(home, ".local", "share")
	}
	return filepath.Join(base, "vpn_specify", "secret.key"), nil
}

func secretKey(create bool) ([]byte, error) {
	p, err := secretKeyPath()
	if err != nil {
		return nil, err
	}
	k, err := os.ReadFile(p)
	if err == nil && len(k) == 32 {
		return k, nil
	}
	if !create {
		return nil, i18n.Error("the password key ({0}) no longer exists; save the credentials again", p)
	}
	k = make([]byte, 32)
	if _, err := rand.Read(k); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return nil, err
	}
	return k, os.WriteFile(p, k, 0o600)
}

func secretAEAD(create bool) (cipher.AEAD, error) {
	k, err := secretKey(create)
	if err != nil {
		return nil, err
	}
	b, err := aes.NewCipher(k)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(b)
}

func Protect(s string) (string, error) {
	g, err := secretAEAD(true)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, g.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	return secretPrefix + base64.StdEncoding.EncodeToString(g.Seal(nonce, nonce, []byte(s), nil)), nil
}

func Unprotect(enc string) (string, error) {
	if !strings.HasPrefix(enc, secretPrefix) {
		return "", i18n.Error("password saved on another system (e.g. Windows DPAPI); save the credentials again")
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(enc, secretPrefix))
	if err != nil {
		return "", err
	}
	g, err := secretAEAD(false)
	if err != nil {
		return "", err
	}
	if len(raw) < g.NonceSize() {
		return "", i18n.Error("the saved password is corrupted")
	}
	out, err := g.Open(nil, raw[:g.NonceSize()], raw[g.NonceSize():], nil)
	if err != nil {
		return "", i18n.Error("could not decrypt the saved password (did the key change?); save the credentials again")
	}
	return string(out), nil
}
