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
	"vpn_specify/internal/i18n"
	"vpn_specify/internal/launcher"
	"vpn_specify/internal/netcheck"
	"vpn_specify/internal/proxy"
	"vpn_specify/internal/tunnel"
)

// Terminal commands: up, run and status. Flag descriptions stay in English
// (like most command-line tools); messages are translated.

func cmdUp(args []string) error {
	fs := flag.NewFlagSet("up", flag.ExitOnError)
	socksAddr := fs.String("socks", engine.DefaultSocksAddr, "local SOCKS5 proxy address")
	httpAddr := fs.String("http", engine.DefaultHTTPAddr, "local HTTP proxy address")
	dnsFlag := fs.String("dns", "", "DNS servers inside the VPN, comma-separated (default: the VPN's)")
	ovpnPath := fs.String("openvpn", "", "path of the openvpn executable (default: found automatically)")
	profile := fs.String("profile", "default", "isolated profile used by the program given after --")
	verbose := fs.Bool("v", false, "verbose log")
	var captures stringList
	fs.Var(&captures, "capture", "capture ALL the traffic (TCP, UDP, voice) of processes with this executable name, e.g. discord.exe (repeatable; Windows only, needs Administrator)")
	var forwards proxy.ForwardList
	fs.Var(&forwards, "L", "forward a local port into the VPN, e.g. udp:5353=10.0.0.5:53 or tcp:3389=srv:3389 (repeatable)")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: vpn_specify up <file.conf|file.ovpn> [options] [-- program args...]")
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
			return i18n.Wrap(err, "capture")
		}
	}

	log.Print(i18n.T("VPN ready. The rest of the computer stays outside the VPN."))
	log.Printf("  SOCKS5: socks5h://%s (TCP, UDP)", *socksAddr)
	log.Printf("  HTTP:   http://%s", *httpAddr)
	for _, f := range forwards {
		log.Print("  " + i18n.T("{0} {1} -> {2} (inside the VPN)", strings.ToUpper(f.Proto), f.Local, f.Remote))
	}
	log.Print(i18n.T("Open apps in the VPN from another terminal:  vpn_specify run <program>"))
	log.Print(i18n.T("Ctrl+C to disconnect."))

	if len(rest) > 0 {
		lo := launcher.Options{Socks: *socksAddr, HTTP: *httpAddr, Profile: *profile}
		if _, err := launcher.Launch(rest[0], rest[1:], lo); err != nil {
			log.Print(i18n.T("could not open {0}: {1}", rest[0], err))
		}
	}

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	for {
		select {
		case <-sig:
			log.Print(i18n.T("disconnecting..."))
			return nil
		case <-tick.C:
			if s := eng.Status(); s.State != engine.StateConnected {
				return errors.New(s.Error)
			}
		}
	}
}

func cmdRun(args []string) error {
	fs := flag.NewFlagSet("run", flag.ExitOnError)
	socksAddr := fs.String("socks", engine.DefaultSocksAddr, "local SOCKS5 proxy address")
	httpAddr := fs.String("http", engine.DefaultHTTPAddr, "local HTTP proxy address")
	profile := fs.String("profile", "default", "isolated profile (browsers get a separate profile folder per name)")
	chromium := fs.Bool("chromium", false, "force the Chromium proxy flags (useful for Electron apps)")
	detach := fs.Bool("detach", false, "do not wait for the program to finish")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: vpn_specify run [options] <program> [args...]")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() == 0 {
		fs.Usage()
		return i18n.Error("enter the program")
	}
	c, err := net.DialTimeout("tcp", *socksAddr, time.Second)
	if err != nil {
		return i18n.Error("no vpn_specify connected on {0}; run \"vpn_specify up <config>\" first", *socksAddr)
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
	httpAddr := fs.String("http", engine.DefaultHTTPAddr, "local HTTP proxy address")
	socksAddr := fs.String("socks", engine.DefaultSocksAddr, "local SOCKS5 proxy address")
	if err := fs.Parse(args); err != nil {
		return err
	}
	defer func() {
		ud, derr := netcheck.UDPDirect()
		uv, verr := netcheck.UDPViaSocks(*socksAddr)
		fmt.Println(i18n.T("UDP direct:      {0}", netcheck.Describe(ud, derr)))
		fmt.Println(i18n.T("UDP through VPN: {0}", netcheck.Describe(uv, verr)))
	}()
	const ipURL = "https://api.ipify.org"
	direct, derr := netcheck.FetchIP(ipURL, nil)
	proxyURL := &url.URL{Scheme: "http", Host: *httpAddr}
	viaVPN, verr := netcheck.FetchIP(ipURL, proxyURL)

	fmt.Println(i18n.T("TCP direct:      {0}", netcheck.Describe(direct, derr)))
	fmt.Println(i18n.T("TCP through VPN: {0}", netcheck.Describe(viaVPN, verr)))
	if verr != nil {
		return i18n.Error("the VPN is not responding; is \"vpn_specify up\" running?")
	}
	if derr == nil && direct == viaVPN {
		fmt.Println(i18n.T("warning: the IPs are the same; the VPN may have no internet exit (internal network only), which is normal for corporate VPNs."))
	}
	return nil
}
