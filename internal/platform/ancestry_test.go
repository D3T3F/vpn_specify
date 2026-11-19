package platform

import "testing"

var testCaptured = map[string]bool{"dorion.exe": true}

// testSelf is the PID of vpn_specify in the tests; no test table uses it
// unless it says so.
const testSelf = 1000

// tableFromNames builds a table where pid i+1 is named names[i] and is the
// child of pid i (pid 1 is the root).
func tableFromNames(names ...string) ProcessTable {
	t := ProcessTable{Parent: map[uint32]uint32{}, Name: map[uint32]string{}}
	for i, n := range names {
		pid := uint32(i + 1)
		t.Name[pid] = n
		if i > 0 {
			t.Parent[pid] = uint32(i)
		}
	}
	return t
}

func TestCapturedAncestor(t *testing.T) {
	tests := []struct {
		name     string
		table    ProcessTable
		pid      uint32
		wantRoot string
		wantOK   bool
		self     uint32
	}{
		{
			name:     "direct name match",
			table:    tableFromNames("dorion.exe"),
			pid:      1,
			wantRoot: "dorion.exe",
			wantOK:   true,
		},
		{
			name:     "child of captured app (depth 1)",
			table:    tableFromNames("dorion.exe", "msedgewebview2.exe"),
			pid:      2,
			wantRoot: "dorion.exe",
			wantOK:   true,
		},
		{
			name:     "grandchild of captured app (depth 2)",
			table:    tableFromNames("dorion.exe", "msedgewebview2.exe", "msedgewebview2.exe"),
			pid:      3,
			wantRoot: "dorion.exe",
			wantOK:   true,
		},
		{
			name:     "great-grandchild of captured app (depth 3)",
			table:    tableFromNames("dorion.exe", "msedgewebview2.exe", "msedgewebview2.exe", "chrome.exe"),
			pid:      4,
			wantRoot: "dorion.exe",
			wantOK:   true,
		},
		{
			name:   "same exe under an uncaptured parent",
			table:  tableFromNames("teams.exe", "msedgewebview2.exe"),
			pid:    2,
			wantOK: false,
		},
		{
			name:   "vpn_specify itself, captured by name",
			table:  tableFromNames("dorion.exe", "vpn_specify.exe"),
			pid:    2,
			self:   2,
			wantOK: false,
		},
		{
			name:   "descendant of vpn_specify under a captured app",
			table:  tableFromNames("dorion.exe", "vpn_specify.exe", "msedgewebview2.exe"),
			pid:    3,
			self:   2,
			wantOK: false,
		},
		{
			// The real case: vpn_specify launches the captured app, so every
			// child of the app also descends from vpn_specify.
			name:     "child of an app launched by vpn_specify",
			table:    tableFromNames("vpn_specify.exe", "dorion.exe", "msedgewebview2.exe", "msedgewebview2.exe"),
			pid:      4,
			self:     1,
			wantRoot: "dorion.exe",
			wantOK:   true,
		},
		{
			name:     "the app launched by vpn_specify, by name",
			table:    tableFromNames("vpn_specify.exe", "dorion.exe"),
			pid:      2,
			self:     1,
			wantRoot: "dorion.exe",
			wantOK:   true,
		},
		{
			name:   "tunnel helper started by vpn_specify",
			table:  tableFromNames("vpn_specify.exe", "netsh.exe"),
			pid:    2,
			self:   1,
			wantOK: false,
		},
		{
			name:   "unknown pid",
			table:  tableFromNames("dorion.exe"),
			pid:    99,
			wantOK: false,
		},
		{
			name: "parent missing from the table",
			table: ProcessTable{
				Parent: map[uint32]uint32{2: 7},
				Name:   map[uint32]string{2: "msedgewebview2.exe"},
			},
			pid:    2,
			wantOK: false,
		},
		{
			name: "parent is pid 0",
			table: ProcessTable{
				Parent: map[uint32]uint32{2: 0},
				Name:   map[uint32]string{2: "msedgewebview2.exe"},
			},
			pid:    2,
			wantOK: false,
		},
		{
			name: "cycle in parents",
			table: ProcessTable{
				Parent: map[uint32]uint32{2: 3, 3: 2},
				Name:   map[uint32]string{2: "msedgewebview2.exe", 3: "msedgewebview2.exe"},
			},
			pid:    2,
			wantOK: false,
		},
		{
			name: "self parent",
			table: ProcessTable{
				Parent: map[uint32]uint32{2: 2},
				Name:   map[uint32]string{2: "msedgewebview2.exe"},
			},
			pid:    2,
			wantOK: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			self := tt.self
			if self == 0 {
				self = testSelf
			}
			root, ok := CapturedAncestor(tt.pid, tt.table, testCaptured, self)
			if ok != tt.wantOK || root != tt.wantRoot {
				t.Fatalf("CapturedAncestor(%d) = (%q, %v), want (%q, %v)", tt.pid, root, ok, tt.wantRoot, tt.wantOK)
			}
		})
	}
}

func TestCapturedAncestorDepthLimit(t *testing.T) {
	// chainOf returns a table of dorion.exe followed by `links` processes, each
	// child of the previous one; the last pid is the one queried. Its captured
	// app is `links` links away.
	chainOf := func(links int) (ProcessTable, uint32) {
		names := []string{"dorion.exe"}
		for range links {
			names = append(names, "msedgewebview2.exe")
		}
		return tableFromNames(names...), uint32(len(names))
	}

	table, pid := chainOf(maxAncestorDepth)
	if root, ok := CapturedAncestor(pid, table, testCaptured, testSelf); !ok || root != "dorion.exe" {
		t.Fatalf("app %d links away: got (%q, %v), want captured", maxAncestorDepth, root, ok)
	}

	table, pid = chainOf(maxAncestorDepth + 1)
	if root, ok := CapturedAncestor(pid, table, testCaptured, testSelf); ok {
		t.Fatalf("app %d links away: got (%q, %v), want not captured", maxAncestorDepth+1, root, ok)
	}
}
