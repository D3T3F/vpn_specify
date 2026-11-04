package tunnel

import (
	"bufio"
	"log"
	"net/netip"
	"strconv"
	"strings"
	"time"

	"golang.zx2c4.com/wireguard/conn"
	"golang.zx2c4.com/wireguard/device"
	"golang.zx2c4.com/wireguard/tun"

	"vpn_specify/internal/i18n"
)

// WireGuard on a real tun interface, for systems where the operating system
// creates the interface and decides which apps use it (Android's VpnService).
// The desktop versions use the netstack in wireguard.go instead.

// DevicePlan is what the system needs to know to create the tun interface.
type DevicePlan struct {
	Addresses []string `json:"addresses"` // "10.0.0.2/32"
	DNS       []string `json:"dns"`
	MTU       int      `json:"mtu"`
	Routes    []string `json:"routes"` // "0.0.0.0/0", with the host bits zeroed
}

// PlanWireGuard validates a configuration and returns its DevicePlan.
func PlanWireGuard(data []byte) (*DevicePlan, error) {
	cfg, err := ParseWireGuardConfig(data)
	if err != nil {
		return nil, i18n.Wrap(err, "WireGuard configuration")
	}
	plan := &DevicePlan{MTU: cfg.mtu}
	for _, a := range cfg.addrs {
		plan.Addresses = append(plan.Addresses, netip.PrefixFrom(a, a.BitLen()).String())
	}
	for _, a := range cfg.dns {
		plan.DNS = append(plan.DNS, a.String())
	}
	if len(plan.DNS) == 0 {
		plan.DNS = []string{"1.1.1.1"} // the same default as the desktop versions
	}
	seen := map[string]bool{}
	for _, p := range cfg.peers {
		for _, s := range p.allowedIPs {
			r := netip.MustParsePrefix(s).Masked().String()
			if !seen[r] {
				seen[r] = true
				plan.Routes = append(plan.Routes, r)
			}
		}
	}
	if len(plan.Routes) == 0 {
		return nil, i18n.Error("no AllowedIPs in the configuration")
	}
	return plan, nil
}

// ResolveWireGuard reports whether the server names can be resolved right now
// (they cannot while the device has no network yet).
func ResolveWireGuard(data []byte) error {
	cfg, err := ParseWireGuardConfig(data)
	if err != nil {
		return i18n.Wrap(err, "WireGuard configuration")
	}
	for _, p := range cfg.peers {
		if p.endpoint != "" {
			if _, err := resolveEndpoint(p.endpoint); err != nil {
				return err
			}
		}
	}
	return nil
}

// WireGuardDevice is a running WireGuard on a tun device.
type WireGuardDevice struct {
	dev   *device.Device
	peers []device.NoisePublicKey
}

// DeviceStats: totals of all the peers. LastHandshake is in Unix seconds, 0 if
// there was none yet.
type DeviceStats struct {
	RxBytes       int64 `json:"rxBytes"`
	TxBytes       int64 `json:"txBytes"`
	LastHandshake int64 `json:"lastHandshake"`
}

// StartWireGuardDevice runs the configuration on tdev. On failure tdev is
// closed; on success Close closes it.
func StartWireGuardDevice(data []byte, tdev tun.Device) (*WireGuardDevice, error) {
	cfg, err := ParseWireGuardConfig(data)
	if err != nil {
		tdev.Close()
		return nil, i18n.Wrap(err, "WireGuard configuration")
	}
	d := &WireGuardDevice{}
	for _, p := range cfg.peers {
		if p.endpoint != "" {
			if p.endpoint, err = resolveEndpoint(p.endpoint); err != nil {
				tdev.Close()
				return nil, err
			}
		}
		var key device.NoisePublicKey
		if err := key.FromHex(p.publicKey); err != nil {
			tdev.Close()
			return nil, err
		}
		d.peers = append(d.peers, key)
	}
	logf := func(format string, args ...any) { log.Printf("[wireguard] "+format, args...) }
	d.dev = device.NewDevice(tdev, conn.NewDefaultBind(), &device.Logger{Verbosef: logf, Errorf: logf})
	if err := d.dev.IpcSet(cfg.ipc()); err != nil {
		d.dev.Close()
		return nil, i18n.Wrap(err, "applying the WireGuard configuration")
	}
	if err := d.dev.Up(); err != nil {
		d.dev.Close()
		return nil, err
	}
	d.Kick()
	log.Printf("WireGuard: address %v, DNS %v", cfg.addrs, cfg.dns)
	return d, nil
}

// Kick starts a handshake now, so the state is known before any app sends
// traffic.
func (d *WireGuardDevice) Kick() {
	for _, key := range d.peers {
		if p := d.dev.LookupPeer(key); p != nil {
			p.SendKeepalive()
		}
	}
}

// Rebind recreates the UDP sockets: call it when the network changes (Wi-Fi to
// mobile data, for example), or the old sockets keep using the lost network.
func (d *WireGuardDevice) Rebind() error {
	var err error
	for range 10 { // the network may be in the middle of changing
		if err = d.dev.BindUpdate(); err == nil {
			d.Kick()
			return nil
		}
		log.Printf("WireGuard: could not recreate the sockets: %v", err)
		time.Sleep(500 * time.Millisecond)
	}
	return err
}

// Stats reads the counters of all the peers.
func (d *WireGuardDevice) Stats() (DeviceStats, error) {
	var st DeviceStats
	s, err := d.dev.IpcGet()
	if err != nil {
		return st, err
	}
	sc := bufio.NewScanner(strings.NewReader(s))
	for sc.Scan() {
		k, v, _ := strings.Cut(sc.Text(), "=")
		n, _ := strconv.ParseInt(v, 10, 64)
		switch k {
		case "rx_bytes":
			st.RxBytes += n
		case "tx_bytes":
			st.TxBytes += n
		case "last_handshake_time_sec":
			st.LastHandshake = max(st.LastHandshake, n)
		}
	}
	return st, nil
}

// Close stops WireGuard and closes the tun device.
func (d *WireGuardDevice) Close() { d.dev.Close() }
