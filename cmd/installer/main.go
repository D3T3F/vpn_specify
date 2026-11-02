// Comando installer: gera o vpn_specify-setup.exe, um instalador offline com
// os arquivos do programa embutidos (veja payload_embed.go).
//
// Compile com scripts\build.ps1 -Dist, que prepara a pasta payload\ e usa a
// tag "installer".
//
// Sem argumentos mostra a tela de instalação. Instalação silenciosa:
//
//	vpn_specify-setup.exe /silent [/dir=C:\Pasta] [/nodesktop] [/nostartmenu]
package main

import (
	"fmt"
	"os"
	"strings"

	"vpn_specify/internal/platform"
	"vpn_specify/internal/setup"
)

func main() {
	files, err := payloadFS()
	if err != nil {
		fail(err, false)
	}

	silent := false
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
		default:
			fail(fmt.Errorf("opção desconhecida: %s", a), silent)
		}
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

func fail(err error, silent bool) {
	if !silent {
		platform.MessageBox("Instalar vpn_specify", "Erro: "+err.Error(), 0x10)
	}
	fmt.Fprintln(os.Stderr, "erro:", err)
	os.Exit(1)
}
