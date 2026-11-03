//go:build !windows && !linux

package gui

import "errors"

var errNoGUI = errors.New("a interface gráfica só existe no Windows; aqui use os comandos de terminal (vpn_specify help)")

func Main() error                   { return errNoGUI }
func LaunchMain(id string) error    { return errNoGUI }
func AgentMain(args []string) error { return errNoGUI }
func AutostartMain() error          { return errNoGUI }
