//go:build !windows

package platform

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// Protect/Unprotect guardam senhas fora do Windows (lá é o DPAPI).
//
// A senha é cifrada (AES-256-GCM) com uma chave aleatória deste usuário em
// ~/.local/share/vpn_specify/secret.key (permissão 0600), separada do
// settings.json em ~/.config. Copiar só o settings.json não revela a senha;
// a proteção da chave é a mesma das chaves SSH: as permissões do arquivo.

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
		return nil, errors.New("a chave das senhas (" + p + ") não existe mais; salve as credenciais de novo")
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
		return "", errors.New("senha salva em outro sistema (ex.: DPAPI do Windows); salve as credenciais de novo")
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
		return "", errors.New("senha salva corrompida")
	}
	out, err := g.Open(nil, raw[:g.NonceSize()], raw[g.NonceSize():], nil)
	if err != nil {
		return "", errors.New("não consegui decifrar a senha salva (a chave mudou?); salve as credenciais de novo")
	}
	return string(out), nil
}
