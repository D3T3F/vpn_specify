//go:build windows

package autostart

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"
	"unicode/utf16"

	"vpn_specify/internal/platform"
)

// TaskName é o nome da tarefa no Agendador de Tarefas.
const TaskName = `vpn_specify\Iniciar apps na VPN`

func esc(s string) string {
	var b bytes.Buffer
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}

// taskXML: dispara no logon deste usuário (15 s depois, para a rede subir),
// roda na sessão dele com privilégio máximo, sem limite de tempo (o
// vpn_specify fica aberto enquanto os apps usam a VPN) e também na bateria.
func taskXML(userID, exe string) string {
	return `<?xml version="1.0" encoding="UTF-16"?>
<Task version="1.2" xmlns="http://schemas.microsoft.com/windows/2004/02/mit/task">
  <RegistrationInfo>
    <Author>vpn_specify</Author>
    <Description>Abre na VPN os apps marcados para iniciar com o sistema no vpn_specify.</Description>
  </RegistrationInfo>
  <Triggers>
    <LogonTrigger>
      <Enabled>true</Enabled>
      <UserId>` + esc(userID) + `</UserId>
      <Delay>PT15S</Delay>
    </LogonTrigger>
  </Triggers>
  <Principals>
    <Principal id="Author">
      <UserId>` + esc(userID) + `</UserId>
      <LogonType>InteractiveToken</LogonType>
      <RunLevel>HighestAvailable</RunLevel>
    </Principal>
  </Principals>
  <Settings>
    <MultipleInstancesPolicy>IgnoreNew</MultipleInstancesPolicy>
    <DisallowStartIfOnBatteries>false</DisallowStartIfOnBatteries>
    <StopIfGoingOnBatteries>false</StopIfGoingOnBatteries>
    <AllowHardTerminate>true</AllowHardTerminate>
    <StartWhenAvailable>false</StartWhenAvailable>
    <RunOnlyIfNetworkAvailable>false</RunOnlyIfNetworkAvailable>
    <AllowStartOnDemand>true</AllowStartOnDemand>
    <Enabled>true</Enabled>
    <Hidden>false</Hidden>
    <ExecutionTimeLimit>PT0S</ExecutionTimeLimit>
    <Priority>7</Priority>
  </Settings>
  <Actions Context="Author">
    <Exec>
      <Command>"` + esc(exe) + `"</Command>
      <Arguments>` + Arg + `</Arguments>
      <WorkingDirectory>` + esc(filepath.Dir(exe)) + `</WorkingDirectory>
    </Exec>
  </Actions>
</Task>
`
}

func schtasks(args ...string) (string, error) {
	cmd := exec.Command(filepath.Join(os.Getenv("SystemRoot"), "System32", "schtasks.exe"), args...)
	platform.HideConsole(cmd)
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

// Enable cria (ou atualiza) a tarefa para rodar exe no logon. Precisa de
// Administrador (o agente da interface já roda assim).
func Enable(exe string) error {
	u, err := user.Current()
	if err != nil {
		return err
	}
	// o schtasks lê o XML em UTF-16 com BOM
	var b bytes.Buffer
	b.Write([]byte{0xFF, 0xFE})
	for _, r := range utf16.Encode([]rune(taskXML(u.Username, exe))) {
		b.WriteByte(byte(r))
		b.WriteByte(byte(r >> 8))
	}
	f, err := os.CreateTemp("", "vpn_specify-task-*.xml")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err := f.Write(b.Bytes()); err != nil {
		f.Close()
		return err
	}
	f.Close()
	if out, err := schtasks("/Create", "/TN", TaskName, "/XML", f.Name(), "/F"); err != nil {
		return fmt.Errorf("não consegui criar a tarefa de inicialização: %v %s", err, out)
	}
	return nil
}

// Disable remove a tarefa (não é erro se ela não existir).
func Disable() error {
	if !Enabled() {
		return nil
	}
	if out, err := schtasks("/Delete", "/TN", TaskName, "/F"); err != nil {
		return fmt.Errorf("não consegui remover a tarefa de inicialização: %v %s", err, out)
	}
	return nil
}

// Enabled diz se a tarefa existe.
func Enabled() bool {
	_, err := schtasks("/Query", "/TN", TaskName)
	return err == nil
}
