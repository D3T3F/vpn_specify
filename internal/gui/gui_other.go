//go:build !windows

package gui

import "errors"

var errNoGUI = errors.New("a interface gráfica só existe no Windows")

func Main() error                   { return errNoGUI }
func LaunchMain(id string) error    { return errNoGUI }
func AgentMain(args []string) error { return errNoGUI }
