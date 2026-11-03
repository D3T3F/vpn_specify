package launcher

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"vpn_specify/internal/platform"
)

type Options struct {
	Socks, HTTP, Profile string
	Chromium             bool // força flags de proxy do Chromium em qualquer programa (Electron)
	Wait                 bool // espera o programa terminar (ignorado para navegadores)
}

var knownPrograms = map[string][]string{
	"chrome":  {`%ProgramFiles%\Google\Chrome\Application\chrome.exe`, `%ProgramFiles(x86)%\Google\Chrome\Application\chrome.exe`, `%LocalAppData%\Google\Chrome\Application\chrome.exe`},
	"msedge":  {`%ProgramFiles(x86)%\Microsoft\Edge\Application\msedge.exe`, `%ProgramFiles%\Microsoft\Edge\Application\msedge.exe`},
	"edge":    {`%ProgramFiles(x86)%\Microsoft\Edge\Application\msedge.exe`, `%ProgramFiles%\Microsoft\Edge\Application\msedge.exe`},
	"brave":   {`%ProgramFiles%\BraveSoftware\Brave-Browser\Application\brave.exe`, `%LocalAppData%\BraveSoftware\Brave-Browser\Application\brave.exe`},
	"vivaldi": {`%LocalAppData%\Vivaldi\Application\vivaldi.exe`, `%ProgramFiles%\Vivaldi\Application\vivaldi.exe`},
	"firefox": {`%ProgramFiles%\Mozilla Firefox\firefox.exe`, `%ProgramFiles(x86)%\Mozilla Firefox\firefox.exe`},
}

// unixPrograms: nomes curtos (os mesmos do Windows) -> comandos usados pelas
// distribuições Linux.
var unixPrograms = map[string][]string{
	"chrome":   {"google-chrome-stable", "google-chrome", "chromium", "chromium-browser"},
	"msedge":   {"microsoft-edge-stable", "microsoft-edge"},
	"edge":     {"microsoft-edge-stable", "microsoft-edge"},
	"brave":    {"brave-browser", "brave"},
	"vivaldi":  {"vivaldi-stable", "vivaldi"},
	"chromium": {"chromium-browser", "chromium"},
}

var chromiumBrowsers = map[string]bool{
	"chrome": true, "msedge": true, "brave": true, "vivaldi": true, "opera": true, "chromium": true, "thorium": true,
	// nomes dos executáveis no Linux
	"google-chrome": true, "google-chrome-stable": true, "google-chrome-beta": true, "google-chrome-unstable": true,
	"chromium-browser": true, "microsoft-edge": true, "microsoft-edge-stable": true, "microsoft-edge-beta": true,
	"brave-browser": true, "vivaldi-stable": true,
}
var firefoxBrowsers = map[string]bool{"firefox": true, "firefox-esr": true, "librewolf": true, "waterfox": true}

