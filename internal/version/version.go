// Package version guarda a versão do programa, definida na compilação:
//
//	go build -ldflags "-X vpn_specify/internal/version.Version=1.2.3"
package version

// Version é "dev" em builds sem versão definida.
var Version = "dev"
