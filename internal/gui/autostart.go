package gui

import (
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
	"time"

	"vpn_specify/internal/autostart"
	"vpn_specify/internal/i18n"
	"vpn_specify/internal/store"
	"vpn_specify/internal/tunnel"
)

// Apps that start with the system: each app has an "Open with the system"
// option; the system has a single entry (internal/autostart) that runs
// "vpn_specify autostart" at sign-in, and the agent opens every marked app
// (they all use the same VPN).

// syncAutostart creates the system entry when apps are marked (always
// pointing at this executable, which may have moved) and removes it when
// none is.
func syncAutostart(st *store.Store) error {
	if len(st.AutostartApps()) == 0 {
		return autostart.Disable()
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	return autostart.Enable(exe)
}

// syncAutostartAfterSave: after saving/removing an app; the error goes to the
// window, but the app is already saved.
func syncAutostartAfterSave(st *store.Store) error {
	if err := syncAutostart(st); err != nil {
		log.Printf("start with the system: %v", err)
		return i18n.Wrap(err, "the app was saved, but starting it with the system could not be set up")
	}
	return nil
}

// runAutostart opens the marked apps, in order. Right after logon the network
// may not be ready yet: each app is tried up to 4 times, with pauses. report
// receives the errors left over.
func runAutostart(st *store.Store, launch func(store.AppEntry) error, report func(string)) {
	apps := st.AutostartApps()
	if len(apps) == 0 {
		log.Printf("no app marked to start with the system")
		return
	}
	log.Printf("starting with the system: %d app(s)", len(apps))
	var failed []string
	for _, app := range apps {
		var err error
		for attempt := 1; attempt <= 4; attempt++ {
			if err = launch(app); err == nil {
				break
			}
			log.Printf("starting %s with the system (attempt %d): %v", app.Name, attempt, err)
			if !retryable(err) {
				break
			}
			if attempt < 4 {
				time.Sleep(time.Duration(attempt) * 10 * time.Second)
			}
		}
		if err != nil {
			failed = append(failed, fmt.Sprintf("%s: %v", app.Name, err))
		}
	}
	if len(failed) > 0 {
		report(i18n.T("Could not open in the VPN when the system started:") + "\n\n" + strings.Join(failed, "\n"))
	}
}

// retryable: failures that retrying cannot fix (administrator password
// refused, missing credentials, missing program or app) are not repeated, so
// the password is not asked several times in a row.
func retryable(err error) bool {
	for _, e := range []error{tunnel.ErrAdminDenied, tunnel.ErrNeedsCredentials, tunnel.ErrNeedsAdmin, tunnel.ErrOpenVPNMissing,
		store.ErrAppNotFound, os.ErrNotExist, exec.ErrNotFound} {
		if errors.Is(err, e) {
			return false
		}
	}
	return true
}