func resolveProgram(name string) (string, error) {
	if p, err := exec.LookPath(name); err == nil {
		return p, nil
	}
	key := strings.TrimSuffix(strings.ToLower(name), ".exe")
	if runtime.GOOS != "windows" {
		for _, c := range unixPrograms[key] {
			if p, err := exec.LookPath(c); err == nil {
				return p, nil
			}
		}
		return "", fmt.Errorf("programa %q não encontrado (passe o caminho completo)", name)
	}
	for _, c := range knownPrograms[key] {
		p := platform.ExpandEnv(c)
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	return "", fmt.Errorf("programa %q não encontrado (passe o caminho completo do .exe)", name)
}

// profileDir cria a pasta de perfil do navegador:
//   - Windows: %LOCALAPPDATA%\vpn_specify\profiles\<perfil>\<app>;
//   - Linux/macOS: ~/.config/vpn_specify/profiles/<perfil>/<app>, ou
//     ~/snap/<app>/common/vpn_specify/profiles/<perfil> para apps em snap
//     (Firefox e Chromium do Ubuntu), que não podem ler pastas ocultas da home.
//
// Rodando via sudo, a pasta fica na home de quem chamou o sudo, com esse
// usuário como dono.
func profileDir(profile, app string, su *platform.SudoUser) (string, error) {
	var dir string
	if runtime.GOOS == "windows" {
		base := os.Getenv("LOCALAPPDATA")
		if base == "" {
			var err error
			if base, err = os.UserConfigDir(); err != nil {
				return "", err
			}
		}
		dir = filepath.Join(base, "vpn_specify", "profiles", profile, app)
	} else {
		home, err := os.UserHomeDir()
		if su != nil {
			home, err = su.Home, nil
		}
		if err != nil {
			return "", err
		}
		config := os.Getenv("XDG_CONFIG_HOME")
		if config == "" || su != nil {
			config = filepath.Join(home, ".config")
		}
		dir = filepath.Join(config, "vpn_specify", "profiles", profile, app)
		if _, err := os.Stat(filepath.Join("/snap/bin", app)); err == nil {
			dir = filepath.Join(home, "snap", app, "common", "vpn_specify", "profiles", profile)
		}
	}
	if su != nil {
		return dir, su.MkdirAllOwned(dir, 0o700)
	}
	return dir, os.MkdirAll(dir, 0o700)
}

func ProxyEnv(o Options) []string {
	vars := map[string]string{
		"HTTP_PROXY":  "http://" + o.HTTP,
		"HTTPS_PROXY": "http://" + o.HTTP,
		"ALL_PROXY":   "socks5h://" + o.Socks,
		"NO_PROXY":    "localhost,127.0.0.1,::1",
		"VPN_SPECIFY": "1",
	}
	var env []string
	for k, v := range vars {
		env = append(env, k+"="+v)
		if runtime.GOOS != "windows" { // no Windows as variáveis não diferenciam maiúsculas
			env = append(env, strings.ToLower(k)+"="+v)
		}
	}
	return env
}

func writeFirefoxPrefs(dir, socks string, su *platform.SudoUser) error {
	host, port, _ := strings.Cut(socks, ":")
	prefs := fmt.Sprintf(`// gerado pelo vpn_specify
user_pref("network.proxy.type", 1);
user_pref("network.proxy.socks", %q);
user_pref("network.proxy.socks_port", %s);
user_pref("network.proxy.socks_version", 5);
user_pref("network.proxy.socks_remote_dns", true);
user_pref("network.proxy.no_proxies_on", "");
user_pref("network.proxy.allow_hijacking_localhost", false);
user_pref("network.trr.mode", 5);
user_pref("media.peerconnection.ice.proxy_only", true);
user_pref("browser.shell.checkDefaultBrowser", false);
`, host, port)
	p := filepath.Join(dir, "user.js")
	if err := os.WriteFile(p, []byte(prefs), 0o600); err != nil {
		return err
	}
	if su != nil {
		return su.Chown(p)
	}
	return nil
}

// Launch abre o programa configurado para sair pela VPN:
//   - todos recebem HTTP_PROXY/HTTPS_PROXY/ALL_PROXY (curl, git, npm, pip, ...);
//   - navegadores Chromium recebem --proxy-server e um perfil separado
//     (senão a janela abriria no Chrome que já está rodando, fora da VPN);
//   - Firefox recebe um perfil próprio com o proxy configurado.
//
// Se o vpn_specify roda como root via sudo (OpenVPN no Linux), o programa é
// aberto como o usuário que chamou o sudo.
func Launch(name string, args []string, o Options) (*exec.Cmd, error) {
	path, err := resolveProgram(name)
	if err != nil {
		return nil, err
	}
	pre, browser, err := ProxyArgsFor(path, o)
	if err != nil {
		return nil, err
	}
	app := strings.TrimSuffix(strings.ToLower(filepath.Base(path)), ".exe")

	cmd := exec.Command(path, append(pre, args...)...)
	cmd.Env = append(os.Environ(), ProxyEnv(o)...)
	if su := platform.InvokingUser(); su != nil {
		su.Apply(cmd)
	}
	if o.Wait && !browser {
		cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
		return cmd, cmd.Run()
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	if browser {
		fmt.Fprintf(os.Stderr, "%s aberto na VPN (perfil %q)\n", app, o.Profile)
	}
	if cmd.Process == nil {
		return nil, errors.New("falha ao iniciar")
	}
	_ = cmd.Process.Release()
	return cmd, nil
}

// ProxyArgsFor devolve os argumentos extras para o programa usar o proxy
// (navegadores e apps Chromium); browser=true se ganhou perfil próprio.
func ProxyArgsFor(path string, o Options) (pre []string, browser bool, err error) {
	app := strings.TrimSuffix(strings.ToLower(filepath.Base(path)), ".exe")
	su := platform.InvokingUser()
	switch {
	case chromiumBrowsers[app] || o.Chromium:
		pre = []string{
			"--proxy-server=socks5://" + o.Socks,
			"--host-resolver-rules=MAP * ~NOTFOUND , EXCLUDE 127.0.0.1",
			"--force-webrtc-ip-handling-policy=disable_non_proxied_udp",
		}
		if chromiumBrowsers[app] {
			dir, err := profileDir(o.Profile, app, su)
			if err != nil {
				return nil, false, err
			}
			pre = append(pre, "--user-data-dir="+dir, "--no-first-run", "--no-default-browser-check")
			browser = true
		}
	case firefoxBrowsers[app]:
		dir, err := profileDir(o.Profile, app, su)
		if err != nil {
			return nil, false, err
		}
		if err := writeFirefoxPrefs(dir, o.Socks, su); err != nil {
			return nil, false, err
		}
		pre = []string{"-no-remote", "-profile", dir}
		browser = true
	}
	return pre, browser, nil
}
