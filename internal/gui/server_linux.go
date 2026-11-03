//go:build linux

package gui

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"reflect"
	"sort"
	"strings"
	"sync"
	"time"

	"vpn_specify/internal/launcher"
)

// Janela no Linux
//
// No Windows a página roda num WebView2 e as funções vs* são ligadas
// direto ao Go. No Linux a mesma página é servida por um servidor HTTP em
// 127.0.0.1 (porta aleatória, protegido por um token aleatório) e aberta
// como janela de app de um navegador Chromium (--app: sem abas nem barra
// de endereço) ou, sem nenhum, no navegador padrão. Um script injetado na
// página transforma cada vsX(...) num POST /rpc/vsX, e uma conexão SSE
// (/events) diz se a janela está aberta e permite mandá-la fechar.

type uiServer struct {
	token    string
	base     string // http://127.0.0.1:porta
	handlers map[string]reflect.Value

	mu        sync.Mutex
	clients   map[chan string]bool
	lastGone  time.Time // quando a última janela desconectou
	openedAt  time.Time // quando pedimos para abrir uma janela
	connected bool      // alguma janela conectou desde o último openedAt
	icons     map[string]bool
}

func newUIServer(handlers map[string]any) (*uiServer, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return nil, err
	}
	s := &uiServer{token: hex.EncodeToString(b), handlers: map[string]reflect.Value{},
		clients: map[chan string]bool{}, icons: map[string]bool{}}
	for name, f := range handlers {
		v := reflect.ValueOf(f)
		if v.Kind() != reflect.Func {
			return nil, fmt.Errorf("%s não é função", name)
		}
		s.handlers[name] = v
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	s.base = "http://" + ln.Addr().String()
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.page)
	mux.HandleFunc("/rpc/", s.rpc)
	mux.HandleFunc("/events", s.events)
	mux.HandleFunc("/icon", s.icon)
	srv := &http.Server{Handler: s.guard(mux), ReadHeaderTimeout: 5 * time.Second}
	go srv.Serve(ln)
	return s, nil
}

func (s *uiServer) pageURL() string { return s.base + "/?t=" + s.token }

// guard recusa nomes de host diferentes (DNS rebinding: uma página da
// internet apontando um domínio para 127.0.0.1) e pedidos sem o token.
func (s *uiServer) guard(next http.Handler) http.Handler {
	host := strings.TrimPrefix(s.base, "http://")
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Host != host {
			http.Error(w, "proibido", http.StatusForbidden)
			return
		}
		tok := r.Header.Get("X-Token")
		if tok == "" {
			tok = r.URL.Query().Get("t")
		}
		if subtle.ConstantTimeCompare([]byte(tok), []byte(s.token)) != 1 {
			http.Error(w, "proibido", http.StatusForbidden)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		next.ServeHTTP(w, r)
	})
}

func (s *uiServer) page(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	io.WriteString(w, pageHTML("linux", s.bridgeScript()))
}

func (s *uiServer) bridgeScript() string {
	var names []string
	for n := range s.handlers {
		names = append(names, n)
	}
	sort.Strings(names)
	j, _ := json.Marshal(names)
	return `<script>
(() => {
  const token = new URLSearchParams(location.search).get("t") || "";
  for (const name of ` + string(j) + `) {
    window[name] = async (...args) => {
      let r;
      try {
        r = await fetch("/rpc/" + name, { method: "POST", headers: { "X-Token": token, "Content-Type": "application/json" }, body: JSON.stringify(args) });
      } catch (e) { throw "o vpn_specify não está respondendo (foi fechado?)"; }
      const j = await r.json().catch(() => ({ error: "resposta inválida (HTTP " + r.status + ")" }));
      if (j.error !== undefined) throw j.error;
      return j.result;
    };
  }
  window.vsIconURL = p => "/icon?t=" + encodeURIComponent(token) + "&p=" + encodeURIComponent(p);
  let failures = 0;
  const es = new EventSource("/events?t=" + encodeURIComponent(token));
  es.addEventListener("close", () => window.close());
  es.addEventListener("focus", () => window.focus());
  es.onopen = () => { failures = 0; document.body.classList.remove("gone"); };
  es.onerror = () => { if (++failures >= 3) { es.close(); document.body.classList.add("gone"); } };
})();
</script>`
}

func (s *uiServer) rpc(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "use POST", http.StatusMethodNotAllowed)
		return
	}
	name := strings.TrimPrefix(r.URL.Path, "/rpc/")
	body, err := io.ReadAll(io.LimitReader(r.Body, 8<<20))
	var res any
	if err == nil {
		res, err = s.call(name, body)
	}
	w.Header().Set("Content-Type", "application/json")
	if err != nil {
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	if err := json.NewEncoder(w).Encode(map[string]any{"result": res}); err != nil {
		log.Printf("rpc %s: %v", name, err)
	}
}

var errorType = reflect.TypeOf((*error)(nil)).Elem()

