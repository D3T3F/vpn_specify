//go:build !windows

package ovpninstall

import (
	"context"
	"os"
	"testing"
)

// TestLinuxFlow only runs with fake pkexec/apt-get in PATH (FAKE_LOG set); it
// must never run the real package manager.
func TestLinuxFlow(t *testing.T) {
	if os.Getenv("FAKE_LOG") == "" {
		t.Skip("needs the fake pkexec/apt-get")
	}
	err := Install(context.Background(), func(s string) { t.Log("progress:", s) })
	t.Logf("RESULT: %v", err)
}
