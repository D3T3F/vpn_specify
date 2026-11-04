# vpn_specify

**English** · [Português (Brasil)](docs/README.pt-BR.md) · [Español](docs/README.es.md) · [Français](docs/README.fr.md) · [Deutsch](docs/README.de.md) · [Русский](docs/README.ru.md) · [简体中文](docs/README.zh-CN.md) · [日本語](docs/README.ja.md) · [हिन्दी](docs/README.hi.md) · [العربية](docs/README.ar.md)

Connects to a VPN **only for the apps you choose**. The rest of the computer
stays on your normal network.

```
            ┌──────────────── vpn_specify ────────────────┐
 chrome ──► │ SOCKS5 proxy 127.0.0.1:10800 ┐               │
 curl   ──► │ HTTP proxy   127.0.0.1:10801 ┴──► VPN tunnel ┼──► VPN network
            └─────────────────────────────────────────────┘
 rest of the PC ──────────────────────────────────────────────► normal internet
```

No background service. On Windows there are two executables built from the
same code:

- `vpn_specify.exe`: graphical interface (no console).
- `vpn_specify-cli.exe`: the terminal commands (`up`, `run`, `status`, `update`).

On Linux there is a single executable, `vpn_specify`, with both the graphical
interface and the terminal commands (see [Linux](#linux)).

The interface, installer and messages are available in **10 languages**
(see [Languages](#languages)).

## Installation

Download (or build, see [Building](#building)) `vpn_specify-<version>-win64.zip`,
extract it and run `vpn_specify-setup.exe`:

- Choose the language at the top of the installer window; the app uses it
  from then on (you can change it later in **Preferences**).
- Default folder: `C:\Program Files\vpn_specify` (pick another with **Browse…**).
- Options: desktop and Start menu shortcuts.
- Nothing is downloaded: the program and WinDivert are inside the installer.
- Running the installer again updates the existing installation (same folder).
- Uninstall: Settings > Apps > Installed apps > vpn_specify. You choose
  whether to delete or keep the data. Shortcuts created by the app (e.g.
  `DiscordVPN`) are removed too.

Unattended (for scripts):

```powershell
vpn_specify-setup.exe /silent [/dir=D:\Apps] [/nodesktop] [/nostartmenu] [/lang=pt-BR]
"C:\Program Files\vpn_specify\vpn_specify.exe" uninstall /silent [/keepdata]
```

It also works without installing (portable mode): just have a `data\` folder
(or an empty file named `portable`) next to `vpn_specify.exe`.

## Graphical interface

Open `vpn_specify.exe`. Windows asks for Administrator permission.

1. **VPNs > Import file…**: the `.ovpn`/`.conf` is copied into the data
   folder. If the VPN needs a login, use **Credentials**: the password is
   stored with DPAPI, so only your Windows account can read it. **Edit** lets
   you rename the VPN (the apps using it follow) and change the file content;
   that VPN must be disconnected first.
2. **Apps in the VPN > Add app…**: choose the `.exe` or a Start menu shortcut
   (e.g. Discord). For apps with an updater (`Update.exe --processStart
   Discord.exe`) the right process (`discord.exe`) is detected automatically.
   - *Full capture* (default): all traffic, including voice/UDP (WinDivert).
   - *Proxy*: TCP only, for browsers and Chromium apps; no driver needed.
3. **Create shortcut**: creates, for example, `DiscordVPN` in the Start menu
   (and, if you want, on the desktop). It shows up in Windows search next to
   the original Discord.

When a shortcut is opened:

- If the VPN is off, UAC appears once. A window shows the progress,
  connects, opens the app and closes by itself.
- If the VPN is already on, the app opens right away, without UAC.
- The app opens as a normal user, not as Administrator.
- With "Disconnect the VPN when the apps opened through it are closed"
  checked and the window closed, the VPN turns off about 30 s after the apps
  close.

### Apps that start with the system

In the app dialog (**Add app…** or **Edit**), check **Open with Windows**.
When you sign in, vpn_specify connects the VPN and opens every marked app,
without showing the window. In the list they carry the "starts with the
system" badge.

- **No UAC at sign-in**: vpn_specify creates a scheduled task,
  `vpn_specify\Start apps in VPN` (in Task Scheduler), that runs
  `vpn_specify.exe autostart` 15 s after sign-in, already with administrator
  privileges. The apps open as a normal user, like with shortcuts.
- On Linux the option is **Open with the system**. The entry is
  `~/.config/autostart/vpn_specify.desktop` and runs 10 s after you log in.
  OpenVPN VPNs ask for the administrator password at that moment.
- There is a single entry, created when the first app is marked and removed
  when none is. It is refreshed every time vpn_specify starts, so it follows
  the executable if it moves or is updated.
- Only one VPN is active at a time, so the marked apps must use the **same
  VPN**. The window warns you otherwise.
- Right after sign-in the network may take a while. Each app is tried up to 4
  times, with pauses. A refused password or missing credentials are not
  retried. Whatever fails to open is shown in a message.
- The uninstaller removes the task.

Only one VPN is active at a time. Avoid capturing generic names such as
`cmd.exe`: every process with that name would enter the VPN.

Data folder (**Data folder** button): `configs\`, `settings.json` (apps and
preferences), `language` and `vpn_specify.log`. It is
`%LOCALAPPDATA%\vpn_specify` when installed, or `data\` next to the exe in
portable mode.

Do not run the installed version and a portable copy at the same time: both
use the same local ports (10799–10801).

## Supported formats

| Config | How it works | Needs admin? |
|---|---|---|
| **WireGuard** (`.conf`) | The tunnel runs entirely inside the process (wireguard-go + gVisor TCP/IP stack). No network adapter is created. | No |
| **OpenVPN** (`.ovpn`) | Runs the installed `openvpn`, but blocks everything that would change the system routing or DNS (redirect-gateway, routes, DNS, block-outside-dns). The proxy binds its sockets to the tunnel, which is what sends the traffic through the VPN. | Yes (OpenVPN creates an adapter) |

If OpenVPN is missing, vpn_specify offers to install it when you connect: on Windows it downloads the official installer (checked against a pinned SHA-256) and installs it silently, without the OpenVPN GUI; on Linux it installs the distribution's `openvpn` package (apt, dnf, yum, zypper, pacman or apk). You can also install it yourself.

## Usage

```powershell
# 1. Connect (keep this terminal open; Ctrl+C disconnects)
vpn_specify-cli up C:\vpns\work.ovpn

# 2. In another terminal, open apps inside the VPN
vpn_specify-cli run chrome                       # separate Chrome, only it in the VPN
vpn_specify-cli run firefox
vpn_specify-cli run msedge https://intranet.company
vpn_specify-cli run curl https://ifconfig.me
vpn_specify-cli run git clone https://git.internal/repo.git
vpn_specify-cli run powershell                   # shell whose commands use the proxy

# Connect and open an app right away
vpn_specify-cli up wg0.conf -- chrome

# Check: direct IP vs IP through the VPN, over TCP and UDP
vpn_specify-cli status
```

### Per-process capture (`-capture`): ALL of an app through the VPN

For apps that ignore proxies or use their own UDP (Discord voice, games,
native apps), `-capture` intercepts the packets of the process by its
executable name. Nothing needs to be configured in the app:

```powershell
# terminal running as Administrator
vpn_specify-cli up work.ovpn -capture discord.exe
# then open Discord normally (Start menu shortcut)
```

- **TCP** goes through a transparent proxy; **UDP** (voice, video) through
  sockets inside the VPN; the process's **IPv6** is blocked, so the app uses
  IPv4 through the VPN.
- Other programs are not affected, but every outgoing packet passes through
  `vpn_specify`, which cost about 10% of speed in testing.
- If the app was already open, its TCP connections are dropped so they
  reconnect through the VPN. Opening the app after `up` is the safest.
- Repeatable: `-capture discord.exe -capture steam.exe`.
- Works with WireGuard and OpenVPN.
- Names (DNS) the app resolves through Windows still use the PC's normal DNS:
  the DNS server sees *which* domains, but the traffic goes through the VPN.

Requirements: Administrator and the [WinDivert 2.2.2](https://github.com/basil00/WinDivert/releases/tag/v2.2.2)
files (`WinDivert.dll` and `WinDivert64.sys`, x64) in `.\windivert\` or next to
the `.exe`. Windows Defender often flags WinDivert as unwanted, because malware
uses it too; allow the folder in Windows Security > Exclusions. During capture
there is a temporary firewall rule, "vpn_specify capture", removed on exit.

### UDP

There are two ways:

1. **SOCKS5 UDP** (`127.0.0.1:10800`, UDP ASSOCIATE): for apps that support
   UDP over SOCKS5.
2. **`-L` port forwarding**: works with **any** program, even those that do
   not know about proxies. The local port becomes an address inside the VPN:

```powershell
vpn_specify-cli up work.ovpn -L udp:15353=10.0.0.5:53 -L tcp:13389=internal-server:3389
# now: internal DNS at 127.0.0.1:15353 and internal RDP at 127.0.0.1:13389
mstsc /v:127.0.0.1:13389
```

`-L` accepts `[tcp|udp:]localPort=host:port` (or a local `ip:port`) and is
repeatable. For UDP, each local source gets its own session; sessions without
a reply for 2 minutes are closed.

### Useful options

- `up -v`: full log (including OpenVPN's).
- `up -dns 10.0.0.53`: force the DNS used inside the VPN.
- `up -socks 127.0.0.1:20800 -http 127.0.0.1:20801`: other ports, e.g. for two VPNs at once. Use the same ports with `run`.
- `run -profile client2 chrome`: separate browser profile (cookies, logins).
- `run -chromium <Electron app>`: force the Chromium proxy flags.
- `run -detach <program>`: do not wait for the program to finish.

## How each app is put in the VPN

- **Chrome / Edge / Brave / Vivaldi**: `--proxy-server=socks5://...`, a profile
  of its own under `%LOCALAPPDATA%\vpn_specify\profiles\` (otherwise the
  window would open in the Chrome already running, outside the VPN), remote
  DNS and WebRTC only through the proxy.
- **Firefox**: its own profile with a SOCKS5 proxy, remote DNS and DoH off.
- **Any other program**: the `HTTP_PROXY`, `HTTPS_PROXY` and `ALL_PROXY`
  variables. Works with curl, git, npm, pip, Go, .NET HttpClient and many
  others.

Names (DNS) are resolved **inside the VPN**, so internal names
(`intranet.company`) work and do not leak to your network's DNS.

## Limitations

- **Apps that ignore proxies**: use `-capture` (or `-L` for fixed addresses).
- Chrome and Firefox do not send UDP (QUIC/HTTP3) over SOCKS; they fall back
  to TCP, which works normally.
- UDP and OpenVPN mode only work with **IPv4**.
- If `vpn_specify` is killed, `openvpn` dies with it (Job Object on Windows,
  SIGTERM on Linux), but the metric-9999 route may stay on the disconnected
  TAP adapter. It affects nothing; a normal Ctrl+C removes everything.
- The proxy only listens on `127.0.0.1`, but **any program on your PC** can
  use it while `up` is running.

## Linux

On Linux the graphical interface and the terminal commands (`up`, `run`,
`status`, `update` and `-L`) work, with WireGuard and OpenVPN, on the same
ports as on Windows. The installer and `-capture` remain Windows-only.

```sh
tar -xzf vpn_specify-<version>-linux-amd64.tar.gz   # or -linux-arm64
sudo install vpn_specify-<version>-linux-amd64/vpn_specify /usr/local/bin/
vpn_specify                                # opens the graphical interface
```

### Graphical interface on Linux

Run `vpn_specify` without arguments. The first time, use **Preferences > Add
"vpn_specify" to the applications menu** to get the icon in the menu. The
window is the same as on Windows, with these differences:

- **Window**: it is the same page as on Windows, served by vpn_specify itself
  on `127.0.0.1` (random port, protected by a random token). It opens as an
  app window (no tabs or address bar) of Chrome, Chromium, Edge, Brave or
  Vivaldi, with a separate profile. Without any of them, it opens as a tab in
  the default browser. Works with X11 and Wayland.
- **No root**: the interface runs as a normal user. When connecting an
  **OpenVPN** VPN, the system asks for the administrator password (`pkexec`,
  from polkit) only for `openvpn`. It runs in a small helper process that
  stops it when you disconnect or close vpn_specify. Binding sockets to the
  tunnel without root needs kernel 5.7 or newer. WireGuard asks for no
  password at all.
- **Add app** lists the apps of the applications menu (`.desktop` files,
  including flatpak and snap), with search. **Choose file…** uses zenity or
  kdialog; without them, type the path in the Program field.
- **Proxy mode only**: there is no full capture. Browsers, Chromium/Electron
  apps (Discord, Slack, VS Code…) and programs that honor `HTTP_PROXY` enter
  the VPN. Discord voice/UDP and games do not.
- **Create shortcut** writes a `.desktop` in `~/.local/share/applications/`
  (it shows up in the menu search, next to the original app) and, if checked,
  on the desktop.
- **Credentials**: the password is encrypted (AES-GCM) with a key in
  `~/.local/share/vpn_specify/secret.key` (mode 0600), kept apart from
  `settings.json`. It is protected by the file permissions, like an SSH key,
  not by the system keyring. Passwords saved on Windows (DPAPI) must be typed
  again.
- **Data**: `~/.config/vpn_specify/` (`configs/`, `settings.json`, `language`,
  `vpn_specify.log`).

The lifecycle is the same as on Windows. Opening a shortcut with the VPN off
shows the progress window, connects, opens the app and closes the window.
With the window closed, the VPN turns off about 30 s after the apps opened
through it close.

### Terminal on Linux

```sh
vpn_specify up wg0.conf                    # WireGuard: no root
sudo vpn_specify up work.ovpn              # OpenVPN: needs root and the openvpn package

# in another terminal, as a normal user
vpn_specify run chrome                     # finds google-chrome-stable, chromium...
vpn_specify run firefox
vpn_specify run curl https://ifconfig.me
vpn_specify status
```

Differences from Windows:

- **OpenVPN**: on Linux, binding the socket to the tunnel IP is not enough for
  it to leave through the tunnel. So the proxy binds its sockets to the `tun`
  interface with `SO_BINDTODEVICE` and sets that interface's `rp_filter` to
  *loose*. Without it, distributions with a strict `rp_filter` (Fedora, RHEL)
  would drop the replies. System routes and DNS stay untouched, like on
  Windows. If `vpn_specify` is killed, `openvpn` gets SIGTERM and exits too.
- **`sudo vpn_specify up ... -- program`**: the program is opened as whoever
  called `sudo`, not as root. The browser profile goes in that user's home.
  Still, the simplest is to run `run` in another terminal, without `sudo`.
- **Browser profiles**: in `~/.config/vpn_specify/profiles/`. Firefox and
  Chromium installed as snaps (Ubuntu's default) cannot read hidden home
  folders, so for them the profile goes in
  `~/snap/<app>/common/vpn_specify/profiles/`.
- **Apps that ignore proxies**: without `-capture`, use `-L` for fixed
  addresses.

## Android

Install `vpn_specify-<version>-android-arm64.apk` (Android 8 or newer; allow
installing from this source when asked). Android has per-app VPN built in, so
the app does not use a proxy: all the TCP and UDP traffic of the chosen apps,
voice included, goes through the tunnel, and needs no root. The rest of the
phone stays on your normal network.

1. **Import file…**: choose a WireGuard `.conf` or an OpenVPN `.ovpn`.
2. **Choose apps…**: mark the apps that use the VPN, Discord for example.
3. **Connect**, and accept Android's VPN permission the first time.

Android cannot notice an app being opened, so there are two ways to have an
app use the VPN:

- **Leave the VPN on** (recommended): the app is always in the VPN, however it
  is opened, and nothing else is. **Connect when the phone starts** does this at
  every start. In the system's VPN settings, **Always-on VPN** makes Android
  restart it by itself, and **Block connections without VPN** stops the chosen
  apps from leaving outside it (it also blocks the rest of the phone).
- **Shortcut**: creates a "Discord (VPN)" icon on the home screen, which connects
  and then opens the app. Opening the normal icon does not start the VPN.

Limitations on Android:

- OpenVPN: the certificates and keys must be inside the `.ovpn`, TAP is not supported, and a private key with a password is not supported yet. Directives that would run programs are removed.
- Android keeps one VPN active at a time: starting another one turns this one off.
- Changing the apps or the VPN while connected recreates the tunnel, which
  interrupts the chosen apps for a moment.

The language follows the system and can be changed in **Settings**. The app
looks for new versions on GitHub, and Android asks you to confirm each update.

## Updates

When the interface opens, vpn_specify checks the latest version in the
[GitHub releases](https://github.com/D3T3F/vpn_specify/releases). If there is
a newer one, a notice appears at the top of the window:

- **Update**: downloads the package, checks its SHA-256, disconnects, closes,
  installs and opens again on the new version.
- **Not now**: hides the notice until the next time the app opens.
- **Skip this version**: no more notices about this version, only the next.
- **What's new**: shows the release text.

In **Preferences** you can turn the check on start off and check right away
(**Check for updates**). In the terminal:

```sh
vpn_specify update           # shows the new version and asks whether to install it
vpn_specify update -check    # only checks
vpn_specify update -y        # installs without asking
vpn_specify version
```

How the new version is installed:

- **Windows, installed**: the release's `vpn_specify-setup.exe` runs in update
  mode. It uses the same folder, keeps the existing shortcuts and asks for UAC
  if needed.
- **Windows, portable**: the same installer only replaces the folder's
  files, with no registry entries or shortcuts.
- **Linux**: the executable is replaced in place. If the folder needs root
  (e.g. `/usr/local/bin`), the system asks for the password (`pkexec`). Before
  replacing it, vpn_specify checks that the new executable runs and reports
  the right version.

Security: a download is only accepted from
`github.com/D3T3F/vpn_specify/releases/download/` and if its SHA-256 matches
what GitHub reports for the file (or a `SHA256SUMS` file in the release).
Without a hash, nothing is installed. This protects against a download
corrupted or tampered with on the way, not against someone with access to the
GitHub account; for that the packages would need to be signed. Builds
without a version (`dev`) never check for updates.

## Languages

The interface, installer, messages and terminal help are available in:

| Code | Language |
|---|---|
| `en` | English (default) |
| `pt-BR` | Português (Brasil) |
| `es` | Español |
| `fr` | Français |
| `de` | Deutsch |
| `ru` | Русский |
| `zh-CN` | 简体中文 |
| `ja` | 日本語 |
| `hi` | हिन्दी |
| `ar` | العربية (right-to-left layout) |

- **Default**: the system language when it is one of the above
  (Windows display language; on Linux `LANGUAGE`, `LC_ALL`, `LC_MESSAGES` or
  `LANG`), otherwise English.
- **Choosing**: at the top of the Windows installer, or later in
  **Preferences > Language** (the window switches right away). The choice is
  saved in the data folder (`language` file) and also applies to the terminal
  commands. The silent installer takes `/lang=<code>`.
- Technical logs and the help of each command-line flag stay in English.

### Adding or improving a translation

Messages are written in English in the code (`i18n.T("...")` in Go,
`data-i18n` and `t("...")` in the web pages); English text is the key. Each
language is a JSON file in `internal/i18n/locales/<code>.json` mapping the
English text to its translation, with `{0}`, `{1}` placeholders that may be
reordered.

`go test ./internal/i18n` extracts every message from the sources and fails if
a language misses one, keeps one no longer used, or has different
placeholders. `I18N_DUMP=missing.json go test ./internal/i18n` writes the
list of missing messages per language. To add a language, add it to
`Languages` in `internal/i18n/i18n.go` and create its JSON file.

## License

The Android APK contains OpenVPN (GPL-2.0), so the APK as a whole is distributed under the GPL-2.0. The source of this project stays MIT, and the source of each APK is this repository at the tag of its release, with `android/openvpn/build.sh`. The licenses are in the app, under **Settings > Licenses**.

[MIT](LICENSE). The text appears in the program (Preferences > View license),
in the installer, in the `.exe` properties and ships in the package as
`LICENSE.txt`. Third-party components keep their own licenses: WinDivert
(LGPL-3.0 / GPL-2.0) is distributed unmodified, and the licenses of all
components ship in the package's `licenses\` folder.

The license has no effect on antivirus products. What reduces Windows warnings
is a **digital signature**: without it, whoever downloads the installer sees
SmartScreen ("Windows protected your PC" > More info > Run anyway), which
`README.txt` in the package explains. At the last check (definitions
1.463.84.0) Defender reported nothing in the package.

## Building

Needs Go 1.26 or newer (`go` downloads the toolchain by itself if needed).

```powershell
.\scripts\build.ps1          # builds vpn_specify.exe and vpn_specify-cli.exe in the root
.\scripts\build.ps1 -Test    # runs go vet and the tests first
.\scripts\build.ps1 -Dist    # also builds the package with the installer (see below)
.\scripts\build.ps1 -Icon    # regenerates the .exe icon/version (needs windres/MinGW)
.\scripts\build.ps1 -Linux   # also builds dist\vpn_specify-<version>-linux-{amd64,arm64}.tar.gz
```

On Linux itself:

```sh
./scripts/build.sh           # builds ./vpn_specify
./scripts/build.sh -test     # runs go vet and the tests first
sudo -E go test ./internal/tunnel   # includes the tun interface test (needs root)
```

`-Dist` builds `dist\vpn_specify-<version>-win64.zip`, with:

- `vpn_specify-setup.exe`: offline installer; the program and WinDivert are
  embedded (`go:embed`, `installer` build tag);
- `README.txt`: installation, requirements and where the data lives;
- `licenses\`: licenses of the third-party components (copied from the Go
  modules).

It needs the WinDivert files in `windivert\`. The version comes from the
`VERSION` file; after changing it, also run `-Icon` to update the version
shown in the `.exe` properties.

Or straight with `go`:

```powershell
go build -trimpath -ldflags "-s -w -H=windowsgui" -o vpn_specify.exe ./cmd/vpn_specify      # interface
go build -trimpath -ldflags "-s -w" -o vpn_specify-cli.exe ./cmd/vpn_specify                # terminal
go test ./...        # includes an end-to-end WireGuard test with an in-memory server
```

Development executables live in the root, in portable mode (with `data\` and
`windivert\` next to them).

The icon and version info are in the `rsrc_windows_amd64.syso` files of
`cmd\vpn_specify` and `cmd\installer`, which `go build` includes by itself.
Their source is in `assets\` (`icon-source.png`, `vpn_specify.ico` and the
`resource.rc.tmpl` template). To change the icon, regenerate the `.ico` and
run `.\scripts\build.ps1 -Icon`.

### Building the Android app

Needs Go, the Android SDK with NDK 27.1 (Android Studio installs them) and JDK 17
or newer. `.\scripts\build.ps1 -Android` writes
`dist\vpn_specify-<version>-android-arm64.apk`.

To publish it, sign with your own key: follow `android/keystore.properties.example`
once. Without `android/keystore.properties` the APK is signed with the debug key,
which is only for testing. **Keep the keystore and its password backed up**:
Android refuses an update signed with another key.

To run on an x86_64 emulator: `cd android; .\gradlew assembleDebug -Pabis=x86_64`.

OpenVPN inside the APK is `android/app/src/main/jniLibs/<abi>/libovpnexec.so`, built, unmodified,
from the official sources by `android/openvpn/build.sh` (Linux or WSL), which pins each version and
its SHA-256. Run it again only to change versions.

**License of the APK:** it contains OpenVPN (GPL-2.0), so the APK as a whole is distributed under the
GPL-2.0, while the code of this project stays MIT. Each release must make the corresponding source
available: this repository at the tag of that release, which includes the build script.

### Publishing a version

1. Change `VERSION` (e.g. `1.1.0`) and run `.\scripts\build.ps1 -Test -Icon -Dist -Linux -Android`.
2. Create the release with the tag **`v1.1.0`** (not marked as pre-release)
   and attach the files from `dist\` **with these names**, which the updater
   looks for:
   - `vpn_specify-1.1.0-win64.zip`
   - `vpn_specify-1.1.0-linux-amd64.tar.gz`
   - `vpn_specify-1.1.0-linux-arm64.tar.gz`
   - `vpn_specify-1.1.0-android-arm64.apk`

   A system without a package in the release simply gets no notice. That is
   why `v1.0.0`, which only has the Windows package, does not show up for
   Linux users.
3. The release text shows up in **What's new**.

## Project structure

```
cmd/vpn_specify/        entry point: GUI, up/run/status/update, autostart and uninstall
cmd/installer/          installer (vpn_specify-setup.exe)
cmd/libvpn_specify/     the Go library of the Android app (one JNI function)
android/                the Android app (Kotlin): VpnService, interface, updater
internal/
  mobile/               what the Android app calls in Go, as JSON
  ovpninstall/          installs OpenVPN when it is missing (Windows MSI, Linux packages)
  tunnel/               VPN connection: WireGuard (in process), OpenVPN, DNS
  proxy/                local SOCKS5 (TCP/UDP) and HTTP proxies, -L forwarding
  capture/              per-process capture with WinDivert (TCP and UDP)
  engine/               tunnel + proxies + capture in one connection with state
  store/                data folder: imported VPNs, apps, preferences
  paths/                where the data folder is (installed or portable)
  control/              local API between instances (shortcuts without a new UAC)
  launcher/             opens apps configured to use the proxy
  netcheck/             public IP check (TCP and UDP)
  update/               finds, downloads (with SHA-256) and installs new versions from GitHub
  autostart/            start with the system: scheduled task (Windows) or XDG autostart (Linux)
  i18n/                 translations (locales/*.json) and language detection
  gui/                  interface and agent: WebView2 (Windows) or local page + app window (Linux); page in gui/web/
  platform/             system APIs: UAC, DPAPI, shortcuts, Job Object (Windows); sudo, Pdeathsig (Linux)
  setup/                install/uninstall; window in setup/web/
  brand/                logo used by the app and the installer
  version/              version (set at build time from VERSION)
  textutil/             text helpers
docs/                   this README in other languages
license.go, LICENSE     MIT license (embedded in the executables)
assets/                 icon, resource (.rc) template and the package README
scripts/                build.ps1 (Windows) and build.sh (Linux)
dist/                   packages built by -Dist and -Linux (not versioned)
windivert/              WinDivert driver (downloaded separately, not versioned)
data/                   user data, created at run time (not versioned)
```

Package dependencies: `cmd → gui → engine → (tunnel, proxy, capture)`;
`store`, `control`, `launcher`, `netcheck`, `i18n` and `platform` are used on
top. The installer only uses `setup`, `platform`, `paths`, `i18n`, `brand`
and `version` (it does not load the VPN code).
