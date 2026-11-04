//go:build windows

package ovpninstall

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"

	"vpn_specify/internal/i18n"
	"vpn_specify/internal/platform"
)

func source() string { return "openvpn.net" }

// The MSI is pinned: a new OpenVPN version means updating both values here
// (Get-FileHash, after checking the Authenticode signature is OpenVPN Inc.'s).
var (
	msiURL    = "https://swupdate.openvpn.org/community/releases/OpenVPN-2.7.8-I002-amd64.msi"
	msiSHA256 = "33104dd02c48cba172612882b1af6276422b1b4340d4c9c84dd2afb4b356fe9f"
	runMSI    = runMsiexec
)

// msiFeatures installs the program and its drivers only: no OpenVPN GUI (it
// would start at every logon), no service, samples or documentation.
const msiFeatures = "OpenVPN,Drivers,Drivers.TAPWindows6,Drivers.OvpnDco"

const maxMSI = 100 << 20

// Install downloads, verifies and silently installs OpenVPN. It needs
// Administrator (the interface agent already runs that way).
func Install(ctx context.Context, progress func(string)) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	// Not the user's temp folder: there, an unelevated program could replace
	// the verified file before the elevated msiexec reads it.
	dir, err := os.MkdirTemp(filepath.Join(os.Getenv("SystemRoot"), "Temp"), "vpn_specify-openvpn-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)

	progress(i18n.T("Downloading OpenVPN…"))
	msi := filepath.Join(dir, "OpenVPN.msi")
	if err := download(ctx, msi); err != nil {
		return err
	}
	progress(i18n.T("Installing OpenVPN…"))
	return runMSI(ctx, msi)
}

func download(ctx context.Context, dst string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, msiURL, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return i18n.Error("the OpenVPN download failed: {0}", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return i18n.Error("the OpenVPN download failed: {0}", resp.Status)
	}
	f, err := os.Create(dst)
	if err != nil {
		return err
	}
	h := sha256.New()
	_, err = io.Copy(io.MultiWriter(f, h), io.LimitReader(resp.Body, maxMSI))
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return i18n.Error("the OpenVPN download failed: {0}", err)
	}
	if hex.EncodeToString(h.Sum(nil)) != msiSHA256 {
		return i18n.Error("the downloaded OpenVPN installer is corrupted or was altered")
	}
	return nil
}

func runMsiexec(ctx context.Context, msi string) error {
	cmd := exec.CommandContext(ctx, filepath.Join(os.Getenv("SystemRoot"), "System32", "msiexec.exe"),
		"/i", msi, "/qn", "/norestart", "ADDLOCAL="+msiFeatures)
	platform.HideConsole(cmd)
	err := cmd.Run()
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		if ee.ExitCode() == 3010 { // ERROR_SUCCESS_REBOOT_REQUIRED: installed
			return nil
		}
		return i18n.Error("the OpenVPN installer failed (code {0})", strconv.Itoa(ee.ExitCode()))
	}
	return err
}
