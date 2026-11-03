package store

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"vpn_specify/internal/paths"

	"vpn_specify/internal/platform"
	"vpn_specify/internal/textutil"
	"vpn_specify/internal/tunnel"
)

// Dados do app (veja paths.DataDir):
//   configs\        arquivos .ovpn / .conf importados
//   settings.json   apps, credenciais e preferências
//   control.json    porta/token da instância em execução
//   vpn_specify.log log

type VPNConfig struct {
	Name     string `json:"name"`
	File     string `json:"file"` // nome do arquivo em configs\
	Kind     string `json:"kind"` // openvpn | wireguard
	NeedAuth bool   `json:"needAuth"`
	User     string `json:"user,omitempty"`
	PassEnc  string `json:"passEnc,omitempty"` // senha protegida com DPAPI (só este usuário do Windows lê)
	HasPass  bool   `json:"hasPass"`
}

type AppEntry struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Path     string   `json:"path"`
	Args     string   `json:"args"`
	WorkDir  string   `json:"workDir"`
	Icon     string   `json:"icon"`
	Config   string   `json:"config"`  // nome da VPN
	Mode     string   `json:"mode"`    // capture | proxy
	Capture  []string `json:"capture"` // executáveis capturados
	Shortcut string   `json:"shortcut"`
	// Autostart: abrir na VPN ao entrar na conta (veja internal/autostart)
	Autostart bool `json:"autostart"`
}

type Settings struct {
	AutoDisconnect bool        `json:"autoDisconnect"` // desconecta quando os apps fecham (sem janela aberta)
	NoUpdateCheck  bool        `json:"noUpdateCheck"`  // não procurar atualizações ao abrir
	SkipVersion    string      `json:"skipVersion"`    // versão que o usuário mandou ignorar
	Configs        []VPNConfig `json:"configs"`
	Apps           []AppEntry  `json:"apps"`
}

type Store struct {
	Dir string
	mu  sync.Mutex
	s   Settings
}

// Open abre a pasta de dados padrão (veja paths.DataDir).
func Open() (*Store, error) { return OpenAt(paths.DataDir()) }

