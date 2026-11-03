//go:build !installer

package main

import (
	"errors"
	"io/fs"
)

// Without the "installer" tag nothing is embedded (so go build/vet ./...
// work without preparing the payload\ folder).
func payloadFS() (fs.FS, error) {
	return nil, errors.New(`installer built without the program files; use scripts\build.ps1 -Dist`)
}
