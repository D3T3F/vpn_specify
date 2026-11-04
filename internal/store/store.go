package store

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"

	"vpn_specify/internal/i18n"
	"vpn_specify/internal/ovpninstall"
	"vpn_specify/internal/paths"

	"vpn_specify/internal/platform"
	"vpn_specify/internal/textutil"
	"vpn_specify/internal/tunnel"
)

// App data (see paths.DataDir):
//   configs\        imported .ovpn / .conf files
//   settings.json   apps, credentials and preferences
//   control.json    port/token of the running instance
//   language        chosen interface language (see internal/i18n)
//   vpn_specify.log log

type VPNConfig struct {
	Name     string `json:"name"`
	File     string `json:"file"` // file name inside configs\
	Kind     string `json:"kind"` // openvpn | wireguard
	NeedAuth bool   `json:"needAuth"`
	User     string `json:"user,omitempty"`
	PassEnc  string `json:"passEnc,omitempty"` // protected password (DPAPI on Windows: only this user can read it)
	HasPass  bool   `json:"hasPass"`
}

type AppEntry struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Path     string   `json:"path"`
	Args     string   `json:"args"`
	WorkDir  string   `json:"workDir"`
	Icon     string   `json:"icon"`
	Config   string   `json:"config"`  // VPN name
	Mode     string   `json:"mode"`    // capture | proxy
	Capture  []string `json:"capture"` // captured executables
	Shortcut string   `json:"shortcut"`
	// Autostart: open in the VPN when signing in (see internal/autostart)
	Autostart bool `json:"autostart"`
}

type Settings struct {
	AutoDisconnect bool        `json:"autoDisconnect"` // disconnect when the apps close (with the window closed)
	NoUpdateCheck  bool        `json:"noUpdateCheck"`  // do not check for updates on start
	SkipVersion    string      `json:"skipVersion"`    // version the user chose to skip
	Configs        []VPNConfig `json:"configs"`
	Apps           []AppEntry  `json:"apps"`
}

type Store struct {
	Dir string
	mu  sync.Mutex
	s   Settings
}

// Open opens the default data folder (see paths.DataDir).
func Open() (*Store, error) { return OpenAt(paths.DataDir()) }

func OpenAt(dir string) (*Store, error) {
	st := &Store{Dir: dir, s: Settings{AutoDisconnect: true}}
	if err := os.MkdirAll(filepath.Join(st.Dir, "configs"), 0o700); err != nil {
		return nil, err
	}
	b, err := os.ReadFile(filepath.Join(st.Dir, "settings.json"))
	if err == nil {
		b = bytes.TrimPrefix(b, []byte{0xEF, 0xBB, 0xBF}) // BOM (e.g. edited in Notepad)
		if err := json.Unmarshal(b, &st.s); err != nil {
			return nil, i18n.Wrap(err, "invalid settings.json")
		}
	}
	return st, nil
}

func (st *Store) saveLocked() error {
	b, err := json.MarshalIndent(st.s, "", "  ")
	if err != nil {
		return err
	}
	tmp := filepath.Join(st.Dir, "settings.json.tmp")
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, filepath.Join(st.Dir, "settings.json"))
}

func (st *Store) Snapshot() Settings {
	st.mu.Lock()
	defer st.mu.Unlock()
	s := st.s
	s.Configs = append([]VPNConfig{}, st.s.Configs...) // never nil: the page expects lists
	s.Apps = append([]AppEntry{}, st.s.Apps...)
	for i := range s.Configs {
		s.Configs[i].PassEnc = "" // never send the password to the interface
	}
	return s
}

func (st *Store) SetAutoDisconnect(v bool) error {
	st.mu.Lock()
	defer st.mu.Unlock()
	st.s.AutoDisconnect = v
	return st.saveLocked()
}

// SetUpdateCheck turns the update check on start on or off.
func (st *Store) SetUpdateCheck(v bool) error {
	st.mu.Lock()
	defer st.mu.Unlock()
	st.s.NoUpdateCheck = !v
	return st.saveLocked()
}

