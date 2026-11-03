// Comando vpn_specify: conecta a uma VPN (WireGuard ou OpenVPN) sem alterar
// as rotas do sistema e coloca nela só os apps escolhidos.
//
// Sem argumentos abre a interface gráfica; "up", "run" e "status" são os
// comandos de terminal (veja usage).
package main

import (
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"syscall"

	"vpn_specify/internal/gui"
	"vpn_specify/internal/setup"
	"vpn_specify/internal/tunnel"
)

func usage() {
	fmt.Fprint(os.Stderr, `vpn_specify - VPN só para os apps que você escolher

Uso:
  vpn_specify
      Abre a interface gráfica.

  vpn_specify up <arquivo.conf|arquivo.ovpn> [opções] [-- programa args...]
      Conecta na VPN e abre o proxy local. Fica rodando até Ctrl+C.
      Se um programa for passado depois de "--", ele é aberto já na VPN.

  vpn_specify run [opções] <programa> [args...]
      Abre um programa usando a VPN (precisa de um "up" rodando).

  vpn_specify status [opções]
      Mostra seu IP público direto e pela VPN.

Exemplos:
  vpn_specify up trabalho.ovpn
  vpn_specify up wg0.conf -- chrome
  vpn_specify run chrome
  vpn_specify run firefox
  vpn_specify run curl https://ifconfig.me
  vpn_specify run -profile cliente2 msedge

Use "vpn_specify <comando> -h" para ver as opções de cada comando.
`)
}

func main() {
	log.SetFlags(log.Ltime)
	var err error
	if len(os.Args) < 2 {
		err = gui.Main() // sem argumentos: interface gráfica
		if err != nil && !isCancelled(err) {
			fmt.Fprintln(os.Stderr, "erro:", err)
			os.Exit(1)
		}
		return
	}
	switch os.Args[1] {
	case "launch":
		if len(os.Args) != 3 {
			fmt.Fprintln(os.Stderr, "uso: vpn_specify launch <id-do-app>")
			os.Exit(2)
		}
		if err := gui.LaunchMain(os.Args[2]); err != nil && !isCancelled(err) {
			os.Exit(1)
		}
		return
	case "agent":
		err = gui.AgentMain(os.Args[2:])
		if err != nil {
			os.Exit(1)
		}
		return
	case "openvpn-helper": // Linux: aberto via pkexec pela interface gráfica (veja tunnel.RunOpenVPNHelper)
		if err := tunnel.RunOpenVPNHelper(os.Args[2:]); err != nil {
			var exitErr *exec.ExitError
			if errors.As(err, &exitErr) {
				os.Exit(exitErr.ExitCode())
			}
			fmt.Fprintln(os.Stderr, "erro:", err)
			os.Exit(1)
		}
		return
	case "uninstall": // chamado por Configurações › Aplicativos instalados
		if err := setup.Uninstall(os.Args[2:]); err != nil && !isCancelled(err) {
			os.Exit(1)
		}
		return
	case "up":
		err = cmdUp(os.Args[2:])
	case "run":
		err = cmdRun(os.Args[2:])
	case "status":
		err = cmdStatus(os.Args[2:])
	case "help", "-h", "--help":
		usage()
		return
	default:
		fmt.Fprintf(os.Stderr, "comando desconhecido: %s\n\n", os.Args[1])
		usage()
		os.Exit(2)
	}
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			os.Exit(exitErr.ExitCode())
		}
		fmt.Fprintln(os.Stderr, "erro:", err)
		os.Exit(1)
	}
}

// isCancelled: o usuário recusou o prompt do UAC (ERROR_CANCELLED).
func isCancelled(err error) bool {
	var en syscall.Errno
	return errors.As(err, &en) && en == 1223
}
