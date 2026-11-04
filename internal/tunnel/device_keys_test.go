package tunnel

import (
	"encoding/base64"
	"testing"

	"golang.org/x/crypto/curve25519"
)

// publicOf returns the base64 public key of a private key.
func publicOf(t *testing.T, private []byte) string {
	t.Helper()
	pub, err := curve25519.X25519(private, curve25519.Basepoint)
	if err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(pub)
}
