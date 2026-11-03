//go:build windows

package platform

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows"

	"vpn_specify/internal/i18n"
)

// ---------- Shortcuts (.lnk) via WScript.Shell ----------

func powershell(script string, env map[string]string) ([]byte, error) {
	cmd := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", script)
	HideConsole(cmd)
	cmd.Env = os.Environ()
	for k, v := range env { // values passed as environment variables: no quoting issues
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	out, err := cmd.Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return out, fmt.Errorf("%v: %s", err, strings.TrimSpace(string(ee.Stderr)))
		}
	}
	return out, err
}

type shortcutInfo struct {
	Target  string `json:"t"`
	Args    string `json:"a"`
	WorkDir string `json:"w"`
	Icon    string `json:"i"`
}

func ResolveShortcut(lnk string) (shortcutInfo, error) {
	out, err := powershell(`$s=(New-Object -ComObject WScript.Shell).CreateShortcut($env:VS_LNK);`+
		`[Console]::OutputEncoding=[Text.Encoding]::UTF8;`+
		`@{t=$s.TargetPath;a=$s.Arguments;w=$s.WorkingDirectory;i=$s.IconLocation}|ConvertTo-Json -Compress`,
		map[string]string{"VS_LNK": lnk})
	if err != nil {
		return shortcutInfo{}, err
	}
	var si shortcutInfo
	if err := json.Unmarshal(out, &si); err != nil {
		return shortcutInfo{}, i18n.Wrap(err, "reading the shortcut")
	}
	if si.Target == "" {
		return si, i18n.Error("this shortcut does not point to an .exe (it may be a Microsoft Store app); choose the .exe")
	}
	return si, nil
}

func CreateShortcut(lnk, target, args, workDir, icon, desc string) error {
	if err := os.MkdirAll(filepath.Dir(lnk), 0o755); err != nil {
		return err
	}
	_, err := powershell(`$s=(New-Object -ComObject WScript.Shell).CreateShortcut($env:VS_LNK);`+
		`$s.TargetPath=$env:VS_T;$s.Arguments=$env:VS_A;$s.WorkingDirectory=$env:VS_W;`+
		`if($env:VS_I){$s.IconLocation=$env:VS_I};$s.Description=$env:VS_D;$s.Save()`,
		map[string]string{"VS_LNK": lnk, "VS_T": target, "VS_A": args, "VS_W": workDir, "VS_I": icon, "VS_D": desc})
	return err
}

func StartMenuDir() string {
	p, err := windows.KnownFolderPath(windows.FOLDERID_Programs, 0)
	if err != nil {
		return filepath.Join(os.Getenv("APPDATA"), `Microsoft\Windows\Start Menu\Programs`)
	}
	return p
}

func DesktopDir() string {
	p, err := windows.KnownFolderPath(windows.FOLDERID_Desktop, 0)
	if err != nil {
		return filepath.Join(os.Getenv("USERPROFILE"), "Desktop")
	}
	return p
}
