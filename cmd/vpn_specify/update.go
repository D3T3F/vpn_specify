package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"vpn_specify/internal/update"
	"vpn_specify/internal/version"
)

// cmdUpdate: "vpn_specify update" procura uma versão nova nas releases do
// GitHub e, se o usuário aceitar, baixa, confere e instala.
func cmdUpdate(args []string) error {
	fs := flag.NewFlagSet("update", flag.ExitOnError)
	yes := fs.Bool("y", false, "instala sem perguntar")
	checkOnly := fs.Bool("check", false, "só verifica se há versão nova")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "Uso: vpn_specify update [-check] [-y]")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	if !update.IsRelease(version.Version) {
		return fmt.Errorf("esta é uma versão de desenvolvimento (%s); só as versões publicadas se atualizam", version.Version)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	rel, err := update.Check(ctx)
	cancel()
	if err != nil {
		return err
	}
	if rel == nil {
		fmt.Printf("Você já está na versão mais recente (%s).\n", version.Version)
		return nil
	}
	fmt.Printf("Nova versão: %s (atual: %s)\n%s\n", rel.Version, version.Version, rel.URL)
	if notes := strings.TrimSpace(rel.Notes); notes != "" {
		fmt.Printf("\n%s\n\n", notes)
	}
	if *checkOnly {
		return nil
	}
	if !*yes {
		fmt.Print("Atualizar agora? [s/N] ")
		line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
		if a := strings.ToLower(strings.TrimSpace(line)); a != "s" && a != "sim" && a != "y" && a != "yes" {
			return errors.New("atualização cancelada")
		}
	}
	dctx, dcancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer dcancel()
	path, err := update.Download(dctx, rel.Asset, func(done, total int64) {
		if total > 0 {
			fmt.Fprintf(os.Stderr, "\rbaixando %s: %3.0f%%", rel.Asset.Name, 100*float64(done)/float64(total))
		}
	})
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return err
	}
	fmt.Println("pacote conferido (SHA-256)")
	plan, err := update.Prepare(path, rel.Version, false)
	if err != nil {
		return err
	}
	if err := plan.Run(); err != nil {
		return err
	}
	fmt.Printf("vpn_specify %s: %s\n", rel.Version, plan.Description)
	return nil
}
