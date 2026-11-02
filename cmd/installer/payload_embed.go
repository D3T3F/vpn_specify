//go:build installer

package main

import (
	"embed"
	"io/fs"
)

// payload\ é preparada pelo scripts\build.ps1 -Dist (não fica no código-fonte).
//
//go:embed all:payload
var payload embed.FS

func payloadFS() (fs.FS, error) { return fs.Sub(payload, "payload") }
