package main

import (
	"errors"
	"flag"
	"fmt"
	"net/netip"
	"strings"

	"vpn_specify/internal/textutil"
)

type stringList []string

func (l *stringList) String() string { return strings.Join(*l, ",") }
func (l *stringList) Set(s string) error {
	*l = append(*l, textutil.SplitList(s)...)
	return nil
}

// parseWithPositional aceita flags antes e depois do primeiro argumento
// posicional; o que sobrar (ex.: depois de "--") é devolvido em rest.
func parseWithPositional(fs *flag.FlagSet, args []string) (pos string, rest []string, err error) {
	if err := fs.Parse(args); err != nil {
		return "", nil, err
	}
	if fs.NArg() == 0 {
		return "", nil, errors.New("informe o arquivo de configuração da VPN")
	}
	pos = fs.Arg(0)
	if err := fs.Parse(fs.Args()[1:]); err != nil {
		return "", nil, err
	}
	return pos, fs.Args(), nil
}

func parseAddrList(s string) ([]netip.Addr, error) {
	var out []netip.Addr
	for _, f := range textutil.SplitList(s) {
		a, err := netip.ParseAddr(f)
		if err != nil {
			return nil, fmt.Errorf("DNS inválido %q", f)
		}
		out = append(out, a)
	}
	return out, nil
}
