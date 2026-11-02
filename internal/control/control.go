package control

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"vpn_specify/internal/paths"
)

// Canal de controle da instância em execução (a que tem a VPN e roda como
// Administrador). Atalhos e novas janelas, que rodam sem elevação, só pedem
// "mostre a janela" ou "abra o app X". Assim o UAC aparece só quando a VPN
// ainda não está ligada.

const controlAddr = "127.0.0.1:10799"

type controlInfo struct {
	Token string `json:"token"`
}

func controlFile() string { return filepath.Join(paths.DataDir(), "control.json") }

// Start abre a porta de controle; falha se já existe outra instância.
func Start(handlers map[string]func(q url.Values) error) (func(), error) {
	ln, err := net.Listen("tcp", controlAddr)
	if err != nil {
		return nil, err
	}
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	token := hex.EncodeToString(b)
	j, _ := json.Marshal(controlInfo{Token: token})
	if err := os.WriteFile(controlFile(), j, 0o600); err != nil {
		ln.Close()
		return nil, err
	}
	mux := http.NewServeMux()
	for path, h := range handlers {
		mux.HandleFunc("/"+path, func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost || subtle.ConstantTimeCompare([]byte(r.Header.Get("X-Token")), []byte(token)) != 1 {
				http.Error(w, "proibido", http.StatusForbidden)
				return
			}
			if err := h(r.URL.Query()); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
			}
		})
	}
	srv := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go srv.Serve(ln)
	return func() { srv.Close(); os.Remove(controlFile()) }, nil
}

// Call envia um comando para a instância em execução.
// Devolve errNoInstance se não houver nenhuma.
var ErrNoInstance = errors.New("nenhuma instância em execução")

func Call(path string, q url.Values) error {
	b, err := os.ReadFile(controlFile())
	if err != nil {
		return ErrNoInstance
	}
	var ci controlInfo
	if json.Unmarshal(b, &ci) != nil {
		return ErrNoInstance
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	u := "http://" + controlAddr + "/" + path
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, u, nil)
	req.Header.Set("X-Token", ci.Token)
	resp, err := (&http.Client{Transport: &http.Transport{Proxy: nil}}).Do(req)
	if err != nil {
		return ErrNoInstance
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusForbidden {
		return ErrNoInstance
	}
	if resp.StatusCode != http.StatusOK {
		var msg [512]byte
		n, _ := resp.Body.Read(msg[:])
		return fmt.Errorf("%s", string(msg[:n]))
	}
	return nil
}
