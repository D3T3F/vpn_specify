// Package gui implements the graphical interface and the "agent": the
// instance that keeps the VPN, opens the apps and exits by itself when they
// close. The page lives in web/index.html and is the same on both systems:
//   - Windows: WebView2; the agent runs elevated (UAC) and the vs* functions
//     are bound straight to Go (agent_windows.go, window_windows.go);
//   - Linux: the page is served on 127.0.0.1 and opened as an app window of a
//     Chromium browser; the vs* functions become HTTP calls. The agent runs as
//     a normal user and only OpenVPN asks for a password (pkexec)
//     (agent_linux.go, server_linux.go, api_linux.go, desktop_linux.go).
//
// Texts are translated with internal/i18n (data-i18n in the page, t() in its
// JavaScript, i18n.T in Go).
package gui
