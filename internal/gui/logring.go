package gui

import (
	_ "embed"
	"strings"
	"sync"
)

// ---------- log em memória (para a tela) ----------

type logRing struct {
	mu    sync.Mutex
	lines []string
	base  int // índice global da primeira linha guardada
}

func (r *logRing) Write(b []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, l := range strings.Split(strings.TrimRight(string(b), "\n"), "\n") {
		r.lines = append(r.lines, strings.TrimRight(l, "\r"))
	}
	if over := len(r.lines) - 1000; over > 0 {
		r.lines = r.lines[over:]
		r.base += over
	}
	return len(b), nil
}

func (r *logRing) since(i int) ([]string, int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if i < r.base {
		i = r.base
	}
	out := append([]string(nil), r.lines[i-r.base:]...)
	return out, r.base + len(r.lines)
}
