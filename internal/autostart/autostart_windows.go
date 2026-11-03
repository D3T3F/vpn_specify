//go:build windows

package autostart

import (
	"bytes"
	"encoding/xml"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"
	"unicode/utf16"

	"vpn_specify/internal/i18n"
	"vpn_specify/internal/platform"
)

// TaskName is the task name in Task Scheduler.
const TaskName = `vpn_specify\Start apps in VPN`

func esc(s string) string {
	var b bytes.Buffer
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}

// taskXML: fires at this user's logon (15 s later, so the network is up),
// runs in their session with highest privileges, with no time limit
// (vpn_specify stays open while the apps use the VPN) and on battery too.
func taskXML(userID, exe string) string {
	return `<?xml version="1.0" encoding="UTF-16"?>
<Task version="1.2" xmlns="http://schemas.microsoft.com/windows/2004/02/mit/task">
  <RegistrationInfo>
    <Author>vpn_specify</Author>
    <Description>Opens in the VPN the apps marked in vpn_specify to start with the system.</Description>
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

// Enable creates (or updates) the task to run exe at logon. It needs
// Administrator (the interface agent already runs that way).
func Enable(exe string) error {
	u, err := user.Current()
	if err != nil {
		return err
	}
	// schtasks reads the XML as UTF-16 with BOM
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
		return i18n.Error("could not create the startup task: {0} {1}", err, out)
	}
	return nil
}

// Disable removes the task (not an error if it does not exist).
func Disable() error {
	if !Enabled() {
		return nil
	}
	if out, err := schtasks("/Delete", "/TN", TaskName, "/F"); err != nil {
		return i18n.Error("could not remove the startup task: {0} {1}", err, out)
	}
	return nil
}

// Enabled reports whether the task exists.
func Enabled() bool {
	_, err := schtasks("/Query", "/TN", TaskName)
	return err == nil
}
