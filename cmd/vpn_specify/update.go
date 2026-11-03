package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"vpn_specify/internal/i18n"
	"vpn_specify/internal/update"
	"vpn_specify/internal/version"
)

// cmdUpdate: "vpn_specify update" looks for a new version in the GitHub
// releases and, if the user agrees, downloads, verifies and installs it.
func cmdUpdate(args []string) error {
	fs := flag.NewFlagSet("update", flag.ExitOnError)
	yes := fs.Bool("y", false, "install without asking")
	checkOnly := fs.Bool("check", false, "only check whether there is a new version")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: vpn_specify update [-check] [-y]")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	if !update.IsRelease(version.Version) {
		return i18n.Error("this is a development build ({0}); only published versions update", version.Version)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	rel, err := update.Check(ctx)
	cancel()
	if err != nil {
		return err
	}
	if rel == nil {
		fmt.Println(i18n.T("You already have the latest version ({0}).", version.Version))
		return nil
	}
	fmt.Println(i18n.T("New version: {0} (current: {1})", rel.Version, version.Version))
	fmt.Println(rel.URL)
	if notes := strings.TrimSpace(rel.Notes); notes != "" {
		fmt.Printf("\n%s\n\n", notes)
	}
	if *checkOnly {
		return nil
	}
	if !*yes {
		fmt.Print(i18n.T("Update now? [y/N] "))
		line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
		if a := strings.ToLower(strings.TrimSpace(line)); !yesAnswer(a) {
			return i18n.Error("update canceled")
		}
	}
	dctx, dcancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer dcancel()
	path, err := update.Download(dctx, rel.Asset, func(done, total int64) {
		if total > 0 {
			fmt.Fprintf(os.Stderr, "\r%s", i18n.T("downloading {0}: {1}%", rel.Asset.Name, fmt.Sprintf("%3.0f", 100*float64(done)/float64(total))))
		}
	})
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return err
	}
	fmt.Println(i18n.T("package verified (SHA-256)"))
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

// yesAnswer accepts "y"/"yes" and the word for yes in the active language
// (e.g. "s"/"sim" in Portuguese, "да" in Russian).
func yesAnswer(a string) bool {
	if a == "y" || a == "yes" {
		return true
	}
	for _, w := range strings.Split(i18n.T("y,yes"), ",") {
		if strings.EqualFold(strings.TrimSpace(w), a) {
			return true
		}
	}
	return false
}
