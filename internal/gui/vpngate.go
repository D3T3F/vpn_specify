package gui

import (
	"context"
	"log"
	"sync"

	"vpn_specify/internal/i18n"
	"vpn_specify/internal/store"
	"vpn_specify/internal/vpngate"
)

// gateFinder holds the last VPN Gate search, for "Add" to take the
// configuration from it.
type gateFinder struct {
	store *store.Store

	mu   sync.Mutex
	list *vpngate.List
}

func newGateFinder(st *store.Store) *gateFinder { return &gateFinder{store: st} }

// api: the window functions of the VPN Gate search.
func (g *gateFinder) api() map[string]any {
	return map[string]any{
		"vsVpnGateList": func() (*vpngate.List, error) {
			l, err := vpngate.Fetch(context.Background(), i18n.SystemLocales())
			if err != nil {
				return nil, err
			}
			log.Printf("VPN Gate: %d servers, closest to %q first (%d left out)", len(l.Servers), l.Near, l.Skipped)
			g.mu.Lock()
			g.list = l
			g.mu.Unlock()
			return l, nil
		},
		"vsVpnGateAdd": func(id string) (*store.VPNConfig, error) {
			g.mu.Lock()
			l := g.list
			g.mu.Unlock()
			s, cfg, err := l.Config(id)
			if err != nil {
				return nil, err
			}
			c, err := g.store.AddOpenVPN(s.Name, cfg)
			if err != nil {
				return nil, err
			}
			log.Printf("configuration added from VPN Gate: %s", c.Name)
			return &c, nil
		},
	}
}
