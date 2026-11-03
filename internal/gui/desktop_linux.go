//go:build linux

package gui

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"

	"vpn_specify/internal/brand"
	"vpn_specify/internal/platform"
)

// Integração com o desktop Linux (padrões freedesktop.org): os atalhos são
// arquivos .desktop, os apps instalados vêm dos .desktop do sistema, e o
// seletor de arquivos é o zenity (GNOME) ou o kdialog (KDE).

// ---------- argumentos ----------

// splitArgs separa uma linha de comando como o shell faria (aspas simples,
// duplas e barra invertida), sem expandir variáveis.
func splitArgs(s string) []string {
	var out []string
	var cur strings.Builder
	inArg, quote := false, rune(0)
	rs := []rune(s)
	for i := 0; i < len(rs); i++ {
		c := rs[i]
		switch {
		case quote == '\'':
			if c == '\'' {
				quote = 0
			} else {
				cur.WriteRune(c)
			}
		case quote == '"':
			if c == '"' {
				quote = 0
			} else if c == '\\' && i+1 < len(rs) && strings.ContainsRune("\"\\$`", rs[i+1]) {
				i++
				cur.WriteRune(rs[i])
			} else {
				cur.WriteRune(c)
			}
		case c == '\'' || c == '"':
			quote, inArg = c, true
		case c == '\\' && i+1 < len(rs):
			i++
			cur.WriteRune(rs[i])
			inArg = true
		case c == ' ' || c == '\t' || c == '\n':
			if inArg {
				out = append(out, cur.String())
				cur.Reset()
				inArg = false
			}
		default:
			cur.WriteRune(c)
			inArg = true
		}
	}
	if inArg {
		out = append(out, cur.String())
	}
	return out
}

// joinArgs é o inverso de splitArgs (para mostrar e salvar os argumentos).
func joinArgs(args []string) string {
	var parts []string
	for _, a := range args {
		if a != "" && !strings.ContainsAny(a, " \t\n'\"\\$`;&|<>()*?[]{}~#") {
			parts = append(parts, a)
			continue
		}
		parts = append(parts, "'"+strings.ReplaceAll(a, "'", `'\''`)+"'")
	}
	return strings.Join(parts, " ")
}

