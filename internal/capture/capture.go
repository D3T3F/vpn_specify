package capture

// Controller controla a captura por processo (WinDivert, só no Windows).
type Controller interface {
	AddNames(names []string)
	Names() []string
	Stop()
}