func OpenAt(dir string) (*Store, error) {
	st := &Store{Dir: dir, s: Settings{AutoDisconnect: true}}
	if err := os.MkdirAll(filepath.Join(st.Dir, "configs"), 0o700); err != nil {
		return nil, err
	}
	b, err := os.ReadFile(filepath.Join(st.Dir, "settings.json"))
	if err == nil {
		b = bytes.TrimPrefix(b, []byte{0xEF, 0xBB, 0xBF}) // BOM (ex.: editado no Bloco de Notas)
		if err := json.Unmarshal(b, &st.s); err != nil {
			return nil, fmt.Errorf("settings.json inválido: %w", err)
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
	s.Configs = append([]VPNConfig{}, st.s.Configs...) // nunca nil: a tela espera listas
	s.Apps = append([]AppEntry{}, st.s.Apps...)
	for i := range s.Configs {
		s.Configs[i].PassEnc = "" // não manda a senha para a interface
	}
	return s
}

func (st *Store) SetAutoDisconnect(v bool) error {
	st.mu.Lock()
	defer st.mu.Unlock()
	st.s.AutoDisconnect = v
	return st.saveLocked()
}

// SetUpdateCheck liga/desliga a busca de atualizações ao abrir.
func (st *Store) SetUpdateCheck(v bool) error {
	st.mu.Lock()
	defer st.mu.Unlock()
	st.s.NoUpdateCheck = !v
	return st.saveLocked()
}

// SetSkipVersion guarda uma versão que não deve mais ser oferecida.
func (st *Store) SetSkipVersion(v string) error {
	st.mu.Lock()
	defer st.mu.Unlock()
	st.s.SkipVersion = v
	return st.saveLocked()
}

var unsafeName = regexp.MustCompile(`[^\p{L}\p{N} ._()-]+`)

// SafeName troca os caracteres inválidos em nomes de arquivo por repl.
func SafeName(s, repl string) string {
	return strings.TrimSpace(unsafeName.ReplaceAllString(s, repl))
}

// ImportConfig copia um arquivo de VPN para data\configs.
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

// inspectConfig descobre o tipo da configuração e valida o conteúdo.
func inspectConfig(data []byte, looksOVPN bool) (VPNConfig, error) {
	lower := strings.ToLower(string(data))
	var c VPNConfig
	switch {
	case strings.Contains(lower, "[interface]"):
		c.Kind = "wireguard"
		if _, err := tunnel.ParseWireGuardConfig(data); err != nil {
			return c, fmt.Errorf("WireGuard inválido: %w", err)
		}
	case strings.Contains(lower, "remote ") || strings.Contains(lower, "<connection>") || looksOVPN:
		c.Kind = "openvpn"
		c.NeedAuth = tunnel.OpenVPNNeedsAuth(data)
	default:
		return c, errors.New("o conteúdo não parece uma configuração OpenVPN (.ovpn) nem WireGuard (.conf)")
	}
	return c, nil
}

func (st *Store) sortConfigsLocked() {
	sort.Slice(st.s.Configs, func(i, j int) bool {
		return strings.ToLower(st.s.Configs[i].Name) < strings.ToLower(st.s.Configs[j].Name)
	})
}

// ConfigText devolve o conteúdo do arquivo de uma VPN salva.
func (st *Store) ConfigText(name string) (string, error) {
	st.mu.Lock()
	c := st.configLocked(name)
	if c == nil {
		st.mu.Unlock()
		return "", fmt.Errorf("configuração %q não existe", name)
	}
	path := filepath.Join(st.Dir, "configs", c.File)
	st.mu.Unlock()
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return strings.ReplaceAll(string(bytes.TrimPrefix(b, []byte{0xEF, 0xBB, 0xBF})), "\r\n", "\n"), nil
}

// UpdateConfig renomeia e/ou altera o conteúdo de uma VPN salva. Os apps que
// usam a VPN passam a apontar para o novo nome; as credenciais são mantidas.
func (st *Store) UpdateConfig(name, newName, text string) (VPNConfig, error) {
	newName = SafeName(newName, "_")
	if newName == "" {
		return VPNConfig{}, errors.New("informe um nome para a VPN")
	}
	if strings.TrimSpace(text) == "" {
		return VPNConfig{}, errors.New("o conteúdo da configuração está vazio")
	}
	st.mu.Lock()
	defer st.mu.Unlock()
	c := st.configLocked(name)
	if c == nil {
		return VPNConfig{}, fmt.Errorf("configuração %q não existe", name)
	}
	if newName != name {
		if other := st.configLocked(newName); other != nil {
			return VPNConfig{}, fmt.Errorf("já existe uma VPN chamada %q", newName)
		}
	}
	info, err := inspectConfig([]byte(text), false) // na edição o conteúdo precisa se identificar
	if err != nil {
		return VPNConfig{}, err
	}

	dir := filepath.Join(st.Dir, "configs")
	oldPath := filepath.Join(dir, c.File)
	data := []byte(text)
	if old, err := os.ReadFile(oldPath); err == nil && bytes.Contains(old, []byte("\r\n")) {
		data = []byte(strings.ReplaceAll(text, "\n", "\r\n")) // mantém o fim de linha do Windows
	}
	newFile := newName + kindExt[info.Kind]
	newPath := filepath.Join(dir, newFile)
	if !strings.EqualFold(newPath, oldPath) {
		if _, err := os.Stat(newPath); err == nil {
			return VPNConfig{}, fmt.Errorf("o arquivo %s já existe na pasta de configurações", newFile)
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

// ConfigFor devolve o caminho do arquivo e as opções de conexão.
func (st *Store) ConfigFor(name string) (string, tunnel.Options, error) {
	st.mu.Lock()
	defer st.mu.Unlock()
	c := st.configLocked(name)
	if c == nil {
		return "", tunnel.Options{}, fmt.Errorf("configuração %q não existe", name)
	}
	o := tunnel.Options{NoConsole: true, AuthUser: c.User}
	if c.PassEnc != "" {
		p, err := platform.Unprotect(c.PassEnc)
		if err != nil {
			return "", o, fmt.Errorf("não consegui ler a senha salva: %w", err)
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
		return fmt.Errorf("configuração %q não existe", name)
	}
	c.User, c.PassEnc, c.HasPass = user, enc, enc != ""
	return st.saveLocked()
}

func (st *Store) RemoveConfig(name string) error {
	st.mu.Lock()
	defer st.mu.Unlock()
	for _, a := range st.s.Apps {
		if a.Config == name {
			return fmt.Errorf("o app %q usa esta VPN; remova ou altere o app antes", a.Name)
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

func newID() string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func (st *Store) SaveApp(a AppEntry) (AppEntry, error) {
	a.Name = strings.TrimSpace(a.Name)
	if a.Name == "" || a.Path == "" {
		return a, errors.New("nome e programa são obrigatórios")
	}
	if a.Mode != "proxy" {
		a.Mode = "capture"
	}
	a.Capture = normalizeList(a.Capture)
	if a.Mode == "capture" && len(a.Capture) == 0 {
		return a, errors.New("informe ao menos um processo para capturar (ex.: discord.exe)")
	}
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.configLocked(a.Config) == nil {
		return a, errors.New("escolha uma VPN para o app")
	}
	if a.Autostart {
		// só uma VPN fica ativa por vez
		for _, o := range st.s.Apps {
			if o.ID != a.ID && o.Autostart && o.Config != a.Config {
				return a, fmt.Errorf("só uma VPN fica ativa por vez: os apps que abrem com o sistema precisam usar a mesma VPN (%s abre com a VPN %q)", o.Name, o.Config)
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
			return a, errors.New("app não encontrado")
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
	return errors.New("app não encontrado")
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
	return AppEntry{}, errors.New("app não encontrado")
}

// AutostartApps devolve os apps marcados para abrir com o sistema.
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
