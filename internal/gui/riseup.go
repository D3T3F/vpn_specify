package gui

import (
	"context"
	"log"
	"sync"
	"time"

	"vpn_specify/internal/i18n"
	"vpn_specify/internal/latency"
	"vpn_specify/internal/riseup"
	"vpn_specify/internal/store"
)

// riseupFinder holds the last RiseUp search, for "Add" to take the
// configuration from it.
type riseupFinder struct {
	store *store.Store

	mu   sync.Mutex
	list *riseup.List
}

func newRiseupFinder(st *store.Store) *riseupFinder { return &riseupFinder{store: st} }

// api: the window functions of the RiseUp search.
func (r *riseupFinder) api() map[string]any {
	return map[string]any{
		"vsRiseupList": func() (*riseup.List, error) {
			l, err := riseup.Fetch(context.Background())
			if err != nil {
				return nil, err
			}
			log.Printf("RiseUp: %d servers", len(l.Servers))
			r.mu.Lock()
			r.list = l
			r.mu.Unlock()
			return l, nil
		},
		// vsRiseupBest measures the gateways nearest to the user, for "Find the best server".
		"vsRiseupBest": func() ([]latency.Result, error) {
			r.mu.Lock()
			l := r.list
			r.mu.Unlock()
			if l == nil {
				return nil, i18n.Error("this server is no longer on the list; search again")
			}
			// The geolocation request has its own, shorter budget: the probes that
			// follow need their time too.
			locateCtx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
			defer cancel()
			near, err := riseup.Locate(locateCtx)
			if err != nil {
				log.Printf("RiseUp: could not sort the gateways by distance; the list order is used")
				near = nil
			}
			return latency.Rank(context.Background(), l.Targets(near, 12)), nil
		},
		"vsRiseupAdd": func(id string) (*store.VPNConfig, error) {
			r.mu.Lock()
			l := r.list
			r.mu.Unlock()
			s, cfg, err := l.Config(context.Background(), id)
			if err != nil {
				return nil, err
			}
			c, err := r.store.AddOpenVPN(s.Name, cfg)
			if err != nil {
				return nil, err
			}
			log.Printf("configuration added from RiseUp: %s", c.Name)
			return &c, nil
		},
	}
}
