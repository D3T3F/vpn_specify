package gui

import (
	"context"
	"log"
	"strings"
	"sync"
	"time"

	"vpn_specify/internal/i18n"
	"vpn_specify/internal/store"
	"vpn_specify/internal/update"
	"vpn_specify/internal/version"
)

// Updates: on start, the agent looks for a new version in the GitHub
// releases (internal/update). If there is one, the window shows a notice and
// the user chooses: update now, later, or skip the version. When updating,
// the package is downloaded and verified; the agent disconnects, exits and
// applies the update (Windows: installer; Linux: replaces the executable),
// and the new version opens by itself.

type updateInfo struct {
	// "" (nothing to show) | checking | current | available | downloading | installing | error
	State     string  `json:"state"`
	Current   string  `json:"current"`
	Version   string  `json:"version"`
	Notes     string  `json:"notes"`
	Progress  float64 `json:"progress"` // 0..1 while downloading
	Error     string  `json:"error"`
	Dismissed bool    `json:"dismissed"` // "Not now" or a skipped version
}

type updater struct {
	store   *store.Store
	apply   func(*update.Plan) // the agent exits and calls plan.Run
	openURL func(string) error

	mu   sync.Mutex
	info updateInfo
	rel  *update.Release
}

func newUpdater(st *store.Store, apply func(*update.Plan), openURL func(string) error) *updater {
	return &updater{store: st, apply: apply, openURL: openURL, info: updateInfo{Current: version.Version}}
}

func (u *updater) snapshot() *updateInfo {
	u.mu.Lock()
	defer u.mu.Unlock()
	i := u.info
	return &i
}

func (u *updater) set(f func(*updateInfo)) {
	u.mu.Lock()
	f(&u.info)
	u.mu.Unlock()
}

// autoCheck: the check made on start. Failures only go to the log.
func (u *updater) autoCheck() {
	update.CleanupOld()
	if u.store.Snapshot().NoUpdateCheck || !update.IsRelease(version.Version) {
		return
	}
	time.Sleep(2 * time.Second) // let the window open first
	rel, err := u.check()
	if err != nil {
		log.Printf("could not check for updates: %v", err)
		u.set(func(i *updateInfo) { i.State, i.Error = "", "" })
		return
	}
	if rel != nil {
		skipped := rel.Version == u.store.Snapshot().SkipVersion
		u.set(func(i *updateInfo) { i.Dismissed = skipped })
		if skipped {
			log.Printf("version %s available (skipped on request)", rel.Version)
		} else {
			log.Printf("version %s available (current: %s)", rel.Version, version.Version)
		}
	}
}

var errBusy = i18n.Sentinel("an update is already in progress")

func (u *updater) check() (*update.Release, error) {
	u.mu.Lock()
	if s := u.info.State; s == "checking" || s == "downloading" || s == "installing" {
		u.mu.Unlock()
		return nil, errBusy
	}
	u.info.State, u.info.Error = "checking", ""
	u.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	rel, err := update.Check(ctx)
	u.mu.Lock()
	defer u.mu.Unlock()
	if err != nil {
		u.info.State, u.info.Error = "error", err.Error()
		return nil, err
	}
	u.rel = rel
	if rel == nil {
		u.info.State = "current"
		return nil, nil
	}
	u.info = updateInfo{State: "available", Current: version.Version, Version: rel.Version, Notes: rel.Notes}
	return rel, nil
}

// install downloads, prepares and hands the update to the agent (in the
// background; the window follows the state).
func (u *updater) install() error {
	u.mu.Lock()
	rel := u.rel
	if s := u.info.State; s == "downloading" || s == "installing" || s == "checking" {
		u.mu.Unlock()
		return errBusy
	}
	if rel == nil {
		u.mu.Unlock()
		return i18n.Error("no update available")
	}
	u.info.State, u.info.Error, u.info.Progress, u.info.Dismissed = "downloading", "", 0, false
	u.mu.Unlock()

	go func() {
		log.Printf("downloading version %s (%s)…", rel.Version, rel.Asset.Name)
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
		defer cancel()
		path, err := update.Download(ctx, rel.Asset, func(done, total int64) {
			if total > 0 {
				u.set(func(i *updateInfo) { i.Progress = float64(done) / float64(total) })
			}
		})
		var plan *update.Plan
		if err == nil {
			u.set(func(i *updateInfo) { i.State, i.Progress = "installing", 1 })
			plan, err = update.Prepare(path, rel.Version, true)
		}
		if err != nil {
			log.Printf("update failed: %v", err)
			u.set(func(i *updateInfo) { i.State, i.Error = "error", err.Error() })
			return
		}
		log.Printf("update to %s ready (%s); exiting to finish", rel.Version, plan.Description)
		time.Sleep(800 * time.Millisecond) // the window shows "installing…"
		u.apply(plan)
	}()
	return nil
}

func (u *updater) dismiss(skip bool) error {
	u.mu.Lock()
	rel := u.rel
	u.info.Dismissed = true
	if u.info.State == "error" && rel != nil {
		u.info.State, u.info.Error = "available", ""
	}
	u.mu.Unlock()
	if skip && rel != nil {
		log.Printf("version %s skipped", rel.Version)
		return u.store.SetSkipVersion(rel.Version)
	}
	return nil
}

// api: the window functions related to updates.
func (u *updater) api() map[string]any {
	return map[string]any{
		// vsUpdateCheck: "Check now"; returns the new version or "".
		"vsUpdateCheck": func() (string, error) {
			if !update.IsRelease(version.Version) {
				return "", i18n.Error("this is a development build ({0}); only published versions update", version.Version)
			}
			rel, err := u.check()
			if err != nil || rel == nil {
				return "", err
			}
			return rel.Version, nil
		},
		"vsUpdateInstall":  func() error { return u.install() },
		"vsUpdateDismiss":  func(skip bool) error { return u.dismiss(skip) },
		"vsSetUpdateCheck": func(v bool) error { return u.store.SetUpdateCheck(v) },
		"vsOpenReleasePage": func() error {
			u.mu.Lock()
			url := update.ReleasesPage
			if u.rel != nil && strings.HasPrefix(u.rel.URL, update.ReleasesPage+"/") { // only this repository's pages
				url = u.rel.URL
			}
			u.mu.Unlock()
			return u.openURL(url)
		},
	}
}
