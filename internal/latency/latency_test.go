package latency

import (
	"context"
	"net"
	"strconv"
	"testing"
	"time"
)

// listen opens a local TCP port that accepts connections, and returns it.
func listen(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			c.Close()
		}
	}()
	return l.Addr().(*net.TCPAddr).Port
}

// closedPort returns a local port with nothing listening on it.
func closedPort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()
	return port
}

func TestRankReachable(t *testing.T) {
	port := listen(t)
	res := Rank(context.Background(), []Target{{ID: "a", IP: "127.0.0.1", Ports: []int{port}}})
	if len(res) != 1 || res[0].ID != "a" || res[0].Ms < 1 {
		t.Fatalf("got %+v, want a measured latency for a", res)
	}
}

func TestRankClosedPort(t *testing.T) {
	res := Rank(context.Background(), []Target{{ID: "a", IP: "127.0.0.1", Ports: []int{closedPort(t)}}})
	if len(res) != 1 || res[0].Ms != -1 {
		t.Fatalf("got %+v, want -1", res)
	}
}

func TestRankNoPorts(t *testing.T) {
	res := Rank(context.Background(), []Target{{ID: "a", IP: "127.0.0.1"}})
	if len(res) != 1 || res[0].Ms != -1 {
		t.Fatalf("got %+v, want -1", res)
	}
}

func TestRankKeepsInputOrder(t *testing.T) {
	open := listen(t)
	closed := closedPort(t)
	targets := []Target{
		{ID: "closed-1", IP: "127.0.0.1", Ports: []int{closed}},
		{ID: "open", IP: "127.0.0.1", Ports: []int{closed, open}},
		{ID: "closed-2", IP: "127.0.0.1", Ports: []int{closed}},
	}
	res := Rank(context.Background(), targets)
	want := []string{"closed-1", "open", "closed-2"}
	if len(res) != len(want) {
		t.Fatalf("got %d results, want %d", len(res), len(want))
	}
	for i, id := range want {
		if res[i].ID != id {
			t.Errorf("result %d is %q, want %q", i, res[i].ID, id)
		}
	}
	if res[0].Ms != -1 || res[1].Ms < 1 || res[2].Ms != -1 {
		t.Errorf("got %+v, want -1, measured, -1", res)
	}
}

func TestRankManyTargets(t *testing.T) {
	open := listen(t)
	var targets []Target
	for i := range 20 {
		targets = append(targets, Target{ID: strconv.Itoa(i), IP: "127.0.0.1", Ports: []int{open}})
	}
	res := Rank(context.Background(), targets)
	for i, r := range res {
		if r.ID != strconv.Itoa(i) || r.Ms < 1 {
			t.Fatalf("result %d is %+v", i, r)
		}
	}
}

func TestRankCancelReturnsPromptly(t *testing.T) {
	// 10.255.255.1 is normally unreachable and drops the SYN, so the connect would
	// wait for its full timeout.
	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(100*time.Millisecond, cancel)
	start := time.Now()
	res := Rank(ctx, []Target{{ID: "a", IP: "10.255.255.1", Ports: []int{9}}})
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Fatalf("Rank took %v after cancel", elapsed)
	}
	if len(res) != 1 || res[0].ID != "a" || res[0].Ms != -1 {
		t.Fatalf("got %+v, want -1", res)
	}
}

func TestMs(t *testing.T) {
	for in, want := range map[time.Duration]int{
		-1:                      -1,
		0:                       1,
		time.Microsecond:        1,
		time.Millisecond:        1,
		time.Millisecond + 1:    2,
		42*time.Millisecond - 1: 42,
		42 * time.Millisecond:   42,
	} {
		if got := ms(in); got != want {
			t.Errorf("ms(%v) = %d, want %d", in, got, want)
		}
	}
}
