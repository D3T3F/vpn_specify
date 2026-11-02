//go:build windows

package platform

import (
	"golang.org/x/sys/windows"
)

var (
	user32                       = windows.NewLazySystemDLL("user32.dll")
	procGetShellWindow           = user32.NewProc("GetShellWindow")
	procMessageBoxW              = user32.NewProc("MessageBoxW")
	procShowWindow               = user32.NewProc("ShowWindow")
	procSetForegroundWindow      = user32.NewProc("SetForegroundWindow")
	procAllowSetForegroundWindow = user32.NewProc("AllowSetForegroundWindow")
	procLoadImageW               = user32.NewProc("LoadImageW")
	procGetSystemMetrics         = user32.NewProc("GetSystemMetrics")
	procSendMessageW             = user32.NewProc("SendMessageW")
	comdlg32                     = windows.NewLazySystemDLL("comdlg32.dll")
	procGetOpenFileNameW         = comdlg32.NewProc("GetOpenFileNameW")
	advapi32                     = windows.NewLazySystemDLL("advapi32.dll")
	procCreateProcessWithTokenW  = advapi32.NewProc("CreateProcessWithTokenW")
)