// SetSkipVersion stores a version that must not be offered again.
func (st *Store) SetSkipVersion(v string) error {
	st.mu.Lock()
	defer st.mu.Unlock()
	st.s.SkipVersion = v
	return st.saveLocked()
}

var unsafeName = regexp.MustCompile(`[^\p{L}\p{N} ._()-]+`)

// SafeName replaces characters that are invalid in file names with repl.
func SafeName(s, repl string) string {
	return strings.TrimSpace(unsafeName.ReplaceAllString(s, repl))
}

// ImportConfig copies a VPN file into data\configs.
func (st *Store) ImportConfig(src string) (VPNConfig, error) {
	data, err := os.ReadFile(src)
	if err != nil {
		return VPNConfig{}, err
	}
	c, err := inspectConfig(data, strings.EqualFold(filepath.Ext(src), ".ovpn"))
	if err != nil {
		return VPNConfig{}, err
	}
	ext := kindExt[c.Kind]
	base := SafeName(strings.TrimSuffix(filepath.Base(src), filepath.Ext(src)), "_")
	if base == "" {
		base = "vpn"
	}

	st.mu.Lock()
	defer st.mu.Unlock()
	name := base
	for i := 2; st.configLocked(name) != nil; i++ {
		name = fmt.Sprintf("%s (%d)", base, i)
	}
	c.Name, c.File = name, name+ext
	if err := os.WriteFile(filepath.Join(st.Dir, "configs", c.File), data, 0o600); err != nil {
		return VPNConfig{}, err
	}
	st.s.Configs = append(st.s.Configs, c)
	st.sortConfigsLocked()
	return c, st.saveLocked()
}

var kindExt = map[string]string{"openvpn": ".ovpn", "wireguard": ".conf"}

// inspectConfig finds the configuration type and validates its content.
func inspectConfig(data []byte, looksOVPN bool) (VPNConfig, error) {
	lower := strings.ToLower(string(data))
	var c VPNConfig
	switch {
	case strings.Contains(lower, "[interface]"):
		c.Kind = "wireguard"
		if _, err := tunnel.ParseWireGuardConfig(data); err != nil {
			return c, i18n.Wrap(err, "invalid WireGuard configuration")
		}
	case strings.Contains(lower, "remote ") || strings.Contains(lower, "<connection>") || looksOVPN:
		c.Kind = "openvpn"
		c.NeedAuth = tunnel.OpenVPNNeedsAuth(data)
	default:
		return c, i18n.Error("the content does not look like an OpenVPN (.ovpn) or WireGuard (.conf) configuration")
	}
	return c, nil
}

func (st *Store) sortConfigsLocked() {
	sort.Slice(st.s.Configs, func(i, j int) bool {
		return strings.ToLower(st.s.Configs[i].Name) < strings.ToLower(st.s.Configs[j].Name)
	})
}

// ConfigText returns the file content of a saved VPN.
func (st *Store) ConfigText(name string) (string, error) {
	st.mu.Lock()
	c := st.configLocked(name)
	if c == nil {
		st.mu.Unlock()
		return "", i18n.Error("the configuration {0} does not exist", strconv.Quote(name))
	}
	path := filepath.Join(st.Dir, "configs", c.File)
	st.mu.Unlock()
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return strings.ReplaceAll(string(bytes.TrimPrefix(b, []byte{0xEF, 0xBB, 0xBF})), "\r\n", "\n"), nil
}