// call decodifica os argumentos JSON nos tipos da função e devolve o
// resultado, como o Bind do WebView2: func(...) [T] [error].
func (s *uiServer) call(name string, body []byte) (res any, err error) {
	f, ok := s.handlers[name]
	if !ok {
		return nil, fmt.Errorf("função desconhecida: %s", name)
	}
	t := f.Type()
	var raw []json.RawMessage
	if len(body) > 0 {
		if err := json.Unmarshal(body, &raw); err != nil {
			return nil, fmt.Errorf("%s: argumentos inválidos: %w", name, err)
		}
	}
	if len(raw) != t.NumIn() {
		return nil, fmt.Errorf("%s: esperava %d argumentos, veio %d", name, t.NumIn(), len(raw))
	}
	in := make([]reflect.Value, t.NumIn())
	for i := range in {
		v := reflect.New(t.In(i))
		if err := json.Unmarshal(raw[i], v.Interface()); err != nil {
			return nil, fmt.Errorf("%s: argumento %d inválido: %w", name, i+1, err)
		}
		in[i] = v.Elem()
	}
	defer func() {
		if p := recover(); p != nil {
			log.Printf("erro interno em %s: %v", name, p)
			err = fmt.Errorf("erro interno em %s", name)
		}
	}()
	out := f.Call(in)
	if n := len(out); n > 0 && t.Out(n-1) == errorType {
		if e := out[n-1]; !e.IsNil() {
			return nil, e.Interface().(error)
		}
		out = out[:n-1]
	}
	if len(out) > 0 {
		return out[0].Interface(), nil
	}
	return nil, nil
}

func (s *uiServer) events(w http.ResponseWriter, r *http.Request) {
	fl, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "sem streaming", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	ch := make(chan string, 4)
	s.mu.Lock()
	s.clients[ch] = true
	s.connected = true
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.clients, ch)
		if len(s.clients) == 0 {
			s.lastGone = time.Now()
		}
		s.mu.Unlock()
	}()
	io.WriteString(w, ": ok\n\n")
	fl.Flush()
	ping := time.NewTicker(20 * time.Second)
	defer ping.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case ev := <-ch:
			fmt.Fprintf(w, "event: %s\ndata: {}\n\n", ev)
			fl.Flush()
		case <-ping.C:
			io.WriteString(w, ": ping\n\n")
			fl.Flush()
		}
	}
}

// allowIcon libera um arquivo de ícone para a página (só os que o próprio
// vpn_specify encontrou, nunca um caminho qualquer).
func (s *uiServer) allowIcon(p string) {
	if p == "" {
		return
	}
	s.mu.Lock()
	s.icons[p] = true
	s.mu.Unlock()
}

func (s *uiServer) icon(w http.ResponseWriter, r *http.Request) {
	p := r.URL.Query().Get("p")
	s.mu.Lock()
	ok := s.icons[p]
	s.mu.Unlock()
	if !ok {
		http.NotFound(w, r)
		return
	}
	if strings.HasSuffix(p, ".svg") {
		w.Header().Set("Content-Type", "image/svg+xml")
		w.Header().Set("Content-Security-Policy", "script-src 'none'")
	}
	w.Header().Set("Cache-Control", "max-age=3600")
	http.ServeFile(w, r, p)
}

// isOpen: há uma janela conectada, uma acabou de recarregar, ou pedimos
// para abrir uma e ela ainda está carregando.
func (s *uiServer) isOpen() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.clients) > 0 || time.Since(s.lastGone) < 5*time.Second {
		return true
	}
	return !s.connected && !s.openedAt.IsZero() && time.Since(s.openedAt) < 45*time.Second
}

func (s *uiServer) broadcast(ev string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for ch := range s.clients {
		select {
		case ch <- ev:
		default:
		}
	}
}

// appBrowsers: navegadores que abrem uma página como janela de app.
var appBrowsers = []string{
	"google-chrome-stable", "google-chrome", "chromium", "chromium-browser",
	"microsoft-edge-stable", "microsoft-edge", "brave-browser", "vivaldi-stable",
}

// openWindow abre a janela. Se já houver uma, ela é fechada e outra é
// aberta: é o jeito confiável de trazê-la para a frente.
func (s *uiServer) openWindow() error {
	s.mu.Lock()
	hadClients := len(s.clients) > 0
	s.openedAt, s.connected = time.Now(), false
	s.mu.Unlock()
	if hadClients {
		s.broadcast("close")
	}
	url := s.pageURL()
	for _, name := range appBrowsers {
		p, err := exec.LookPath(name)
		if err != nil {
			continue
		}
		dir, err := launcher.BrowserProfileDir("ui", p)
		if err != nil {
			continue
		}
		_, err = startDetached(p, []string{
			"--app=" + url, "--user-data-dir=" + dir, "--class=vpn_specify",
			"--no-first-run", "--no-default-browser-check", "--password-store=basic",
			"--window-size=1000,760", "--disable-features=Translate",
		}, "", nil)
		if err == nil {
			log.Printf("janela aberta com %s", name)
			return nil
		}
	}
	if err := openWithDefault(url); err == nil {
		log.Printf("janela aberta no navegador padrão (instale o Chromium ou o Chrome para ter uma janela própria)")
		return nil
	}
	return errors.New("não achei um navegador para mostrar a janela; abra este endereço: " + url)
}

func hasDisplay() bool { return os.Getenv("DISPLAY") != "" || os.Getenv("WAYLAND_DISPLAY") != "" }
