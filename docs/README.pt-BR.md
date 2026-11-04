# vpn_specify

[English](../README.md) · **Português (Brasil)** · [Español](README.es.md) · [Français](README.fr.md) · [Deutsch](README.de.md) · [Русский](README.ru.md) · [简体中文](README.zh-CN.md) · [日本語](README.ja.md) · [हिन्दी](README.hi.md) · [العربية](README.ar.md)

Conecta numa VPN **só para os apps que você escolher**. O resto do computador
continua na sua rede normal.

```
            ┌──────────────── vpn_specify ────────────────┐
 chrome ──► │ proxy SOCKS5 127.0.0.1:10800 ┐               │
 curl   ──► │ proxy HTTP   127.0.0.1:10801 ┴──► túnel VPN ─┼──► rede da VPN
            └─────────────────────────────────────────────┘
 resto do PC ─────────────────────────────────────────────────► internet normal
```

Sem serviço rodando em segundo plano. No Windows há dois executáveis do mesmo
código:

- `vpn_specify.exe`: interface gráfica (sem console).
- `vpn_specify-cli.exe`: os comandos de terminal (`up`, `run`, `status`, `update`).

No Linux é um executável só, `vpn_specify`, com interface gráfica e terminal
(veja [Linux](#linux)).

A interface, o instalador e as mensagens estão disponíveis em **10 idiomas**
(veja [Idiomas](#idiomas)).

> Este é o guia de uso. Para compilar, publicar versões e conhecer a
> estrutura do código, veja o [README em inglês](../README.md#building).

## Instalação

Baixe o `vpn_specify-<versão>-win64.zip` nas
[releases](https://github.com/D3T3F/vpn_specify/releases), extraia e execute
`vpn_specify-setup.exe`:

- Escolha o idioma no topo da janela do instalador; o app passa a usá-lo
  (dá para trocar depois em **Preferências**).
- Pasta padrão: `C:\Program Files\vpn_specify` (pode escolher outra em **Procurar…**).
- Opções: atalho na Área de Trabalho e no Menu Iniciar.
- Nada é baixado da rede: o programa e o WinDivert já vão dentro do instalador.
- Rodar o instalador de novo atualiza a instalação existente (na mesma pasta).
- Desinstalar: Configurações > Aplicativos > Aplicativos instalados >
  vpn_specify. Você escolhe se apaga ou mantém os dados. Os atalhos criados
  pelo app (ex.: `DiscordVPN`) também são removidos.

Sem cliques (para scripts):

```powershell
vpn_specify-setup.exe /silent [/dir=D:\Apps] [/nodesktop] [/nostartmenu] [/lang=pt-BR]
"C:\Program Files\vpn_specify\vpn_specify.exe" uninstall /silent [/keepdata]
```

Também funciona sem instalar (modo portátil): basta ter a pasta `data\` (ou
um arquivo vazio chamado `portable`) ao lado do `vpn_specify.exe`.

## Interface gráfica

Abra `vpn_specify.exe`. O Windows pede permissão de Administrador.

1. **VPNs > Importar arquivo…**: o `.ovpn`/`.conf` é copiado para a pasta de
   dados. Se a VPN pede login, use **Credenciais**: a senha é guardada com
   DPAPI, então só a sua conta do Windows consegue ler. **Editar** permite
   renomear a VPN (os apps que a usam acompanham) e alterar o conteúdo do
   arquivo; é preciso desconectar essa VPN antes.
2. **Apps na VPN > Adicionar app…**: escolha o `.exe` ou um atalho do Menu
   Iniciar (ex.: Discord). Para apps com atualizador (`Update.exe --processStart
   Discord.exe`) o processo certo (`discord.exe`) é detectado sozinho.
   - *Captura completa* (padrão): todo o tráfego, incluindo voz/UDP (WinDivert).
   - *Proxy*: só TCP, para navegadores e apps Chromium; não precisa de driver.
3. **Criar atalho**: cria, por exemplo, `DiscordVPN` no Menu Iniciar (e,
   se quiser, na Área de Trabalho). Ele aparece na pesquisa do Windows ao lado
   do Discord original.

Ao abrir um atalho:

- Se a VPN não está ligada, o UAC aparece uma vez. Uma janela mostra o
  progresso, conecta, abre o app e fecha sozinha.
- Se a VPN já está ligada, o app abre na hora, sem UAC.
- O app abre como usuário comum, não como Administrador.
- Com "Desconectar a VPN quando os apps abertos por ela forem fechados"
  marcado e a janela fechada, a VPN desliga sozinha uns 30 s depois que os
  apps fecharem.

### Apps que abrem com o sistema

No diálogo do app (**Adicionar app…** ou **Editar**), marque **Abrir junto
com o Windows**. Ao entrar na conta, o vpn_specify conecta a VPN e abre todos
os apps marcados, sem mostrar a janela. Na lista, eles aparecem com o selo
"inicia com o sistema".

- **Sem UAC no logon**: o vpn_specify cria uma tarefa agendada,
  `vpn_specify\Start apps in VPN` (no Agendador de Tarefas), que roda
  `vpn_specify.exe autostart` 15 s depois do logon, já com privilégio de
  administrador. Os apps abrem como usuário comum, como nos atalhos.
- No Linux a opção é **Abrir junto com o sistema**. A entrada fica em
  `~/.config/autostart/vpn_specify.desktop` e roda 10 s depois de entrar na
  sessão. VPNs OpenVPN pedem a senha de administrador nessa hora.
- Há uma entrada só, criada quando o primeiro app é marcado e removida
  quando nenhum estiver. Ela é atualizada sempre que o vpn_specify abre, e
  segue o executável se ele mudar de pasta ou for atualizado.
- Como só uma VPN fica ativa por vez, os apps marcados precisam usar a
  **mesma VPN**. A tela avisa se não usarem.
- Logo depois do logon a rede pode demorar. Cada app é tentado até 4 vezes,
  com pausas. Senha recusada ou credenciais faltando não são repetidas. O
  que não abrir aparece numa mensagem.
- O desinstalador remove a tarefa.

Só uma VPN fica ativa por vez. Evite capturar nomes genéricos como
`cmd.exe`, porque todos os processos com esse nome entram na VPN.

Pasta de dados (botão **Pasta de dados**): `configs\`, `settings.json` (apps
e preferências), `language` e `vpn_specify.log`. Fica em
`%LOCALAPPDATA%\vpn_specify` quando instalado, ou em `data\` ao lado do exe
no modo portátil.

Não rode a versão instalada e uma cópia portátil ao mesmo tempo: as duas
usam as mesmas portas locais (10799–10801).

## Formatos suportados

| Config | Como funciona | Precisa de admin? |
|---|---|---|
| **WireGuard** (`.conf`) | O túnel roda inteiro dentro do processo (wireguard-go + pilha TCP/IP gVisor). Nenhum adaptador de rede é criado. | Não |
| **OpenVPN** (`.ovpn`) | Usa o `openvpn` instalado, mas bloqueia tudo que mudaria o roteamento ou o DNS do sistema (redirect-gateway, rotas, DNS, block-outside-dns). O proxy prende os sockets ao túnel, e é isso que manda o tráfego pela VPN. | Sim (o OpenVPN cria um adaptador) |

Se o OpenVPN não estiver instalado, o vpn_specify oferece a instalação ao conectar: no Windows, baixa o instalador oficial (conferido com um SHA-256 fixo) e instala em silêncio, sem a interface do OpenVPN; no Linux, instala o pacote `openvpn` da distribuição (apt, dnf, yum, zypper, pacman ou apk). Você também pode instalá-lo por conta própria.

## Uso

```powershell
# 1. Conectar (deixe esse terminal aberto; Ctrl+C desconecta)
vpn_specify-cli up C:\vpns\trabalho.ovpn

# 2. Em outro terminal, abrir apps dentro da VPN
vpn_specify-cli run chrome                       # Chrome separado, só ele na VPN
vpn_specify-cli run firefox
vpn_specify-cli run msedge https://intranet.empresa
vpn_specify-cli run curl https://ifconfig.me
vpn_specify-cli run git clone https://git.interno/repo.git
vpn_specify-cli run powershell                   # shell cujos comandos usam o proxy

# Conectar e já abrir um app
vpn_specify-cli up wg0.conf -- chrome

# Conferir: IP direto vs IP pela VPN, em TCP e em UDP
vpn_specify-cli status
```

### Captura por processo (`-capture`): TUDO de um app pela VPN

Para apps que ignoram proxy ou usam UDP próprio (Discord com voz, jogos,
apps nativos), o `-capture` intercepta os pacotes do processo pelo nome do
executável. Nada precisa ser configurado no app:

```powershell
# terminal como Administrador
vpn_specify-cli up trabalho.ovpn -capture discord.exe
# depois abra o Discord normalmente (atalho do Menu Iniciar)
```

- **TCP** vai por um proxy transparente; **UDP** (voz, vídeo) vai por sockets
  dentro da VPN; **IPv6** do processo é bloqueado, para o app usar IPv4 pela VPN.
- Os outros programas não são afetados, mas todo pacote de saída passa pelo
  `vpn_specify`, o que custou uns 10% de velocidade no teste.
- Se o app já estava aberto, as conexões TCP dele são derrubadas para
  reconectarem pela VPN. O mais garantido é abrir o app depois do `up`.
- Pode repetir: `-capture discord.exe -capture steam.exe`.
- Funciona com WireGuard e com OpenVPN.
- Os nomes (DNS) que o app resolve pelo Windows continuam usando o DNS normal
  do PC: o servidor DNS vê *quais* domínios, mas o tráfego vai pela VPN.

Requisitos: Administrador e os arquivos do [WinDivert 2.2.2](https://github.com/basil00/WinDivert/releases/tag/v2.2.2)
(`WinDivert.dll` e `WinDivert64.sys`, x64) em `.\windivert\` ou ao lado do
`.exe`. O Windows Defender costuma marcar o WinDivert como indesejado, porque
malwares também o usam; libere a pasta em Segurança do Windows > Exclusões.
Durante a captura existe uma regra de firewall temporária, "vpn_specify
capture", removida ao sair.

### UDP

Há duas formas:

1. **SOCKS5 UDP** (`127.0.0.1:10800`, UDP ASSOCIATE): para apps que
   suportam UDP via SOCKS5.
2. **Encaminhamento de porta `-L`**: funciona com **qualquer** programa,
   mesmo os que não sabem usar proxy. A porta local vira um endereço dentro da VPN:

```powershell
vpn_specify-cli up trabalho.ovpn -L udp:15353=10.0.0.5:53 -L tcp:13389=servidor-interno:3389
# agora: DNS interno em 127.0.0.1:15353 e RDP interno em 127.0.0.1:13389
mstsc /v:127.0.0.1:13389
```

`-L` aceita `[tcp|udp:]portaLocal=host:porta` (ou `ip:porta` local) e pode ser
repetido. Para UDP, cada origem local ganha sua própria sessão; sessões sem
resposta por 2 minutos são fechadas.

### Opções úteis

- `up -v`: log completo (inclusive do OpenVPN).
- `up -dns 10.0.0.53`: força o DNS usado dentro da VPN.
- `up -socks 127.0.0.1:20800 -http 127.0.0.1:20801`: outras portas, por exemplo para duas VPNs ao mesmo tempo. Use as mesmas portas no `run`.
- `run -profile cliente2 chrome`: perfil de navegador separado (cookies, logins).
- `run -chromium <app Electron>`: força as flags de proxy do Chromium.
- `run -detach <programa>`: não espera o programa terminar.

## Como cada app é colocado na VPN

- **Chrome / Edge / Brave / Vivaldi**: `--proxy-server=socks5://...`, um perfil
  próprio em `%LOCALAPPDATA%\vpn_specify\profiles\` (senão a janela abriria no
  Chrome que já está aberto, fora da VPN), DNS remoto e WebRTC só pelo proxy.
- **Firefox**: perfil próprio com proxy SOCKS5, DNS remoto e DoH desativado.
- **Qualquer outro programa**: variáveis `HTTP_PROXY`, `HTTPS_PROXY` e
  `ALL_PROXY`. Funciona com curl, git, npm, pip, Go, .NET HttpClient e vários outros.

Os nomes (DNS) são resolvidos **dentro da VPN**, então nomes internos
(`intranet.empresa`) funcionam e não vazam para o DNS da sua rede.

## Limitações

- **Apps que ignoram proxy**: use `-capture` (ou `-L` para endereços fixos).
- Chrome e Firefox não mandam UDP (QUIC/HTTP3) por SOCKS; eles usam TCP, o
  que funciona normalmente.
- UDP e o modo OpenVPN só funcionam com **IPv4**.
- Se o `vpn_specify` for morto à força, o `openvpn` morre junto (Job Object
  no Windows, SIGTERM no Linux), mas a rota de métrica 9999 pode ficar no
  adaptador TAP desconectado. Ela não afeta nada; o Ctrl+C normal remove tudo.
- O proxy escuta só em `127.0.0.1`, mas **qualquer programa do seu PC** pode
  usá-lo enquanto o `up` estiver rodando.

## Linux

No Linux funcionam a interface gráfica e os comandos de terminal (`up`,
`run`, `status`, `update` e `-L`), com WireGuard e OpenVPN, nas mesmas portas
do Windows. O instalador e o `-capture` continuam só no Windows.

```sh
tar -xzf vpn_specify-<versão>-linux-amd64.tar.gz   # ou -linux-arm64
sudo install vpn_specify-<versão>-linux-amd64/vpn_specify /usr/local/bin/
vpn_specify                                # abre a interface gráfica
```

### Interface gráfica no Linux

Abra `vpn_specify` sem argumentos. Na primeira vez, use **Preferências >
Adicionar "vpn_specify" ao menu de aplicativos** para ter o ícone no menu.
A tela é a mesma do Windows, com estas diferenças:

- **Janela**: é a mesma página do Windows, servida pelo próprio vpn_specify
  em `127.0.0.1` (porta aleatória, protegida por um token aleatório). Ela é
  aberta como janela de app, sem abas nem barra de endereço, do Chrome,
  Chromium, Edge, Brave ou Vivaldi, com um perfil separado. Sem nenhum
  deles, ela abre como aba no navegador padrão. Funciona com X11 e Wayland.
- **Sem root**: a interface roda como usuário comum. Ao conectar uma VPN
  **OpenVPN**, o sistema pede a senha de administrador (`pkexec`, do
  polkit) só para o `openvpn`. Ele fica num pequeno processo auxiliar que o
  encerra quando você desconecta ou fecha o vpn_specify. Prender o socket ao
  túnel sem root precisa do kernel 5.7 ou mais novo. WireGuard não pede
  senha nenhuma.
- **Adicionar app** mostra os apps do menu de aplicativos (arquivos
  `.desktop`, inclusive flatpak e snap), com busca. **Escolher arquivo…**
  usa o zenity ou o kdialog; sem eles, digite o caminho no campo Programa.
- **Só o modo proxy**: não há captura completa. Navegadores, apps
  Chromium/Electron (Discord, Slack, VS Code…) e programas que respeitam
  `HTTP_PROXY` entram na VPN. Voz/UDP do Discord e jogos não entram.
- **Criar atalho** grava um `.desktop` em `~/.local/share/applications/`
  (aparece na pesquisa do menu, ao lado do app original) e, se marcado, na
  Área de Trabalho.
- **Credenciais**: a senha é cifrada (AES-GCM) com uma chave em
  `~/.local/share/vpn_specify/secret.key` (permissão 0600), separada do
  `settings.json`. Ela fica protegida pelas permissões do arquivo, como uma
  chave SSH, e não pelo chaveiro do sistema. Senhas salvas no Windows (DPAPI)
  precisam ser digitadas de novo.
- **Dados**: `~/.config/vpn_specify/` (`configs/`, `settings.json`,
  `language`, `vpn_specify.log`).

O ciclo de vida é o do Windows. Ao abrir um atalho com a VPN desligada, a
janela mostra o progresso, conecta, abre o app e fecha sozinha. Com a janela
fechada, a VPN desliga uns 30 s depois que os apps abertos por ela fecharem.

### Terminal no Linux

```sh
vpn_specify up wg0.conf                    # WireGuard: sem root
sudo vpn_specify up trabalho.ovpn          # OpenVPN: precisa de root e do pacote openvpn

# em outro terminal, como usuário comum
vpn_specify run chrome                     # acha google-chrome-stable, chromium...
vpn_specify run firefox
vpn_specify run curl https://ifconfig.me
vpn_specify status
```

Diferenças em relação ao Windows:

- **OpenVPN**: no Linux, ligar o socket ao IP do túnel não basta para ele
  sair pelo túnel. Por isso o proxy prende os sockets à interface `tun` com
  `SO_BINDTODEVICE` e deixa o `rp_filter` dessa interface em modo *loose*.
  Sem isso, distribuições com `rp_filter` estrito (Fedora, RHEL) descartariam
  as respostas. Rotas e DNS do sistema continuam intocados, como no Windows.
  Se o `vpn_specify` for morto, o `openvpn` recebe SIGTERM e sai também.
- **`sudo vpn_specify up ... -- programa`**: o programa é aberto como quem
  chamou o `sudo`, não como root. O perfil do navegador fica na home desse
  usuário. Mesmo assim, o mais simples é rodar o `run` em outro terminal, sem
  `sudo`.
- **Perfis dos navegadores**: ficam em `~/.config/vpn_specify/profiles/`.
  Firefox e Chromium instalados por snap (padrão no Ubuntu) não podem ler
  pastas ocultas da home, então nesses casos o perfil vai para
  `~/snap/<app>/common/vpn_specify/profiles/`.
- **Apps que ignoram proxy**: sem `-capture`, use `-L` para endereços fixos.

## Android

Instale o `vpn_specify-<versão>-android-arm64.apk` (Android 8 ou superior;
permita a instalação dessa origem quando pedir). O Android tem VPN por aplicativo
nativa, então o app não usa proxy: todo o tráfego TCP e UDP dos apps escolhidos,
incluindo voz, passa pelo túnel, sem precisar de root. O resto do celular fica na
sua rede normal.

1. **Importar arquivo…**: escolha um `.conf` do WireGuard ou um `.ovpn` do OpenVPN.
2. **Escolher apps…**: marque os apps que usam a VPN, o Discord por exemplo.
3. **Conectar**, e aceite a permissão de VPN do Android na primeira vez.

O Android não consegue perceber quando um app é aberto, então há duas formas de
um app usar a VPN:

- **Deixar a VPN ligada** (recomendado): o app fica sempre na VPN, seja como for
  aberto, e nada mais fica. **Conectar quando o celular ligar** faz isso a cada
  início. Nas configurações de VPN do sistema, **VPN sempre ativa** faz o Android
  religá-la sozinho, e **Bloquear conexões sem VPN** impede que os apps escolhidos
  saiam por fora dela (também bloqueia o resto do celular).
- **Atalho**: cria um ícone "Discord (VPN)" na tela inicial, que conecta e depois
  abre o app. Abrir pelo ícone normal não liga a VPN.

Limitações no Android:

- Um app escolhido nunca sai por fora da VPN. Se a VPN só alcança uma rede interna, sites da internet não abrem nesse app.
- OpenVPN: os certificados e chaves precisam estar dentro do `.ovpn`, TAP não é suportado e chave privada com senha ainda não é suportada. Diretivas que executariam programas são removidas.
- O Android mantém uma VPN ativa por vez: iniciar outra desliga esta.
- Mudar os apps ou a VPN com a conexão ativa recria o túnel, o que interrompe os
  apps escolhidos por um instante.

O idioma segue o do sistema e pode ser trocado em **Configurações**. O app procura
novas versões no GitHub, e o Android pede confirmação a cada atualização.

## Atualizações

Ao abrir a interface, o vpn_specify consulta a última versão nas
[releases do GitHub](https://github.com/D3T3F/vpn_specify/releases). Se
houver uma mais nova, aparece um aviso no topo da janela:

- **Atualizar**: baixa o pacote, confere o SHA-256, desconecta, fecha,
  instala e abre de novo já na versão nova.
- **Agora não**: esconde o aviso até a próxima vez que o app abrir.
- **Ignorar esta versão**: não avisa mais dessa versão, só da seguinte.
- **Novidades**: mostra o texto da release.

Em **Preferências** dá para desligar a busca ao abrir e procurar na hora
(**Procurar atualizações**). No terminal:

```sh
vpn_specify update           # mostra a versão nova e pergunta se quer instalar
vpn_specify update -check    # só verifica
vpn_specify update -y        # instala sem perguntar
vpn_specify version
```

Como a versão nova é instalada:

- **Windows, instalado**: o `vpn_specify-setup.exe` da release roda em modo
  de atualização. Ele usa a mesma pasta, mantém os atalhos que existem e pede
  o UAC se preciso.
- **Windows, portátil**: o mesmo instalador só troca os arquivos da pasta,
  sem registro nem atalhos.
- **Linux**: o executável é trocado no lugar. Se a pasta exigir root (ex.:
  `/usr/local/bin`), o sistema pede a senha (`pkexec`). Antes da troca, o
  vpn_specify confere que o executável novo roda e informa a versão certa.

Segurança: o download só é aceito de
`github.com/D3T3F/vpn_specify/releases/download/` e se o SHA-256 bater com
o que o GitHub informa para o arquivo (ou com um `SHA256SUMS` na release).
Sem hash, nada é instalado. Isso protege contra download corrompido ou
adulterado no caminho, mas não contra alguém com acesso à conta do GitHub;
para isso seria preciso assinar os pacotes. Builds sem versão (`dev`) nunca
procuram atualizações.

## Idiomas

A interface, o instalador, as mensagens e a ajuda do terminal estão em:
English, Português (Brasil), Español, Français, Deutsch, Русский, 简体中文,
日本語, हिन्दी e العربية (com layout da direita para a esquerda).

- **Padrão**: o idioma do sistema, se for um desses (idioma de exibição do
  Windows; no Linux, `LANGUAGE`, `LC_ALL`, `LC_MESSAGES` ou `LANG`); senão,
  inglês.
- **Escolher**: no topo do instalador do Windows ou, depois, em
  **Preferências > Idioma** (a janela muda na hora). A escolha fica salva na
  pasta de dados (arquivo `language`) e vale também para os comandos de
  terminal. O instalador silencioso aceita `/lang=<código>`.
- Os logs técnicos e a ajuda de cada opção do terminal ficam em inglês.

Quer melhorar uma tradução ou adicionar um idioma? Veja
[Adding or improving a translation](../README.md#adding-or-improving-a-translation).

## Licença

O APK do Android contém o OpenVPN (GPL-2.0), então o APK como um todo é distribuído sob a GPL-2.0. O código deste projeto continua MIT, e o código-fonte de cada APK é este repositório na tag da versão, com `android/openvpn/build.sh`. As licenças estão no app, em **Configurações > Licenças**.

[MIT](../LICENSE). O texto aparece no programa (Preferências > Ver licença),
no instalador, nas propriedades dos `.exe` e vai no pacote como
`LICENSE.txt`. Componentes de terceiros mantêm as próprias licenças: o
WinDivert (LGPL-3.0 / GPL-2.0) é distribuído sem modificações, e as licenças
de todos os componentes vão na pasta `licenses\` do pacote.

O instalador não tem assinatura digital: ao abri-lo, o Windows pode mostrar o
SmartScreen ("O Windows protegeu o computador" > Mais informações > Executar
assim mesmo), como explica o `README.txt` do pacote.