// UpdateConfig renames and/or changes the content of a saved VPN. Apps using
// the VPN follow the new name; the credentials are kept.
func (st *Store) UpdateConfig(name, newName, text string) (VPNConfig, error) {
	newName = SafeName(newName, "_")
	if newName == "" {
		return VPNConfig{}, i18n.Error("enter a name for the VPN")
	}
	if strings.TrimSpace(text) == "" {
		return VPNConfig{}, i18n.Error("the configuration content is empty")
	}
	st.mu.Lock()
	defer st.mu.Unlock()
	c := st.configLocked(name)
	if c == nil {
		return VPNConfig{}, i18n.Error("the configuration {0} does not exist", strconv.Quote(name))
	}
	if newName != name {
		if other := st.configLocked(newName); other != nil {
			return VPNConfig{}, i18n.Error("a VPN named {0} already exists", strconv.Quote(newName))
		}
	}
	info, err := inspectConfig([]byte(text), false) // when editing, the content must identify itself
	if err != nil {
		return VPNConfig{}, err
	}

	dir := filepath.Join(st.Dir, "configs")
	oldPath := filepath.Join(dir, c.File)
	data := []byte(text)
	if old, err := os.ReadFile(oldPath); err == nil && bytes.Contains(old, []byte("\r\n")) {
		data = []byte(strings.ReplaceAll(text, "\n", "\r\n")) // keep Windows line endings
	}
	newFile := newName + kindExt[info.Kind]
	newPath := filepath.Join(dir, newFile)
	if !strings.EqualFold(newPath, oldPath) {
		if _, err := os.Stat(newPath); err == nil {
			return VPNConfig{}, i18n.Error("the file {0} already exists in the configurations folder", newFile)
		}
	}
	tmp := newPath + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return VPNConfig{}, err
	}
	if err := os.Rename(tmp, newPath); err != nil {
		os.Remove(tmp)
		return VPNConfig{}, err
	}
	if !strings.EqualFold(newPath, oldPath) {
		_ = os.Remove(oldPath)
	}

	c.Name, c.File, c.Kind, c.NeedAuth = newName, newFile, info.Kind, info.NeedAuth
	updated := *c
	for i := range st.s.Apps {
		if st.s.Apps[i].Config == name {
			st.s.Apps[i].Config = newName
		}
	}
	st.sortConfigsLocked()
	updated.PassEnc = ""
	return updated, st.saveLocked()
}

func (st *Store) configLocked(name string) *VPNConfig {
	for i := range st.s.Configs {
		if st.s.Configs[i].Name == name {
			return &st.s.Configs[i]
		}
	}
	return nil
}

// ConfigFor returns the file path and the connection options.
func (st *Store) ConfigFor(name string) (string, tunnel.Options, error) {
	st.mu.Lock()
	defer st.mu.Unlock()
	c := st.configLocked(name)
	if c == nil {
		return "", tunnel.Options{}, i18n.Error("the configuration {0} does not exist", strconv.Quote(name))
	}
	o := tunnel.Options{NoConsole: true, AuthUser: c.User, InstallOpenVPN: installOpenVPNGUI}
	if c.PassEnc != "" {
		p, err := platform.Unprotect(c.PassEnc)
		if err != nil {
			return "", o, i18n.Wrap(err, "could not read the saved password")
		}
		o.AuthPass = p
	}
	return filepath.Join(st.Dir, "configs", c.File), o, nil
}

func (st *Store) SetCredentials(name, user, pass string) error {
	enc := ""
	if pass != "" {
		var err error
		if enc, err = platform.Protect(pass); err != nil {
			return err
		}
	}
	st.mu.Lock()
	defer st.mu.Unlock()
	c := st.configLocked(name)
	if c == nil {
		return i18n.Error("the configuration {0} does not exist", strconv.Quote(name))
	}
	c.User, c.PassEnc, c.HasPass = user, enc, enc != ""
	return st.saveLocked()
}

func (st *Store) RemoveConfig(name string) error {
	st.mu.Lock()
	defer st.mu.Unlock()
	for _, a := range st.s.Apps {
		if a.Config == name {
			return i18n.Error("the app {0} uses this VPN; remove or change the app first", strconv.Quote(a.Name))
		}
	}
	for i, c := range st.s.Configs {
		if c.Name == name {
			_ = os.Remove(filepath.Join(st.Dir, "configs", c.File))
			st.s.Configs = append(st.s.Configs[:i], st.s.Configs[i+1:]...)
			return st.saveLocked()
		}
	}
	return nil
}

