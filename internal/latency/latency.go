// Package latency measures how long a TCP connection takes to open to each of a
// list of servers, to pick the closest one. It needs no privileges: ICMP needs
// Administrator rights on Windows and does not work on Android.
package latency

import (
	"context"
	"net"
	"strconv"
	"sync"
	"time"
)

const (
	dialTimeout = 2 * time.Second
	attempts    = 3
	gap         = 100 * time.Millisecond
	workers     = 8
	deadline    = 10 * time.Second
)

// Target is one server to measure: its address and the TCP ports to try.
type Target struct {
	ID    string
	IP    string
	Ports []int
}

// Result is the latency of one target.
type Result struct {
	ID string `json:"id"`
	Ms int    `json:"ms"` // -1 = could not measure
}

// Rank measures the targets, a few at a time, and returns one Result per target
// in the same order. The latency of a target is the shortest connect time over
// its ports and attempts. Targets not measured before the deadline or the
// cancellation of ctx get -1.
func Rank(ctx context.Context, targets []Target) []Result {
	ctx, cancel := context.WithTimeout(ctx, deadline)
	defer cancel()
	out := make([]Result, len(targets))
	for i, t := range targets {
		out[i] = Result{ID: t.ID, Ms: -1}
	}
	sem := make(chan struct{}, workers)
	var wg sync.WaitGroup
	for i := range targets {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				return
			}
			defer func() { <-sem }()
			out[i].Ms = measure(ctx, targets[i])
		}(i)
	}
	wg.Wait()
	return out
}

// measure returns the shortest connect time of a target in whole milliseconds,
// or -1 when no attempt connected.
func measure(ctx context.Context, t Target) int {
	best := time.Duration(-1)
	for _, port := range t.Ports {
		addr := net.JoinHostPort(t.IP, strconv.Itoa(port))
		for i := range attempts {
			if i > 0 {
				select {
				case <-ctx.Done():
					return ms(best)
				case <-time.After(gap):
				}
			}
			d, ok := connect(ctx, addr)
			if ok && (best < 0 || d < best) {
				best = d
			}
			if ctx.Err() != nil {
				return ms(best)
			}
			if !ok {
				// Repeating only pays off on a server that answers; a dead one
				// would otherwise use up the shared deadline of the others.
				break
			}
		}
	}
	return ms(best)
}

// connect opens and closes one TCP connection and returns how long opening it took.
func connect(ctx context.Context, addr string) (time.Duration, bool) {
	d := net.Dialer{Timeout: dialTimeout}
	start := time.Now()
	c, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return 0, false
	}
	elapsed := time.Since(start)
	c.Close()
	return elapsed, true
}

// ms rounds a duration up to whole milliseconds, at least 1. A negative duration
// (nothing connected) is -1.
func ms(d time.Duration) int {
	if d < 0 {
		return -1
	}
	return max(int((d+time.Millisecond-1)/time.Millisecond), 1)
}
