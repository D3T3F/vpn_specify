// Package capture coloca TODO o tráfego de processos escolhidos (pelo nome
// do executável) dentro da VPN, inclusive UDP (voz), usando o driver
// WinDivert. TCP vai por um proxy transparente; UDP por sockets do túnel.
//
// Só funciona no Windows e precisa de Administrador.
package capture
