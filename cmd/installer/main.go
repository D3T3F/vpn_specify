// Command installer builds vpn_specify-setup.exe, an offline installer with
// the program files embedded (see payload_embed.go).
//
// Build it with scripts\build.ps1 -Dist, which prepares the payload\ folder
// and uses the "installer" tag.
//
// Without arguments it shows the installer window. Silent install:
//
//	vpn_specify-setup.exe /silent [/dir=C:\Folder] [/nodesktop] [/nostartmenu] [/lang=pt-BR]
//
// Used by the vpn_specify updater (internal/update):
//
//	/update             updates the existing installation (same folder and shortcuts)
//	/extract=C:\Folder  only replaces the files of a portable copy
//	/wait=<pid>         waits for that process to exit first
//	/run                opens vpn_specify at the end
package main

import (
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"vpn_specify/internal/i18n"
	"vpn_specify/internal/platform"
	"vpn_specify/internal/setup"
)

func main() {
	i18n.Init()
	files, err := payloadFS()
	if err != nil {
		fail(err, false)
	}

	silent, updateMode, run := false, false, false
	extractDir := ""
	var waitPID uint64
	o := setup.Options{Dir: setup.DefaultDir(), Desktop: true, StartMenu: true}
	if d := setup.InstalledDir(); d != "" {
		o.Dir = d // update: same folder
	}
	for _, a := range os.Args[1:] {
		low := strings.ToLower(a)
		switch {
		case low == "/silent" || low == "/s":
			silent = true
		case strings.HasPrefix(low, "/dir="):
			o.Dir = a[len("/dir="):]
		case low == "/nodesktop":
			o.Desktop = false
		case low == "/nostartmenu":
			o.StartMenu = false
		// used by vpn_specify's own updater (internal/update):
		case low == "/update":
			updateMode = true
		case strings.HasPrefix(low, "/extract="):
			extractDir = a[len("/extract="):]
		case strings.HasPrefix(low, "/wait="):
			waitPID, _ = strconv.ParseUint(a[len("/wait="):], 10, 32)
		case low == "/run":
			run = true
		case strings.HasPrefix(low, "/lang="):
			o.Language = a[len("/lang="):]
			i18n.Set(o.Language)
		default:
			fail(i18n.Error("unknown option: {0}", a), silent)
		}
	}

	if updateMode || extractDir != "" {
		runUpdate(files, updateMode, extractDir, uint32(waitPID), run)
		return
	}

	if !silent {
		if err := setup.RunInstaller(files); err != nil {
			fail(err, false)
		}
		return
	}
	if !platform.IsElevated() {
		if err := platform.ElevateSelf(os.Args[1:]); err != nil {
			fail(err, true)
		}
		return
	}
	dir, err := setup.Install(files, o, func(msg string) { fmt.Println(msg) })
	if err != nil {
		fail(err, true)
	}
	if o.Language != "" {
		_ = i18n.Save(o.Language)
	}
	fmt.Println(i18n.T("installed in {0}", dir))
}

// runUpdate: called by vpn_specify when updating. It waits for it to exit,
// installs without asking anything (errors appear in a message box) and,
// with /run, opens the new version.
func runUpdate(files fs.FS, installed bool, extractDir string, waitPID uint32, run bool) {
	if !platform.IsElevated() && installed {
		if err := platform.ElevateSelf(os.Args[1:]); err != nil {
			fail(err, false)
		}
		return
	}
	if waitPID != 0 {
		setup.WaitProcessExit(waitPID, 60*time.Second)
	}
	quiet := func(string) {}
	dir := extractDir
	var err error
	if installed {
		o := setup.UpdateOptions()
		if o.Dir == "" {
			fail(i18n.Error("could not find the vpn_specify installation to update"), false)
		}
		dir, err = setup.Install(files, o, quiet)
	} else {
		err = setup.Extract(files, extractDir, quiet)
	}
	if err != nil {
		fail(i18n.Wrap(err, "the update was not installed"), false)
	}
	if run {
		_ = exec.Command(filepath.Join(dir, setup.MainExe)).Start()
	}
}

func fail(err error, silent bool) {
	if !silent {
		platform.MessageBox(i18n.T("Install vpn_specify"), i18n.T("Error: {0}", err), 0x10)
	}
	fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
}
