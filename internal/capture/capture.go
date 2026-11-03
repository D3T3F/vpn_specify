package capture

// Controller controls per-process capture (WinDivert, Windows only).
type Controller interface {
	AddNames(names []string)
	Names() []string
	Stop()
}
