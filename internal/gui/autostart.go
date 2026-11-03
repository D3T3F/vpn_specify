package gui

import (
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"vpn_specify/internal/autostart"
	"vpn_specify/internal/store"
)

// Apps que abrem com o sistema: cada app tem a opção "Abrir junto com o
// sistema"; o sistema tem uma entrada só (internal/autostart) que roda
// "vpn_specify autostart" ao entrar na conta, e o agente abre todos os apps
// marcados (que usam a mesma VPN).

// syncAutostart cria a entrada do sistema se há apps marcados (sempre
// apontando para este executável, que pode ter mudado de pasta) e a remove
// se não há nenhum.
func syncAutostart(st *store.Store) error {
	if len(st.AutostartApps()) == 0 {
		return autostart.Disable()
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	return autostart.Enable(exe)
}

// syncAutostartAfterSave: depois de salvar/remover um app; o erro vai para a
// tela, mas o app já foi salvo.
func syncAutostartAfterSave(st *store.Store) error {
	if err := syncAutostart(st); err != nil {
		log.Printf("inicialização com o sistema: %v", err)
		return fmt.Errorf("o app foi salvo, mas não consegui configurar a inicialização com o sistema: %w", err)
	}
	return nil
}

// runAutostart abre os apps marcados, em ordem. Logo depois do logon a rede
// pode ainda não estar pronta: cada app é tentado até 4 vezes, com pausas.
// report recebe os erros que sobrarem.
func runAutostart(st *store.Store, launch func(store.AppEntry) error, report func(string)) {
	apps := st.AutostartApps()
	if len(apps) == 0 {
		log.Printf("nenhum app marcado para abrir com o sistema")
		return
	}
	log.Printf("abrindo com o sistema: %d app(s)", len(apps))
	var failed []string
	for _, app := range apps {
		var err error
		for attempt := 1; attempt <= 4; attempt++ {
			if err = launch(app); err == nil {
				break
			}
			log.Printf("abrir %s com o sistema (tentativa %d): %v", app.Name, attempt, err)
			if !retryable(err) {
				break
			}
			if attempt < 4 {
				time.Sleep(time.Duration(attempt) * 10 * time.Second)
			}
		}
		if err != nil {
			failed = append(failed, fmt.Sprintf("%s: %v", app.Name, err))
		}
	}
	if len(failed) > 0 {
		report("Não consegui abrir na VPN ao iniciar o sistema:\n\n" + strings.Join(failed, "\n"))
	}
}

// retryable: falhas que tentar de novo não resolve (senha de administrador
// recusada, credenciais faltando, VPN ou programa inexistente) não repetem,
// para não pedir a senha várias vezes seguidas.
func retryable(err error) bool {
	msg := strings.ToLower(err.Error())
	for _, s := range []string{"permissão", "usuário e senha", "não existe", "not found", "no such file", "está ativa"} {
		if strings.Contains(msg, s) {
			return false
		}
	}
	return true
}
