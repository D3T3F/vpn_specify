# vpn_specify

[English](../README.md) · [Português (Brasil)](README.pt-BR.md) · **Español** · [Français](README.fr.md) · [Deutsch](README.de.md) · [Русский](README.ru.md) · [简体中文](README.zh-CN.md) · [日本語](README.ja.md) · [हिन्दी](README.hi.md) · [العربية](README.ar.md)

Se conecta a una VPN **solo para las apps que elijas**. El resto del equipo
sigue en tu red normal.

```
            ┌──────────────── vpn_specify ────────────────┐
 chrome ──► │ proxy SOCKS5 127.0.0.1:10800 ┐               │
 curl   ──► │ proxy HTTP   127.0.0.1:10801 ┴──► túnel VPN ─┼──► red de la VPN
            └─────────────────────────────────────────────┘
 resto del PC ────────────────────────────────────────────────► internet normal
```

Sin servicios en segundo plano. En Windows hay dos ejecutables del mismo
código:

- `vpn_specify.exe`: interfaz gráfica (sin consola).
- `vpn_specify-cli.exe`: los comandos de terminal (`up`, `run`, `status`, `update`).

En Linux hay un único ejecutable, `vpn_specify`, con interfaz gráfica y
terminal (ver [Linux](#linux)).

La interfaz, el instalador y los mensajes están disponibles en **10 idiomas**
(ver [Idiomas](#idiomas)).

> Esta es la guía de uso. Para compilar, publicar versiones y conocer la
> estructura del código, consulta el [README en inglés](../README.md#building).

## Instalación

Descarga `vpn_specify-<versión>-win64.zip` desde las
[releases](https://github.com/D3T3F/vpn_specify/releases), extráelo y ejecuta
`vpn_specify-setup.exe`:

- Elige el idioma en la parte superior de la ventana del instalador; la app
  lo usará a partir de entonces (puedes cambiarlo luego en **Preferencias**).
- Carpeta predeterminada: `C:\Program Files\vpn_specify` (elige otra con **Examinar…**).
- Opciones: accesos directos en el escritorio y en el menú Inicio.
- No se descarga nada: el programa y WinDivert ya están dentro del instalador.
- Ejecutar de nuevo el instalador actualiza la instalación existente (misma carpeta).
- Desinstalar: Configuración > Aplicaciones > Aplicaciones instaladas >
  vpn_specify. Eliges si borrar o conservar los datos. Los accesos directos
  creados por la app (p. ej. `DiscordVPN`) también se eliminan.

Sin clics (para scripts):

```powershell
vpn_specify-setup.exe /silent [/dir=D:\Apps] [/nodesktop] [/nostartmenu] [/lang=es]
"C:\Program Files\vpn_specify\vpn_specify.exe" uninstall /silent [/keepdata]
```

También funciona sin instalar (modo portátil): basta con tener una carpeta
`data\` (o un archivo vacío llamado `portable`) junto a `vpn_specify.exe`.

## Interfaz gráfica

Abre `vpn_specify.exe`. Windows pide permiso de Administrador.

1. **VPN > Importar archivo…**: el `.ovpn`/`.conf` se copia a la carpeta de
   datos. Si la VPN pide inicio de sesión, usa **Credenciales**: la contraseña
   se guarda con DPAPI, así que solo tu cuenta de Windows puede leerla.
   **Editar** permite cambiar el nombre de la VPN (las apps que la usan lo
   siguen) y su contenido; antes hay que desconectar esa VPN.
2. **Apps en la VPN > Añadir app…**: elige el `.exe` o un acceso directo del
   menú Inicio (p. ej. Discord). En apps con actualizador (`Update.exe
   --processStart Discord.exe`) se detecta solo el proceso correcto
   (`discord.exe`).
   - *Captura completa* (predeterminado): todo el tráfico, incluidos voz/UDP (WinDivert).
   - *Proxy*: solo TCP, para navegadores y apps Chromium; no necesita controlador.
3. **Crear acceso directo**: crea, por ejemplo, `DiscordVPN` en el menú Inicio
   (y, si quieres, en el escritorio). Aparece en la búsqueda de Windows junto
   al Discord original.

Al abrir un acceso directo:

- Si la VPN no está activa, UAC aparece una vez. Una ventana muestra el
  progreso, conecta, abre la app y se cierra sola.
- Si la VPN ya está activa, la app se abre al instante, sin UAC.
- La app se abre como usuario normal, no como Administrador.
- Con "Desconectar la VPN cuando se cierren las apps abiertas con ella"
  marcado y la ventana cerrada, la VPN se apaga unos 30 s después de que se
  cierren las apps.

### Apps que se abren con el sistema

En el diálogo de la app (**Añadir app…** o **Editar**), marca **Abrir con
Windows**. Al iniciar sesión, vpn_specify conecta la VPN y abre todas las
apps marcadas, sin mostrar la ventana. En la lista llevan la etiqueta "se
abre con el sistema".

- **Sin UAC al iniciar sesión**: vpn_specify crea una tarea programada,
  `vpn_specify\Start apps in VPN` (en el Programador de tareas), que ejecuta
  `vpn_specify.exe autostart` 15 s después del inicio de sesión, ya con
  privilegios de administrador. Las apps se abren como usuario normal.
- En Linux la opción es **Abrir con el sistema**. La entrada es
  `~/.config/autostart/vpn_specify.desktop` y se ejecuta 10 s después de
  iniciar sesión. Las VPN OpenVPN piden la contraseña de administrador en ese
  momento.
- Hay una sola entrada, creada al marcar la primera app y eliminada cuando
  no queda ninguna. Se actualiza cada vez que vpn_specify se abre, así que
  sigue al ejecutable si cambia de carpeta o se actualiza.
- Solo hay una VPN activa a la vez, así que las apps marcadas deben usar la
  **misma VPN**. La ventana avisa si no es así.
- Justo después de iniciar sesión la red puede tardar. Cada app se intenta
  hasta 4 veces, con pausas. Una contraseña rechazada o credenciales
  faltantes no se reintentan. Lo que no se abra aparece en un mensaje.
- El desinstalador elimina la tarea.

Solo hay una VPN activa a la vez. Evita capturar nombres genéricos como
`cmd.exe`: todos los procesos con ese nombre entrarían en la VPN.

Carpeta de datos (botón **Carpeta de datos**): `configs\`, `settings.json`
(apps y preferencias), `language` y `vpn_specify.log`. Está en
`%LOCALAPPDATA%\vpn_specify` cuando se instala, o en `data\` junto al exe en
modo portátil.

No ejecutes la versión instalada y una copia portátil a la vez: ambas usan
los mismos puertos locales (10799–10801).

## Formatos compatibles

| Configuración | Cómo funciona | ¿Necesita administrador? |
|---|---|---|
| **WireGuard** (`.conf`) | El túnel funciona entero dentro del proceso (wireguard-go + pila TCP/IP de gVisor). No se crea ningún adaptador de red. | No |
| **OpenVPN** (`.ovpn`) | Usa el `openvpn` instalado, pero bloquea todo lo que cambiaría el enrutamiento o el DNS del sistema (redirect-gateway, rutas, DNS, block-outside-dns). El proxy ata sus sockets al túnel, y eso es lo que envía el tráfico por la VPN. | Sí (OpenVPN crea un adaptador) |

Si falta OpenVPN, vpn_specify ofrece instalarlo al conectar: en Windows descarga el instalador oficial (comprobado con un SHA-256 fijo) y lo instala en silencio, sin la interfaz de OpenVPN; en Linux instala el paquete `openvpn` de la distribución (apt, dnf, yum, zypper, pacman o apk). También puedes instalarlo por tu cuenta.

## Uso

```powershell
# 1. Conectar (deja esta terminal abierta; Ctrl+C desconecta)
vpn_specify-cli up C:\vpns\trabajo.ovpn

# 2. En otra terminal, abrir apps dentro de la VPN
vpn_specify-cli run chrome                       # Chrome aparte, solo él en la VPN
vpn_specify-cli run firefox
vpn_specify-cli run msedge https://intranet.empresa
vpn_specify-cli run curl https://ifconfig.me
vpn_specify-cli run git clone https://git.interno/repo.git
vpn_specify-cli run powershell                   # shell cuyos comandos usan el proxy

# Conectar y abrir una app enseguida
vpn_specify-cli up wg0.conf -- chrome

# Comprobar: IP directa frente a IP por la VPN, en TCP y UDP
vpn_specify-cli status
```

### Captura por proceso (`-capture`): TODO el tráfico de una app por la VPN

Para apps que ignoran el proxy o usan su propio UDP (voz de Discord, juegos,
apps nativas), `-capture` intercepta los paquetes del proceso por el nombre
del ejecutable. No hay que configurar nada en la app:

```powershell
# terminal como Administrador
vpn_specify-cli up trabajo.ovpn -capture discord.exe
# luego abre Discord normalmente (acceso directo del menú Inicio)
```

- **TCP** va por un proxy transparente; **UDP** (voz, vídeo) por sockets
  dentro de la VPN; el **IPv6** del proceso se bloquea para que use IPv4 por
  la VPN.
- Los demás programas no se ven afectados, pero todo paquete saliente pasa
  por `vpn_specify`, lo que costó alrededor de un 10 % de velocidad en las
  pruebas.
- Si la app ya estaba abierta, sus conexiones TCP se cortan para que se
  reconecten por la VPN. Lo más seguro es abrir la app después del `up`.
- Se puede repetir: `-capture discord.exe -capture steam.exe`.
- Funciona con WireGuard y con OpenVPN.
- Los nombres (DNS) que la app resuelve mediante Windows siguen usando el DNS
  normal del PC: el servidor DNS ve *qué* dominios, pero el tráfico va por la
  VPN.

Requisitos: Administrador y los archivos de [WinDivert 2.2.2](https://github.com/basil00/WinDivert/releases/tag/v2.2.2)
(`WinDivert.dll` y `WinDivert64.sys`, x64) en `.\windivert\` o junto al
`.exe`. Windows Defender suele marcar WinDivert como no deseado, porque el
malware también lo usa; permite la carpeta en Seguridad de Windows >
Exclusiones. Durante la captura existe una regla de firewall temporal,
"vpn_specify capture", que se elimina al salir.

### UDP

Hay dos formas:

1. **SOCKS5 UDP** (`127.0.0.1:10800`, UDP ASSOCIATE): para apps compatibles
   con UDP sobre SOCKS5.
2. **Reenvío de puertos `-L`**: funciona con **cualquier** programa, incluso
   los que no saben usar un proxy. El puerto local se convierte en una
   dirección dentro de la VPN:

```powershell
vpn_specify-cli up trabajo.ovpn -L udp:15353=10.0.0.5:53 -L tcp:13389=servidor-interno:3389
# ahora: DNS interno en 127.0.0.1:15353 y RDP interno en 127.0.0.1:13389
mstsc /v:127.0.0.1:13389
```

`-L` acepta `[tcp|udp:]puertoLocal=host:puerto` (o un `ip:puerto` local) y se
puede repetir. En UDP, cada origen local tiene su propia sesión; las sesiones
sin respuesta durante 2 minutos se cierran.

### Opciones útiles

- `up -v`: registro completo (incluido el de OpenVPN).
- `up -dns 10.0.0.53`: fuerza el DNS usado dentro de la VPN.
- `up -socks 127.0.0.1:20800 -http 127.0.0.1:20801`: otros puertos, p. ej. para dos VPN a la vez. Usa los mismos puertos en `run`.
- `run -profile cliente2 chrome`: perfil de navegador aparte (cookies, sesiones).
- `run -chromium <app Electron>`: fuerza las flags de proxy de Chromium.
- `run -detach <programa>`: no espera a que el programa termine.

## Cómo entra cada app en la VPN

- **Chrome / Edge / Brave / Vivaldi**: `--proxy-server=socks5://...`, un perfil
  propio en `%LOCALAPPDATA%\vpn_specify\profiles\` (si no, la ventana se
  abriría en el Chrome que ya está abierto, fuera de la VPN), DNS remoto y
  WebRTC solo por el proxy.
- **Firefox**: perfil propio con proxy SOCKS5, DNS remoto y DoH desactivado.
- **Cualquier otro programa**: las variables `HTTP_PROXY`, `HTTPS_PROXY` y
  `ALL_PROXY`. Funciona con curl, git, npm, pip, Go, .NET HttpClient y muchos
  otros.

Los nombres (DNS) se resuelven **dentro de la VPN**, así que los nombres
internos (`intranet.empresa`) funcionan y no se filtran al DNS de tu red.

## Limitaciones

- **Apps que ignoran el proxy**: usa `-capture` (o `-L` para direcciones fijas).
- Chrome y Firefox no envían UDP (QUIC/HTTP3) por SOCKS; usan TCP, que
  funciona con normalidad.
- UDP y el modo OpenVPN solo funcionan con **IPv4**.
- Si `vpn_specify` se cierra a la fuerza, `openvpn` se cierra con él (Job
  Object en Windows, SIGTERM en Linux), pero la ruta de métrica 9999 puede
  quedar en el adaptador TAP desconectado. No afecta a nada; un Ctrl+C normal
  lo elimina todo.
- El proxy solo escucha en `127.0.0.1`, pero **cualquier programa de tu PC**
  puede usarlo mientras `up` esté en ejecución.

## Linux

En Linux funcionan la interfaz gráfica y los comandos de terminal (`up`,
`run`, `status`, `update` y `-L`), con WireGuard y OpenVPN, en los mismos
puertos que en Windows. El instalador y `-capture` siguen siendo solo de
Windows.

```sh
tar -xzf vpn_specify-<versión>-linux-amd64.tar.gz   # o -linux-arm64
sudo install vpn_specify-<versión>-linux-amd64/vpn_specify /usr/local/bin/
vpn_specify                                # abre la interfaz gráfica
```

### Interfaz gráfica en Linux

Ejecuta `vpn_specify` sin argumentos. La primera vez, usa **Preferencias >
Añadir "vpn_specify" al menú de aplicaciones** para tener el icono en el
menú. La ventana es la misma que en Windows, con estas diferencias:

- **Ventana**: es la misma página de Windows, servida por el propio
  vpn_specify en `127.0.0.1` (puerto aleatorio, protegida por un token
  aleatorio). Se abre como ventana de aplicación (sin pestañas ni barra de
  direcciones) de Chrome, Chromium, Edge, Brave o Vivaldi, con un perfil
  aparte. Si no hay ninguno, se abre como pestaña en el navegador
  predeterminado. Funciona con X11 y Wayland.
- **Sin root**: la interfaz se ejecuta como usuario normal. Al conectar una
  VPN **OpenVPN**, el sistema pide la contraseña de administrador (`pkexec`,
  de polkit) solo para `openvpn`. Este se ejecuta en un pequeño proceso
  auxiliar que lo detiene al desconectar o cerrar vpn_specify. Atar los
  sockets al túnel sin root requiere el kernel 5.7 o posterior. WireGuard no
  pide ninguna contraseña.
- **Añadir app** muestra las apps del menú de aplicaciones (archivos
  `.desktop`, incluidos flatpak y snap), con búsqueda. **Elegir archivo…**
  usa zenity o kdialog; sin ellos, escribe la ruta en el campo Programa.
- **Solo modo proxy**: no hay captura completa. Entran en la VPN los
  navegadores, las apps Chromium/Electron (Discord, Slack, VS Code…) y los
  programas que respetan `HTTP_PROXY`. La voz/UDP de Discord y los juegos no.
- **Crear acceso directo** escribe un `.desktop` en
  `~/.local/share/applications/` (aparece en la búsqueda del menú, junto a la
  app original) y, si se marca, en el escritorio.
- **Credenciales**: la contraseña se cifra (AES-GCM) con una clave en
  `~/.local/share/vpn_specify/secret.key` (permisos 0600), separada de
  `settings.json`. La protegen los permisos del archivo, como una clave SSH,
  no el llavero del sistema. Las contraseñas guardadas en Windows (DPAPI)
  deben escribirse de nuevo.
- **Datos**: `~/.config/vpn_specify/` (`configs/`, `settings.json`,
  `language`, `vpn_specify.log`).

El ciclo de vida es el mismo que en Windows. Al abrir un acceso directo con
la VPN apagada, la ventana muestra el progreso, conecta, abre la app y se
cierra. Con la ventana cerrada, la VPN se apaga unos 30 s después de que se
cierren las apps abiertas con ella.

### Terminal en Linux

```sh
vpn_specify up wg0.conf                    # WireGuard: sin root
sudo vpn_specify up trabajo.ovpn           # OpenVPN: necesita root y el paquete openvpn

# en otra terminal, como usuario normal
vpn_specify run chrome                     # encuentra google-chrome-stable, chromium...
vpn_specify run firefox
vpn_specify run curl https://ifconfig.me
vpn_specify status
```

Diferencias con Windows:

- **OpenVPN**: en Linux, atar el socket a la IP del túnel no basta para que
  salga por el túnel. Por eso el proxy ata sus sockets a la interfaz `tun` con
  `SO_BINDTODEVICE` y pone el `rp_filter` de esa interfaz en modo *loose*. Sin
  eso, las distribuciones con `rp_filter` estricto (Fedora, RHEL) descartarían
  las respuestas. Las rutas y el DNS del sistema no se tocan, igual que en
  Windows. Si `vpn_specify` se cierra a la fuerza, `openvpn` recibe SIGTERM y
  también termina.
- **`sudo vpn_specify up ... -- programa`**: el programa se abre como quien
  ejecutó `sudo`, no como root. El perfil del navegador queda en la carpeta
  personal de ese usuario. Aun así, lo más sencillo es usar `run` en otra
  terminal, sin `sudo`.
- **Perfiles de navegador**: en `~/.config/vpn_specify/profiles/`. Firefox y
  Chromium instalados como snap (lo predeterminado en Ubuntu) no pueden leer
  carpetas ocultas de la carpeta personal, así que para ellos el perfil va en
  `~/snap/<app>/common/vpn_specify/profiles/`.
- **Apps que ignoran el proxy**: sin `-capture`, usa `-L` para direcciones fijas.

## Android

Instala `vpn_specify-<versión>-android-arm64.apk` (Android 8 o superior; permite
instalar desde ese origen cuando te lo pida). Android incluye VPN por aplicación,
así que la app no usa proxy: todo el tráfico TCP y UDP de las apps elegidas, voz
incluida, pasa por el túnel, sin root. El resto del teléfono sigue en tu red normal.

1. **Importar archivo…**: elige un `.conf` de WireGuard o un `.ovpn` de OpenVPN.
2. **Elegir apps…**: marca las apps que usan la VPN, Discord por ejemplo.
3. **Conectar**, y acepta el permiso de VPN de Android la primera vez.

Android no puede detectar cuándo se abre una app, así que hay dos formas de que
una app use la VPN:

- **Dejar la VPN encendida** (recomendado): la app está siempre en la VPN, se abra
  como se abra, y nada más lo está. **Conectar al encender el teléfono** lo hace en
  cada inicio. En los ajustes de VPN del sistema, **VPN siempre activa** hace que
  Android la reinicie por sí solo, y **Bloquear conexiones sin VPN** impide que las
  apps elegidas salgan por fuera de ella (también bloquea el resto del teléfono).
- **Acceso directo**: crea un icono "Discord (VPN)" en la pantalla de inicio, que
  conecta y luego abre la app. Abrirla con el icono normal no enciende la VPN.

Limitaciones en Android:

- OpenVPN: los certificados y claves deben estar dentro del `.ovpn`, TAP no es compatible y una clave privada con contraseña aún no lo es. Se eliminan las directivas que ejecutarían programas.
- Android mantiene una sola VPN activa: iniciar otra apaga esta.
- Cambiar las apps o la VPN mientras está conectada recrea el túnel, lo que
  interrumpe las apps elegidas un momento.

El idioma sigue al del sistema y se puede cambiar en **Ajustes**. La app busca
versiones nuevas en GitHub, y Android pide confirmar cada actualización.

## Actualizaciones

Al abrir la interfaz, vpn_specify consulta la última versión en las
[releases de GitHub](https://github.com/D3T3F/vpn_specify/releases). Si hay
una más nueva, aparece un aviso en la parte superior de la ventana:

- **Actualizar**: descarga el paquete, verifica su SHA-256, desconecta, se
  cierra, instala y se abre de nuevo ya con la versión nueva.
- **Ahora no**: oculta el aviso hasta la próxima vez que se abra la app.
- **Omitir esta versión**: no avisa más de esta versión, solo de la siguiente.
- **Novedades**: muestra el texto de la release.

En **Preferencias** puedes desactivar la comprobación al abrir y comprobar al
momento (**Buscar actualizaciones**). En la terminal:

```sh
vpn_specify update           # muestra la versión nueva y pregunta si instalarla
vpn_specify update -check    # solo comprueba
vpn_specify update -y        # instala sin preguntar
vpn_specify version
```

Cómo se instala la versión nueva:

- **Windows, instalado**: el `vpn_specify-setup.exe` de la release se ejecuta
  en modo actualización. Usa la misma carpeta, conserva los accesos directos
  existentes y pide UAC si hace falta.
- **Windows, portátil**: el mismo instalador solo reemplaza los archivos de
  la carpeta, sin registro ni accesos directos.
- **Linux**: el ejecutable se reemplaza en su sitio. Si la carpeta requiere
  root (p. ej. `/usr/local/bin`), el sistema pide la contraseña (`pkexec`).
  Antes de reemplazarlo, vpn_specify comprueba que el ejecutable nuevo
  funciona e informa la versión correcta.

Seguridad: la descarga solo se acepta desde
`github.com/D3T3F/vpn_specify/releases/download/` y si su SHA-256 coincide
con el que GitHub indica para el archivo (o con un `SHA256SUMS` en la
release). Sin hash, no se instala nada. Esto protege frente a una descarga
dañada o alterada por el camino, no frente a alguien con acceso a la cuenta
de GitHub; para eso habría que firmar los paquetes. Las compilaciones sin
versión (`dev`) nunca buscan actualizaciones.

## Idiomas

La interfaz, el instalador, los mensajes y la ayuda de la terminal están en:
English, Português (Brasil), Español, Français, Deutsch, Русский, 简体中文,
日本語, हिन्दी y العربية (con diseño de derecha a izquierda).

- **Predeterminado**: el idioma del sistema si es uno de estos (idioma de
  visualización de Windows; en Linux, `LANGUAGE`, `LC_ALL`, `LC_MESSAGES` o
  `LANG`); si no, inglés.
- **Elegir**: en la parte superior del instalador de Windows o, después, en
  **Preferencias > Idioma** (la ventana cambia al momento). La elección se
  guarda en la carpeta de datos (archivo `language`) y también se aplica a los
  comandos de terminal. El instalador silencioso acepta `/lang=<código>`.
- Los registros técnicos y la ayuda de cada opción de la terminal quedan en
  inglés.

¿Quieres mejorar una traducción o añadir un idioma? Consulta
[Adding or improving a translation](../README.md#adding-or-improving-a-translation).

## Licencia

El APK de Android contiene OpenVPN (GPL-2.0), por lo que el APK en su conjunto se distribuye bajo la GPL-2.0. El código de este proyecto sigue siendo MIT, y el código fuente de cada APK es este repositorio en la etiqueta de su versión, con `android/openvpn/build.sh`. Las licencias están en la app, en **Ajustes > Licencias**.

[MIT](../LICENSE). El texto aparece en el programa (Preferencias > Ver
licencia), en el instalador, en las propiedades de los `.exe` y va en el
paquete como `LICENSE.txt`. Los componentes de terceros mantienen sus propias
licencias: WinDivert (LGPL-3.0 / GPL-2.0) se distribuye sin modificar, y las
licencias de todos los componentes van en la carpeta `licenses\` del paquete.

El instalador no tiene firma digital: al abrirlo, Windows puede mostrar
SmartScreen ("Windows protegió su PC" > Más información > Ejecutar de todas
formas), como explica el `README.txt` del paquete.
