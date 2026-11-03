package main

import (
	"flag"
	"net/netip"
	"strconv"
	"strings"

	"vpn_specify/internal/i18n"
	"vpn_specify/internal/textutil"
)

type stringList []string

func (l *stringList) String() string { return strings.Join(*l, ",") }
func (l *stringList) Set(s string) error {
	*l = append(*l, textutil.SplitList(s)...)
	return nil
}

// parseWithPositional accepts flags before and after the first positional
// argument; whatever is left (e.g. after "--") is returned in rest.
func parseWithPositional(fs *flag.FlagSet, args []string) (pos string, rest []string, err error) {
	if err := fs.Parse(args); err != nil {
		return "", nil, err
	}
	if fs.NArg() == 0 {
		return "", nil, i18n.Error("enter the VPN configuration file")
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
			return nil, i18n.Error("invalid DNS {0}", strconv.Quote(f))
		}
		out = append(out, a)
	}
	return out, nil
}
