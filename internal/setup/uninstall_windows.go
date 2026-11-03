//go:build windows

package setup

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"

	"vpn_specify/internal/autostart"
	"vpn_specify/internal/paths"
	"vpn_specify/internal/platform"
)

// Uninstall remove o vpn_specify instalado na pasta deste executável
// ("vpn_specify.exe uninstall", chamado por Aplicativos instalados).
//
// Opções: /silent (sem perguntas nem mensagens; apaga os dados, a não ser
// com /keepdata).
func Uninstall(args []string) error {
	silent, keepData := false, false
	for _, a := range args {
		switch strings.ToLower(a) {
		case "/silent", "-silent":
			silent = true
		case "/keepdata", "-keepdata":
			keepData = true
		}
	}
	if !platform.IsElevated() {
		return platform.ElevateSelf(append([]string{"uninstall"}, args...))
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	dir := filepath.Dir(exe)
	dataDir := paths.DataDir()
	portableData := strings.EqualFold(dataDir, filepath.Join(dir, "data"))
	notify := func(text string, flags uintptr) {
		if silent {
			fmt.Fprintln(os.Stderr, text)
		} else {
			platform.MessageBox("Desinstalar o vpn_specify", text, flags)
		}
	}

	removeData := !keepData
	if !silent {
		answer := platform.Ask("Desinstalar o vpn_specify",
			"Desinstalar o vpn_specify de "+dir+"?\n\n"+
				"Sim: remove o programa e também seus dados (VPNs importadas, apps e senhas salvas).\n"+
				"Não: remove só o programa e mantém os dados para uma reinstalação.\n"+
				"Cancelar: não desinstala.", true)
		if answer == platform.AnswerCancel {
			return nil
		}
		removeData = answer == platform.AnswerYes
	}

	if len(RunningInstances(dir)) > 0 {
		notify("Não foi possível desinstalar: "+errRunning.Error()+".", 0x30)
		return errRunning
	}

	var problems []string
	// atalhos: os do instalador e os criados pelo app ("DiscordVPN" etc.), que apontariam para um exe apagado
	for _, lnk := range append([]string{
		filepath.Join(platform.StartMenuDir(), shortcutName),
		filepath.Join(platform.DesktopDir(), shortcutName),
	}, appShortcuts(dataDir)...) {
		if err := os.Remove(lnk); err != nil && !errors.Is(err, os.ErrNotExist) {
			problems = append(problems, lnk)
		}
	}
	// tarefa agendada dos apps que abrem com o sistema
	if err := autostart.Disable(); err != nil {
		problems = append(problems, "tarefa "+autostart.TaskName)
	}
	if err := registry.DeleteKey(registry.LOCAL_MACHINE, uninstallKey); err != nil && !errors.Is(err, registry.ErrNotExist) {
		problems = append(problems, "registro de desinstalação")
	}
	if removeData {
		if err := os.RemoveAll(dataDir); err != nil {
			problems = append(problems, dataDir)
		}
	}

	// só os arquivos que o instalador colocou; a pasta só some se ficar vazia
	for _, f := range []string{CLIExe, `windivert\WinDivert.dll`, `windivert\WinDivert64.sys`, `windivert\LICENSE`, "LEIA-ME.txt", "LICENSE.txt"} {
		removeOrDelay(filepath.Join(dir, f))
	}
	_ = os.Remove(filepath.Join(dir, "windivert"))
	if removeData && portableData {
		_ = os.RemoveAll(filepath.Join(dir, "data"))
	}
	deleteSelfLater(exe, dir)

	msg := "O vpn_specify foi desinstalado."
	if !removeData {
		msg += "\n\nSeus dados foram mantidos em " + dataDir + "."
	}
	if len(problems) > 0 {
		msg += "\n\nNão consegui remover:\n" + strings.Join(problems, "\n")
	}
	notify(msg, 0x40)
	if len(problems) > 0 {
		return fmt.Errorf("desinstalação incompleta: %s", strings.Join(problems, "; "))
	}
	return nil
}

// appShortcuts lê, das configurações, os atalhos criados pelo app (e as
// cópias de mesmo nome na Área de Trabalho).
func appShortcuts(dataDir string) []string {
	b, err := os.ReadFile(filepath.Join(dataDir, "settings.json"))
	if err != nil {
		return nil
	}
	var s struct { // só o que interessa de settings.json
		Apps []struct {
			Shortcut string `json:"shortcut"`
		} `json:"apps"`
	}
	if json.Unmarshal(bytes.TrimPrefix(b, []byte{0xEF, 0xBB, 0xBF}), &s) != nil {
		return nil
	}
	var out []string
	for _, a := range s.Apps {
		if a.Shortcut != "" {
			out = append(out, a.Shortcut, filepath.Join(platform.DesktopDir(), filepath.Base(a.Shortcut)))
		}
	}
	return out
}

// removeOrDelay apaga o arquivo ou, se ele estiver em uso (ex.: driver
// WinDivert carregado), agenda a remoção para a próxima reinicialização.
func removeOrDelay(path string) {
	err := os.Remove(path)
	if err == nil || errors.Is(err, os.ErrNotExist) {
		return
	}
	p, _ := windows.UTF16PtrFromString(path)
	_ = windows.MoveFileEx(p, nil, windows.MOVEFILE_DELAY_UNTIL_REBOOT)
}

// deleteSelfLater apaga este .exe (e a pasta, se vazia) depois que o
// processo terminar: um executável não consegue apagar a si mesmo.
func deleteSelfLater(exe, dir string) {
	script := fmt.Sprintf(`ping -n 3 127.0.0.1 >nul & del /f /q "%s" & rmdir "%s"`, exe, dir)
	cmd := exec.Command(filepath.Join(os.Getenv("WINDIR"), "System32", "cmd.exe"))
	cmd.SysProcAttr = &syscall.SysProcAttr{CmdLine: `cmd.exe /d /c "` + script + `"`, HideWindow: true, CreationFlags: 0x08000000} // CREATE_NO_WINDOW
	cmd.Dir = os.TempDir()
	if err := cmd.Start(); err != nil {
		removeOrDelay(exe)
		return
	}
	_ = cmd.Process.Release()
}
