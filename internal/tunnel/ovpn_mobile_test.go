package tunnel

import (
	"strings"
	"testing"
)

func TestPlanOpenVPNRemovesWhatRuns(t *testing.T) {
	in := "client\r\n" +
		"remote vpn.example 1194\n" +
		"redirect-gateway def1\n" + // honored here, unlike on the desktop
		"script-security 2\n" +
		"up /system/bin/sh\n" +
		"--down /bin/evil\n" +
		"UP /bin/evil-too\n" +
		"management 0.0.0.0 7505\n" +
		"log /sdcard/x\n" +
		"plugin evil.so\n" +
		"<ca>\nup this-is-certificate-text\n</ca>\n" +
		"# up a comment stays\n"
	p, err := PlanOpenVPN([]byte(in))
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(p.Config, "\n") {
		f := strings.Fields(strings.ToLower(line))
		if len(f) == 0 || f[0] == "#" {
			continue
		}
		switch strings.TrimPrefix(f[0], "--") {
		case "script-security", "down", "management", "log", "plugin":
			t.Errorf("not removed: %q", line)
		case "up":
			if line != "up this-is-certificate-text" {
				t.Errorf("not removed: %q", line)
			}
		}
	}
	for _, keep := range []string{"redirect-gateway def1", "remote vpn.example 1194", "up this-is-certificate-text"} {
		if !strings.Contains(p.Config, keep) {
			t.Errorf("removed too much: %q", keep)
		}
	}
	if p.NeedsAuth {
		t.Error("does not ask for a password")
	}
}

func TestPlanOpenVPNRefuses(t *testing.T) {
	const ok = "client\nremote vpn.example 1194\n"
	for name, conf := range map[string]string{
		"no remote":        "client\n",
		"tap":              ok + "dev tap\n",
		"tap type":         ok + "dev-type tap\n",
		"ca file":          ok + "ca ca.crt\n",
		"key file":         ok + "key /sdcard/client.key\n",
		"tls-auth file":    ok + "tls-auth ta.key 1\n",
		"credentials file": ok + "auth-user-pass /sdcard/pass.txt\n",
	} {
		if _, err := PlanOpenVPN([]byte(conf)); err == nil {
			t.Errorf("%s: should be refused", name)
		}
	}
}

func TestPlanOpenVPNAccepts(t *testing.T) {
	p, err := PlanOpenVPN([]byte("client\nremote v 1\nauth-user-pass\n<ca>\nx\n</ca>\ntls-auth [inline] 1\n"))
	if err != nil || !p.NeedsAuth {
		t.Fatalf("%+v %v", p, err)
	}
	p, err = PlanOpenVPN([]byte("client\nremote v 1\nauth-user-pass\n<auth-user-pass>\nu\np\n</auth-user-pass>\n"))
	if err != nil || p.NeedsAuth {
		t.Fatalf("inline credentials: %+v %v", p, err)
	}
	if _, err = PlanOpenVPN([]byte("client\n<connection>\nremote v 1\n</connection>\n")); err != nil {
		t.Fatalf("connection block: %v", err)
	}
}
