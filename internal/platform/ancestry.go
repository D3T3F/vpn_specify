package platform

// maxAncestorDepth bounds how many parent links CapturedAncestor follows.
const maxAncestorDepth = 16

// ProcessTable is a snapshot of the running processes: the parent PID and the
// lower-case executable name of each PID.
//
// Known limit: the snapshot has no process creation time, so a PID reused by
// a new process after its parent exited can look like a child of the old
// parent. The parent must be in the same snapshot, which narrows this window
// but does not close it.
type ProcessTable struct {
	Parent map[uint32]uint32
	Name   map[uint32]string
}

// CapturedAncestor reports whether pid belongs to a captured process: its own
// name is in names, or one of its ancestors (up to maxAncestorDepth links) is.
// root is the name of the captured process that matched, the nearest one.
// self is the PID of vpn_specify: it is never captured, and the search for a
// captured ancestor stops at it (apps started by vpn_specify are its children,
// so a child of such an app is still captured through the app). Pure function:
// it makes no OS calls.
func CapturedAncestor(pid uint32, t ProcessTable, names map[string]bool, self uint32) (root string, ok bool) {
	name := t.Name[pid]
	if name == "" || pid == self {
		return "", false
	}
	if names[name] {
		root, ok = name, true
	}
	visited := map[uint32]bool{pid: true}
	cur := pid
	for range maxAncestorDepth {
		parent, found := t.Parent[cur]
		if !found || parent == 0 || visited[parent] {
			break
		}
		parentName, known := t.Name[parent]
		if !known {
			break
		}
		if parent == self {
			// vpn_specify launches the captured apps, so everything above this point
			// is its own. Stop here, keeping a captured ancestor found below it; a
			// process with none (its tunnel helpers) stays uncaptured.
			break
		}
		if !ok && names[parentName] {
			root, ok = parentName, true
		}
		visited[parent] = true
		cur = parent
	}
	return root, ok
}
