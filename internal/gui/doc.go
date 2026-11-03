// Package gui implementa a interface gráfica e o "agente": a instância que
// mantém a VPN, abre os apps e se encerra sozinha quando eles fecham. A
// página fica em web/index.html e é a mesma nos dois sistemas:
//   - Windows: WebView2; o agente roda elevado (UAC) e as funções vs* são
//     ligadas direto ao Go (agent_windows.go, window_windows.go);
//   - Linux: a página é servida em 127.0.0.1 e aberta como janela de app de
//     um navegador Chromium; as funções vs* viram chamadas HTTP. O agente
//     roda como usuário comum e só o OpenVPN pede senha (pkexec)
//     (agent_linux.go, server_linux.go, api_linux.go, desktop_linux.go).
package gui
