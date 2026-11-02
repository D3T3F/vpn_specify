// Package tunnel abre a conexão VPN dentro do processo, sem mudar as rotas
// nem o DNS do Windows.
//
//   - WireGuard: wireguard-go com pilha TCP/IP em espaço de usuário (gVisor);
//     não cria adaptador e não precisa de Administrador.
//   - OpenVPN: usa o openvpn.exe instalado, filtrando tudo que alteraria o
//     roteamento; o tráfego sai pela VPN porque os sockets são ligados ao IP
//     do túnel.
//
// Os dois expõem a mesma interface [Tunnel] (TCP, UDP e DNS por dentro da VPN).
package tunnel
