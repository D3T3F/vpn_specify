// Package autostart registra o vpn_specify para rodar ao entrar na conta,
// com "vpn_specify autostart", que abre na VPN os apps marcados para
// iniciar com o sistema. Há uma entrada só, criada quando algum app é
// marcado e removida quando nenhum está:
//   - Windows: tarefa agendada "vpn_specify\Iniciar apps na VPN" no logon do
//     usuário, com privilégio máximo (o vpn_specify precisa de Administrador;
//     a tarefa evita o UAC a cada logon);
//   - Linux: ~/.config/autostart/vpn_specify.desktop (padrão XDG, seguido
//     por GNOME, KDE, XFCE, Cinnamon, MATE...).
package autostart

// Arg é o argumento que a entrada passa para o vpn_specify.
const Arg = "autostart"
