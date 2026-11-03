//go:build installer

package main

import (
	"embed"
	"io/fs"
)

// payload\ is prepared by scripts\build.ps1 -Dist (it is not in the source tree).
//
//go:embed all:payload
var payload embed.FS

func payloadFS() (fs.FS, error) { return fs.Sub(payload, "payload") }
