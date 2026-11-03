// Comando installer: gera o vpn_specify-setup.exe, um instalador offline com
// os arquivos do programa embutidos (veja payload_embed.go).
//
// Compile com scripts\build.ps1 -Dist, que prepara a pasta payload\ e usa a
// tag "installer".
//
// Sem argumentos mostra a tela de instalação. Instalação silenciosa:
//
//	vpn_specify-setup.exe /silent [/dir=C:\Pasta] [/nodesktop] [/nostartmenu]
//
// Usados pelo atualizador do vpn_specify (internal/update):
//
//	/update            atualiza a instalação existente (mesma pasta e atalhos)
//	/extract=C:\Pasta  só troca os arquivos de uma cópia portátil
//	/wait=<pid>        espera esse processo fechar antes
//	/run               abre o vpn_specify no fim
package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"vpn_specify/internal/platform"
	"vpn_specify/internal/setup"
)

func main() {
	files, err := payloadFS()
	if err != nil {
		fail(err, false)
	}

	silent, updateMode, run := false, false, false
	extractDir := ""
	var waitPID uint64
	o := setup.Options{Dir: setup.DefaultDir(), Desktop: true, StartMenu: true}
	if d := setup.InstalledDir(); d != "" {
		o.Dir = d // atualização: mesma pasta
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
		// usados pelo atualizador do próprio vpn_specify (internal/update):
		case low == "/update":
			updateMode = true
		case strings.HasPrefix(low, "/extract="):
			extractDir = a[len("/extract="):]
		case strings.HasPrefix(low, "/wait="):
			waitPID, _ = strconv.ParseUint(a[len("/wait="):], 10, 32)
		case low == "/run":
			run = true
		default:
			fail(fmt.Errorf("opção desconhecida: %s", a), silent)
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
	fmt.Println("instalado em", dir)
}

// runUpdate: chamado pelo vpn_specify ao atualizar. Espera ele fechar,
// instala sem perguntar nada (erros aparecem numa caixa de mensagem) e,
// com /run, abre a versão nova.
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
			fail(errors.New("não achei a instalação do vpn_specify para atualizar"), false)
		}
		dir, err = setup.Install(files, o, quiet)
	} else {
		err = setup.Extract(files, extractDir, quiet)
	}
	if err != nil {
		fail(fmt.Errorf("a atualização não foi instalada: %w", err), false)
	}
	if run {
		_ = exec.Command(filepath.Join(dir, setup.MainExe)).Start()
	}
}

func fail(err error, silent bool) {
	if !silent {
		platform.MessageBox("Instalar vpn_specify", "Erro: "+err.Error(), 0x10)
	}
	fmt.Fprintln(os.Stderr, "erro:", err)
	os.Exit(1)
}
