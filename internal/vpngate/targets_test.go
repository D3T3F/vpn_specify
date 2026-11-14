package vpngate

import (
	"reflect"
	"testing"

	"vpn_specify/internal/latency"
)

func TestTargets(t *testing.T) {
	l := &List{Servers: []Server{
		{ID: "a", IP: "10.0.0.1", Proto: "tcp", Port: 995},
		{ID: "b", IP: "10.0.0.2", Proto: "udp", Port: 1194},
		{ID: "c", IP: "10.0.0.3", Proto: "tcp", Port: 80},
	}}
	got := l.Targets(2)
	want := []latency.Target{
		{ID: "a", IP: "10.0.0.1", Ports: []int{995}},
		{ID: "b", IP: "10.0.0.2", Ports: []int{443}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
	if got := (*List)(nil).Targets(16); len(got) != 0 {
		t.Fatalf("nil list gave %+v", got)
	}
}
