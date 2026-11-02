//go:build !installer

package main

import (
	"errors"
	"io/fs"
)

// Sem a tag "installer" não há arquivos embutidos (assim go build/vet ./...
// funciona sem preparar a pasta payload\).
func payloadFS() (fs.FS, error) {
	return nil, errors.New(`instalador compilado sem os arquivos do programa; use scripts\build.ps1 -Dist`)
}