// installOpenVPNGUI asks in a dialog on Windows, where the agent is already
// elevated and nothing else would ask. On Linux pkexec's password dialog is
// the question: dismissing it cancels the installation.
func installOpenVPNGUI(progress func(string)) error {
	if !platform.Confirm("vpn_specify", ovpninstall.Question()) {
		return tunnel.ErrOpenVPNMissing
	}
	return ovpninstall.Install(context.Background(), progress)
}

// ErrAppNotFound: no app with that id.
var ErrAppNotFound = i18n.Sentinel("app not found")

func newID() string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func (st *Store) SaveApp(a AppEntry) (AppEntry, error) {
	a.Name = strings.TrimSpace(a.Name)
	if a.Name == "" || a.Path == "" {
		return a, i18n.Error("name and program are required")
	}
	if a.Mode != "proxy" {
		a.Mode = "capture"
	}
	a.Capture = normalizeList(a.Capture)
	if a.Mode == "capture" && len(a.Capture) == 0 {
		return a, i18n.Error("enter at least one process to capture (e.g. discord.exe)")
	}
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.configLocked(a.Config) == nil {
		return a, i18n.Error("choose a VPN for the app")
	}
	if a.Autostart {
		// only one VPN is active at a time
		for _, o := range st.s.Apps {
			if o.ID != a.ID && o.Autostart && o.Config != a.Config {
				return a, i18n.Error("only one VPN is active at a time: the apps that start with the system must use the same VPN ({0} starts with the VPN {1})", o.Name, strconv.Quote(o.Config))
			}
		}
	}
	if a.ID == "" {
		a.ID = newID()
		st.s.Apps = append(st.s.Apps, a)
	} else {
		found := false
		for i := range st.s.Apps {
			if st.s.Apps[i].ID == a.ID {
				a.Shortcut = st.s.Apps[i].Shortcut
				st.s.Apps[i] = a
				found = true
			}
		}
		if !found {
			return a, ErrAppNotFound
		}
	}
	return a, st.saveLocked()
}

func (st *Store) App(id string) (AppEntry, bool) {
	st.mu.Lock()
	defer st.mu.Unlock()
	for _, a := range st.s.Apps {
		if a.ID == id {
			return a, true
		}
	}
	return AppEntry{}, false
}

func (st *Store) SetShortcut(id, path string) error {
	st.mu.Lock()
	defer st.mu.Unlock()
	for i := range st.s.Apps {
		if st.s.Apps[i].ID == id {
			st.s.Apps[i].Shortcut = path
			return st.saveLocked()
		}
	}
	return ErrAppNotFound
}

func (st *Store) RemoveApp(id string) (AppEntry, error) {
	st.mu.Lock()
	defer st.mu.Unlock()
	for i, a := range st.s.Apps {
		if a.ID == id {
			st.s.Apps = append(st.s.Apps[:i], st.s.Apps[i+1:]...)
			return a, st.saveLocked()
		}
	}
	return AppEntry{}, ErrAppNotFound
}

// AutostartApps returns the apps marked to start with the system.
func (st *Store) AutostartApps() []AppEntry {
	st.mu.Lock()
	defer st.mu.Unlock()
	var out []AppEntry
	for _, a := range st.s.Apps {
		if a.Autostart {
			out = append(out, a)
		}
	}
	return out
}

func normalizeList(in []string) []string {
	var out []string
	seen := map[string]bool{}
	for _, s := range in {
		for _, p := range textutil.SplitList(s) {
			p = strings.ToLower(p)
			if !strings.HasSuffix(p, ".exe") {
				p += ".exe"
			}
			if !seen[p] {
				seen[p] = true
				out = append(out, p)
			}
		}
	}
	return out
}