// desktopQuote escapa um argumento para a chave Exec de um .desktop.
func desktopQuote(a string) string {
	if a != "" && !strings.ContainsAny(a, " \t\n\"'\\><~|&;$*?#()`%") {
		return a
	}
	r := strings.NewReplacer(`\`, `\\\\`, `"`, `\\"`, "`", "\\\\`", `$`, `\\$`, `%`, `%%`)
	return `"` + r.Replace(a) + `"`
}

// ---------- arquivos .desktop ----------

type desktopEntry struct {
	ID, File                         string
	Name, Comment, Exec, Icon, Path  string
	Hidden, NoDisplay, TerminalApp   bool
	OnlyShowIn, NotShowIn, EntryType string
}

// unescapeValue trata os escapes de valores .desktop (\s \n \t \r \\).
func unescapeValue(v string) string {
	return strings.NewReplacer(`\s`, " ", `\n`, "\n", `\t`, "\t", `\r`, "\r", `\\`, `\`).Replace(v)
}

func parseDesktopFile(path string) (desktopEntry, error) {
	f, err := os.Open(path)
	if err != nil {
		return desktopEntry{}, err
	}
	defer f.Close()
	e := desktopEntry{File: path}
	lang := strings.SplitN(os.Getenv("LANG"), ".", 2)[0] // ex.: pt_BR
	short, _, _ := strings.Cut(lang, "_")
	var nameLang, nameShort, commentLang, commentShort string
	section := ""
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "[") {
			section = line
			continue
		}
		if section != "[Desktop Entry]" {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k, v = strings.TrimSpace(k), unescapeValue(strings.TrimSpace(v))
		switch k {
		case "Type":
			e.EntryType = v
		case "Name":
			e.Name = v
		case "Name[" + lang + "]":
			nameLang = v
		case "Name[" + short + "]":
			nameShort = v
		case "Comment":
			e.Comment = v
		case "Comment[" + lang + "]":
			commentLang = v
		case "Comment[" + short + "]":
			commentShort = v
		case "Exec":
			e.Exec = v
		case "Icon":
			e.Icon = v
		case "Path":
			e.Path = v
		case "Hidden":
			e.Hidden = v == "true"
		case "NoDisplay":
			e.NoDisplay = v == "true"
		case "Terminal":
			e.TerminalApp = v == "true"
		case "OnlyShowIn":
			e.OnlyShowIn = v
		case "NotShowIn":
			e.NotShowIn = v
		}
	}
	for _, n := range []string{nameLang, nameShort} {
		if n != "" {
			e.Name = n
			break
		}
	}
	for _, c := range []string{commentLang, commentShort} {
		if c != "" {
			e.Comment = c
			break
		}
	}
	if e.EntryType != "Application" || e.Exec == "" {
		return e, errors.New("não é um aplicativo")
	}
	return e, nil
}

// execCommand converte a chave Exec em programa + argumentos, tirando os
// códigos de campo (%u, %F, ...), que o vpn_specify não usa.
func (e desktopEntry) execCommand() (path string, args []string, err error) {
	var parts []string
	for _, a := range splitArgs(e.Exec) {
		switch a {
		case "%f", "%F", "%u", "%U", "%d", "%D", "%n", "%N", "%v", "%m", "%i", "%c", "%k":
			continue
		}
		parts = append(parts, strings.ReplaceAll(a, "%%", "%"))
	}
	if len(parts) == 0 {
		return "", nil, errors.New("Exec vazio")
	}
	if strings.EqualFold(filepath.Base(parts[0]), "env") { // "env VAR=x programa"
		i := 1
		for i < len(parts) && strings.Contains(parts[i], "=") {
			i++
		}
		if i < len(parts) {
			parts = parts[i:]
		}
	}
	p, err := exec.LookPath(parts[0])
	if err != nil {
		return "", nil, err
	}
	return p, parts[1:], nil
}

func dataHome() string {
	if d := os.Getenv("XDG_DATA_HOME"); d != "" {
		return d
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "share")
}

// dataDirs: onde procurar applications/ e icons/, do mais para o menos
// prioritário (o usuário primeiro, depois flatpak, snap e o sistema).
func dataDirs() []string {
	dirs := []string{dataHome()}
	sys := os.Getenv("XDG_DATA_DIRS")
	if sys == "" {
		sys = "/usr/local/share:/usr/share"
	}
	dirs = append(dirs, strings.Split(sys, ":")...)
	dirs = append(dirs, filepath.Join(dataHome(), "flatpak", "exports", "share"),
		"/var/lib/flatpak/exports/share", "/var/lib/snapd/desktop")
	var out []string
	seen := map[string]bool{}
	for _, d := range dirs {
		if d = filepath.Clean(d); d != "." && !seen[d] {
			seen[d] = true
			out = append(out, d)
		}
	}
	return out
}

type installedApp struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Comment string `json:"comment"`
	Path    string `json:"path"`
	Args    string `json:"args"`
	WorkDir string `json:"workDir"`
	Icon    string `json:"icon"`     // nome do ícone ou caminho (vai para o atalho)
	IconURL string `json:"iconFile"` // arquivo do ícone, para a tela (vazio se não achou)
}

// listInstalledApps lê os .desktop visíveis (como o menu de aplicativos).
func listInstalledApps() []installedApp {
	desktop := strings.Split(strings.ToLower(os.Getenv("XDG_CURRENT_DESKTOP")), ":")
	inDesktop := func(list string) bool {
		for _, d := range strings.Split(strings.ToLower(list), ";") {
			for _, cur := range desktop {
				if d != "" && d == cur {
					return true
				}
			}
		}
		return false
	}
	seen := map[string]bool{}
	var out []installedApp
	for _, d := range dataDirs() {
		root := filepath.Join(d, "applications")
		_ = filepath.WalkDir(root, func(p string, de os.DirEntry, err error) error {
			if err != nil || de.IsDir() || !strings.HasSuffix(p, ".desktop") {
				return nil
			}
			rel, _ := filepath.Rel(root, p)
			id := strings.ReplaceAll(rel, string(filepath.Separator), "-") // regra do padrão
			if seen[id] {
				return nil // o primeiro (mais prioritário) vence
			}
			seen[id] = true
			e, err := parseDesktopFile(p)
			if err != nil || e.Hidden || e.NoDisplay || e.TerminalApp || strings.HasPrefix(id, "vpn_specify") {
				return nil
			}
			if (e.OnlyShowIn != "" && !inDesktop(e.OnlyShowIn)) || (e.NotShowIn != "" && inDesktop(e.NotShowIn)) {
				return nil
			}
			path, args, err := e.execCommand()
			if err != nil {
				return nil
			}
			out = append(out, installedApp{
				ID: id, Name: e.Name, Comment: e.Comment, Path: path, Args: joinArgs(args),
				WorkDir: e.Path, Icon: e.Icon, IconURL: resolveIcon(e.Icon),
			})
			return nil
		})
	}
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name) })
	return out
}

// appFromDesktopFile: o usuário escolheu um .desktop no seletor de arquivos.
func appFromDesktopFile(p string) (*pickedApp, error) {
	e, err := parseDesktopFile(p)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", filepath.Base(p), err)
	}
	path, args, err := e.execCommand()
	if err != nil {
		return nil, fmt.Errorf("%s: programa não encontrado (%v)", filepath.Base(p), err)
	}
	return &pickedApp{Name: e.Name, Path: path, Args: joinArgs(args), WorkDir: e.Path, Icon: e.Icon,
		Capture: []string{filepath.Base(path)}}, nil
}

// resolveIcon acha o arquivo de um ícone do tema (busca simplificada:
// hicolor nos tamanhos comuns, depois pixmaps). Vazio se não achar.
func resolveIcon(name string) string {
	if name == "" {
		return ""
	}
	if filepath.IsAbs(name) {
		if _, err := os.Stat(name); err == nil {
			return name
		}
		return ""
	}
	exts := []string{".png", ".svg"}
	home, _ := os.UserHomeDir()
	bases := []string{filepath.Join(home, ".icons")}
	for _, d := range dataDirs() {
		bases = append(bases, filepath.Join(d, "icons"))
	}
	for _, b := range bases {
		for _, theme := range []string{"hicolor", "Adwaita", "breeze"} {
			for _, size := range []string{"64x64", "48x48", "128x128", "96x96", "256x256", "scalable", "512x512", "32x32"} {
				for _, ext := range exts {
					p := filepath.Join(b, theme, size, "apps", name+ext)
					if _, err := os.Stat(p); err == nil {
						return p
					}
				}
			}
		}
	}
	for _, ext := range append(exts, ".xpm") {
		p := filepath.Join("/usr/share/pixmaps", name+ext)
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

// ---------- atalhos ----------

func applicationsDir() string { return filepath.Join(dataHome(), "applications") }

// desktopDir: a Área de Trabalho do usuário (xdg-user-dir, que respeita o idioma).
func desktopDir() string {
	if out, err := exec.Command("xdg-user-dir", "DESKTOP").Output(); err == nil {
		if d := strings.TrimSpace(string(out)); d != "" {
			return d
		}
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Desktop")
}

// logoIconPath grava o logotipo do vpn_specify para os atalhos usarem.
func logoIconPath() string {
	p := filepath.Join(dataHome(), "vpn_specify", "vpn_specify.png")
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err == nil {
		_ = os.WriteFile(p, brand.LogoPNG(), 0o644)
	}
	return p
}

// writeDesktopFile cria um atalho .desktop. Na Área de Trabalho ele precisa
// ser executável e, no GNOME, marcado como confiável para abrir com clique.
func writeDesktopFile(path, name, comment, execLine, icon string) error {
	if icon == "" {
		icon = logoIconPath()
	}
	esc := func(s string) string { return strings.ReplaceAll(strings.ReplaceAll(s, "\n", " "), "\\", "\\\\") }
	content := "[Desktop Entry]\nType=Application\nVersion=1.0\n" +
		"Name=" + esc(name) + "\n" +
		"Comment=" + esc(comment) + "\n" +
		"Exec=" + execLine + "\n" +
		"Icon=" + esc(icon) + "\n" +
		"Terminal=false\nCategories=Network;\n"
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		return err
	}
	_ = exec.Command("gio", "set", path, "metadata::trusted", "true").Run()
	return nil
}

// ---------- seletor de arquivos ----------

var errNoFileDialog = errors.New("não achei um seletor de arquivos (instale o zenity ou o kdialog); você também pode digitar o caminho do programa no campo Programa")

// pickFile abre o seletor de arquivos do sistema. ok=false se cancelou.
// patterns são globs (ex.: "*.desktop").
func pickFile(title, dir, filterName string, patterns []string) (string, bool, error) {
	var cmd *exec.Cmd
	if p, err := exec.LookPath("zenity"); err == nil {
		args := []string{"--file-selection", "--title=" + title, "--filename=" + strings.TrimSuffix(dir, "/") + "/"}
		if len(patterns) > 0 {
			args = append(args, "--file-filter="+filterName+" | "+strings.Join(patterns, " "), "--file-filter=Todos os arquivos | *")
		}
		cmd = exec.Command(p, args...)
	} else if p, err := exec.LookPath("kdialog"); err == nil {
		filter := ""
		if len(patterns) > 0 {
			filter = strings.Join(patterns, " ") + "|" + filterName + "\n*|Todos os arquivos"
		}
		cmd = exec.Command(p, "--title", title, "--getopenfilename", dir, filter)
	} else {
		return "", false, errNoFileDialog
	}
	out, err := cmd.Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) && ee.ExitCode() == 1 {
			return "", false, nil // cancelado
		}
		return "", false, err
	}
	p := strings.TrimRight(string(out), "\r\n")
	return p, p != "", nil
}

// ---------- processos ----------

// startDetached abre um programa fora da sessão do vpn_specify (ele continua
// aberto se o vpn_specify fechar). Rodando via sudo, abre como o usuário.
func startDetached(path string, args []string, dir string, env []string) (int, error) {
	cmd := exec.Command(path, args...)
	cmd.Dir, cmd.Env = dir, env
	if cmd.Dir == "" {
		cmd.Dir, _ = os.UserHomeDir()
	}
	if fi, err := os.Stat(cmd.Dir); err != nil || !fi.IsDir() {
		cmd.Dir = ""
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true} // grupo de processos próprio (pgid = pid)
	if su := platform.InvokingUser(); su != nil {
		su.Apply(cmd)
	}
	if err := cmd.Start(); err != nil {
		return 0, err
	}
	go cmd.Wait() // evita processo zumbi
	return cmd.Process.Pid, nil
}

// anyRunning diz se ainda existe algum processo nos grupos pgids (os
// processos abertos pelo vpn_specify e seus filhos) ou com um dos nomes de
// executável em names.
func anyRunning(pgids map[int]bool, names map[string]bool) bool {
	ents, _ := os.ReadDir("/proc")
	short := map[string]bool{} // /proc/<pid>/comm guarda só 15 caracteres
	for n := range names {
		if len(n) > 15 {
			n = n[:15]
		}
		short[n] = true
	}
	for _, e := range ents {
		pid, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}
		b, err := os.ReadFile("/proc/" + e.Name() + "/stat")
		if err != nil {
			continue
		}
		// formato: pid (comm) estado ppid pgrp ...; o comm pode ter espaços
		s := string(b)
		i, j := strings.IndexByte(s, '('), strings.LastIndexByte(s, ')')
		if i < 0 || j < i {
			continue
		}
		comm := s[i+1 : j]
		f := strings.Fields(s[j+1:])
		if len(f) >= 3 {
			if pg, err := strconv.Atoi(f[2]); err == nil && pgids[pg] && f[0] != "Z" {
				return true
			}
		}
		if exe, err := os.Readlink(fmt.Sprintf("/proc/%d/exe", pid)); err == nil && names[filepath.Base(exe)] {
			return true
		}
		if short[comm] {
			return true
		}
	}
	return false
}

// openWithDefault abre uma pasta ou URL no programa padrão (xdg-open).
func openWithDefault(target string) error {
	p, err := exec.LookPath("xdg-open")
	if err != nil {
		return errors.New("xdg-open não encontrado")
	}
	_, err = startDetached(p, []string{target}, "", nil)
	return err
}
