package main

import (
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"vpn_specify/internal/engine"
	"vpn_specify/internal/launcher"
	"vpn_specify/internal/netcheck"
	"vpn_specify/internal/proxy"
	"vpn_specify/internal/tunnel"
)

// Comandos de terminal: up, run e status.

func cmdUp(args []string) error {
	fs := flag.NewFlagSet("up", flag.ExitOnError)
	socksAddr := fs.String("socks", engine.DefaultSocksAddr, "endereço do proxy SOCKS5 local")
	httpAddr := fs.String("http", engine.DefaultHTTPAddr, "endereço do proxy HTTP local")
	dnsFlag := fs.String("dns", "", "servidores DNS dentro da VPN, separados por vírgula (padrão: os da VPN)")
	ovpnPath := fs.String("openvpn", "", "caminho do openvpn.exe (padrão: procura automaticamente)")
	profile := fs.String("profile", "default", "perfil isolado usado pelo programa passado depois de --")
	verbose := fs.Bool("v", false, "log detalhado")
	var captures stringList
	fs.Var(&captures, "capture", "captura TODO o tráfego (TCP, UDP, voz) dos processos com esse nome de executável, ex.: discord.exe (pode repetir; precisa de Administrador)")
	var forwards proxy.ForwardList
	fs.Var(&forwards, "L", "encaminha uma porta local para dentro da VPN, ex.: udp:5353=10.0.0.5:53 ou tcp:3389=srv:3389 (pode repetir)")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "Uso: vpn_specify up <arquivo.conf|arquivo.ovpn> [opções] [-- programa args...]")
		fs.PrintDefaults()
	}

	config, rest, err := parseWithPositional(fs, args)
	if err != nil {
		fs.Usage()
		return err
	}
	dns, err := parseAddrList(*dnsFlag)
	if err != nil {
		return err
	}

	eng := engine.New()
	eng.SocksAddr, eng.HTTPAddr, eng.Verbose = *socksAddr, *httpAddr, *verbose
	if err := eng.Connect(config, config, tunnel.Options{DNS: dns, OpenVPNPath: *ovpnPath, Verbose: *verbose}, forwards); err != nil {
		return err
	}
	defer eng.Disconnect()
	if len(captures) > 0 {
		if err := eng.EnsureCapture(captures); err != nil {
			return fmt.Errorf("captura: %w", err)
		}
	}

	log.Printf("VPN pronta. O resto do computador continua fora da VPN.")
	log.Printf("  SOCKS5: socks5h://%s (TCP e UDP)", *socksAddr)
	log.Printf("  HTTP:   http://%s", *httpAddr)
	for _, f := range forwards {
		log.Printf("  %s %s -> %s (dentro da VPN)", strings.ToUpper(f.Proto), f.Local, f.Remote)
	}
	log.Printf("Abra apps na VPN em outro terminal:  vpn_specify run <programa>")
	log.Printf("Ctrl+C para desconectar.")

	if len(rest) > 0 {
		lo := launcher.Options{Socks: *socksAddr, HTTP: *httpAddr, Profile: *profile}
		if _, err := launcher.Launch(rest[0], rest[1:], lo); err != nil {
			log.Printf("não consegui abrir %s: %v", rest[0], err)
		}
	}

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	for {
		select {
		case <-sig:
			log.Printf("desconectando...")
			return nil
		case <-tick.C:
			if s := eng.Status(); s.State != "conectado" {
				return errors.New(s.Error)
			}
		}
	}
}

func cmdRun(args []string) error {
	fs := flag.NewFlagSet("run", flag.ExitOnError)
	socksAddr := fs.String("socks", engine.DefaultSocksAddr, "endereço do proxy SOCKS5 local")
	httpAddr := fs.String("http", engine.DefaultHTTPAddr, "endereço do proxy HTTP local")
	profile := fs.String("profile", "default", "perfil isolado (navegadores ganham uma pasta de perfil separada por nome)")
	chromium := fs.Bool("chromium", false, "força as flags de proxy do Chromium (útil para apps Electron)")
	detach := fs.Bool("detach", false, "não espera o programa terminar")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "Uso: vpn_specify run [opções] <programa> [args...]")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() == 0 {
		fs.Usage()
		return errors.New("informe o programa")
	}
	c, err := net.DialTimeout("tcp", *socksAddr, time.Second)
	if err != nil {
		return fmt.Errorf("nenhum vpn_specify conectado em %s; rode \"vpn_specify up <config>\" primeiro", *socksAddr)
	}
	c.Close()

	_, err = launcher.Launch(fs.Arg(0), fs.Args()[1:], launcher.Options{
		Socks: *socksAddr, HTTP: *httpAddr, Profile: *profile,
		Chromium: *chromium, Wait: !*detach,
	})
	return err
}

func cmdStatus(args []string) error {
	fs := flag.NewFlagSet("status", flag.ExitOnError)
	httpAddr := fs.String("http", engine.DefaultHTTPAddr, "endereço do proxy HTTP local")
	socksAddr := fs.String("socks", engine.DefaultSocksAddr, "endereço do proxy SOCKS5 local")
	if err := fs.Parse(args); err != nil {
		return err
	}
	defer func() {
		ud, derr := netcheck.UDPDirect()
		uv, verr := netcheck.UDPViaSocks(*socksAddr)
		fmt.Printf("UDP direto:   %s\n", netcheck.Describe(ud, derr))
		fmt.Printf("UDP pela VPN: %s\n", netcheck.Describe(uv, verr))
	}()
	const ipURL = "https://api.ipify.org"
	direct, derr := netcheck.FetchIP(ipURL, nil)
	proxyURL := &url.URL{Scheme: "http", Host: *httpAddr}
	viaVPN, verr := netcheck.FetchIP(ipURL, proxyURL)

	if derr != nil {
		fmt.Printf("TCP direto:   erro (%v)\n", derr)
	} else {
		fmt.Printf("TCP direto:   %s\n", direct)
	}
	if verr != nil {
		fmt.Printf("TCP pela VPN: erro (%v)\n", verr)
		return errors.New("VPN não está respondendo; o \"vpn_specify up\" está rodando?")
	}
	fmt.Printf("TCP pela VPN: %s\n", viaVPN)
	if derr == nil && direct == viaVPN {
		fmt.Println("aviso: os IPs são iguais; a VPN pode não ter saída para a internet (só rede interna), o que é normal em VPNs corporativas.")
	}
	return nil
}
