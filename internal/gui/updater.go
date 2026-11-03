package gui

import (
	"context"
	"errors"
	"log"
	"strings"
	"sync"
	"time"

	"vpn_specify/internal/store"
	"vpn_specify/internal/update"
	"vpn_specify/internal/version"
)

// Atualizações: ao abrir, o agente procura uma versão nova nas releases do
// GitHub (internal/update). Se houver, a tela mostra um aviso e o usuário
// escolhe: atualizar agora, deixar para depois ou ignorar a versão. Ao
// atualizar, o pacote é baixado e conferido; o agente desconecta, fecha e
// aplica a atualização (Windows: instalador; Linux: troca o executável), e
// a versão nova abre sozinha.

type updateInfo struct {
	// "" (nada a mostrar) | checking | current | available | downloading | installing | error
	State     string  `json:"state"`
	Current   string  `json:"current"`
	Version   string  `json:"version"`
	Notes     string  `json:"notes"`
	Progress  float64 `json:"progress"` // 0..1 durante o download
	Error     string  `json:"error"`
	Dismissed bool    `json:"dismissed"` // "Agora não" ou versão ignorada
}

type updater struct {
	store   *store.Store
	apply   func(*update.Plan) // o agente fecha e chama plan.Run
	openURL func(string) error

	mu   sync.Mutex
	info updateInfo
	rel  *update.Release
}

func newUpdater(st *store.Store, apply func(*update.Plan), openURL func(string) error) *updater {
	return &updater{store: st, apply: apply, openURL: openURL, info: updateInfo{Current: version.Version}}
}

func (u *updater) snapshot() *updateInfo {
	u.mu.Lock()
	defer u.mu.Unlock()
	i := u.info
	return &i
}

func (u *updater) set(f func(*updateInfo)) {
	u.mu.Lock()
	f(&u.info)
	u.mu.Unlock()
}

// autoCheck: a busca feita ao abrir. Falhas só vão para o log.
func (u *updater) autoCheck() {
	update.CleanupOld()
	if u.store.Snapshot().NoUpdateCheck || !update.IsRelease(version.Version) {
		return
	}
	time.Sleep(2 * time.Second) // deixa a janela abrir primeiro
	rel, err := u.check()
	if err != nil {
		log.Printf("não consegui procurar atualizações: %v", err)
		u.set(func(i *updateInfo) { i.State, i.Error = "", "" })
		return
	}
	if rel != nil {
		skipped := rel.Version == u.store.Snapshot().SkipVersion
		u.set(func(i *updateInfo) { i.Dismissed = skipped })
		if skipped {
			log.Printf("versão %s disponível (ignorada a pedido)", rel.Version)
		} else {
			log.Printf("versão %s disponível (atual: %s)", rel.Version, version.Version)
		}
	}
}

var errBusy = errors.New("uma atualização já está em andamento")

func (u *updater) check() (*update.Release, error) {
	u.mu.Lock()
	if s := u.info.State; s == "checking" || s == "downloading" || s == "installing" {
		u.mu.Unlock()
		return nil, errBusy
	}
	u.info.State, u.info.Error = "checking", ""
	u.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	rel, err := update.Check(ctx)
	u.mu.Lock()
	defer u.mu.Unlock()
	if err != nil {
		u.info.State, u.info.Error = "error", err.Error()
		return nil, err
	}
	u.rel = rel
	if rel == nil {
		u.info.State = "current"
		return nil, nil
	}
	u.info = updateInfo{State: "available", Current: version.Version, Version: rel.Version, Notes: rel.Notes}
	return rel, nil
}

// install baixa, prepara e entrega a atualização ao agente (em segundo plano;
// a tela acompanha pelo estado).
func (u *updater) install() error {
	u.mu.Lock()
	rel := u.rel
	if s := u.info.State; s == "downloading" || s == "installing" || s == "checking" {
		u.mu.Unlock()
		return errBusy
	}
	if rel == nil {
		u.mu.Unlock()
		return errors.New("nenhuma atualização disponível")
	}
	u.info.State, u.info.Error, u.info.Progress, u.info.Dismissed = "downloading", "", 0, false
	u.mu.Unlock()

	go func() {
		log.Printf("baixando a versão %s (%s)…", rel.Version, rel.Asset.Name)
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
		defer cancel()
		path, err := update.Download(ctx, rel.Asset, func(done, total int64) {
			if total > 0 {
				u.set(func(i *updateInfo) { i.Progress = float64(done) / float64(total) })
			}
		})
		var plan *update.Plan
		if err == nil {
			u.set(func(i *updateInfo) { i.State, i.Progress = "installing", 1 })
			plan, err = update.Prepare(path, rel.Version, true)
		}
		if err != nil {
			log.Printf("atualização falhou: %v", err)
			u.set(func(i *updateInfo) { i.State, i.Error = "error", err.Error() })
			return
		}
		log.Printf("atualização para %s pronta (%s); fechando para concluir", rel.Version, plan.Description)
		time.Sleep(800 * time.Millisecond) // a tela mostra "instalando…"
		u.apply(plan)
	}()
	return nil
}

func (u *updater) dismiss(skip bool) error {
	u.mu.Lock()
	rel := u.rel
	u.info.Dismissed = true
	if u.info.State == "error" && rel != nil {
		u.info.State, u.info.Error = "available", ""
	}
	u.mu.Unlock()
	if skip && rel != nil {
		log.Printf("versão %s ignorada", rel.Version)
		return u.store.SetSkipVersion(rel.Version)
	}
	return nil
}

// api: as funções da tela ligadas às atualizações.
func (u *updater) api() map[string]any {
	return map[string]any{
		// vsUpdateCheck: "Procurar agora"; devolve a versão nova ou "".
		"vsUpdateCheck": func() (string, error) {
			if !update.IsRelease(version.Version) {
				return "", errors.New("esta é uma versão de desenvolvimento (" + version.Version + "); só as versões publicadas se atualizam")
			}
			rel, err := u.check()
			if err != nil || rel == nil {
				return "", err
			}
			return rel.Version, nil
		},
		"vsUpdateInstall":  func() error { return u.install() },
		"vsUpdateDismiss":  func(skip bool) error { return u.dismiss(skip) },
		"vsSetUpdateCheck": func(v bool) error { return u.store.SetUpdateCheck(v) },
		"vsOpenReleasePage": func() error {
			u.mu.Lock()
			url := update.ReleasesPage
			if u.rel != nil && strings.HasPrefix(u.rel.URL, update.ReleasesPage+"/") { // só páginas deste repositório
				url = u.rel.URL
			}
			u.mu.Unlock()
			return u.openURL(url)
		},
	}
}
