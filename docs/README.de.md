# vpn_specify

[English](../README.md) · [Português (Brasil)](README.pt-BR.md) · [Español](README.es.md) · [Français](README.fr.md) · **Deutsch** · [Русский](README.ru.md) · [简体中文](README.zh-CN.md) · [日本語](README.ja.md) · [हिन्दी](README.hi.md) · [العربية](README.ar.md)

Verbindet ein VPN **nur für die Apps, die du auswählst**. Der Rest des
Computers bleibt in deinem normalen Netzwerk.

```
            ┌──────────────── vpn_specify ────────────────┐
 chrome ──► │ SOCKS5-Proxy 127.0.0.1:10800 ┐               │
 curl   ──► │ HTTP-Proxy   127.0.0.1:10801 ┴──► VPN-Tunnel ┼──► VPN-Netzwerk
            └─────────────────────────────────────────────┘
 Rest des PCs ────────────────────────────────────────────────► normales Internet
```

Kein Hintergrunddienst. Unter Windows gibt es zwei Programmdateien aus
demselben Code:

- `vpn_specify.exe`: grafische Oberfläche (ohne Konsole).
- `vpn_specify-cli.exe`: die Terminal-Befehle (`up`, `run`, `status`, `update`).

Unter Linux gibt es eine einzige Programmdatei, `vpn_specify`, mit grafischer
Oberfläche und Terminal (siehe [Linux](#linux)).

Oberfläche, Installationsprogramm und Meldungen gibt es in **10 Sprachen**
(siehe [Sprachen](#sprachen)).

> Dies ist die Bedienungsanleitung. Zum Kompilieren, Veröffentlichen von
> Versionen und zur Code-Struktur siehe das [englische README](../README.md#building).

## Installation

Lade `vpn_specify-<Version>-win64.zip` aus den
[Releases](https://github.com/D3T3F/vpn_specify/releases) herunter, entpacke
es und starte `vpn_specify-setup.exe`:

- Wähle oben im Installationsfenster die Sprache; die App verwendet sie
  danach (änderbar später unter **Einstellungen**).
- Standardordner: `C:\Program Files\vpn_specify` (einen anderen über **Durchsuchen…** wählen).
- Optionen: Verknüpfungen auf dem Desktop und im Startmenü.
- Es wird nichts heruntergeladen: Programm und WinDivert stecken im Installationsprogramm.
- Ein erneuter Start des Installationsprogramms aktualisiert die vorhandene Installation (gleicher Ordner).
- Deinstallieren: Einstellungen > Apps > Installierte Apps > vpn_specify. Du
  entscheidest, ob die Daten gelöscht oder behalten werden. Von der App
  erstellte Verknüpfungen (z. B. `DiscordVPN`) werden ebenfalls entfernt.

Ohne Klicks (für Skripte):

```powershell
vpn_specify-setup.exe /silent [/dir=D:\Apps] [/nodesktop] [/nostartmenu] [/lang=de]
"C:\Program Files\vpn_specify\vpn_specify.exe" uninstall /silent [/keepdata]
```

Funktioniert auch ohne Installation (portabler Modus): Es genügt ein Ordner
`data\` (oder eine leere Datei namens `portable`) neben `vpn_specify.exe`.

## Grafische Oberfläche

Öffne `vpn_specify.exe`. Windows fragt nach Administratorrechten.

1. **VPNs > Datei importieren…**: Die `.ovpn`/`.conf` wird in den Datenordner
   kopiert. Verlangt das VPN eine Anmeldung, nutze **Anmeldedaten**: Das
   Passwort wird mit DPAPI gespeichert, nur dein Windows-Konto kann es lesen.
   Mit **Bearbeiten** kannst du das VPN umbenennen (die Apps, die es nutzen,
   ziehen mit) und den Dateiinhalt ändern; dieses VPN muss vorher getrennt
   werden.
2. **Apps im VPN > App hinzufügen…**: Wähle die `.exe` oder eine Startmenü-
   Verknüpfung (z. B. Discord). Bei Apps mit Updater (`Update.exe
   --processStart Discord.exe`) wird der richtige Prozess (`discord.exe`)
   automatisch erkannt.
   - *Vollständige Erfassung* (Standard): der gesamte Datenverkehr, inklusive Sprache/UDP (WinDivert).
   - *Proxy*: nur TCP, für Browser und Chromium-Apps; kein Treiber nötig.
3. **Verknüpfung erstellen**: Erstellt z. B. `DiscordVPN` im Startmenü (und
   auf Wunsch auf dem Desktop). Sie erscheint in der Windows-Suche neben dem
   originalen Discord.

Beim Öffnen einer Verknüpfung:

- Ist das VPN aus, erscheint die UAC einmal. Ein Fenster zeigt den
  Fortschritt, verbindet, öffnet die App und schließt sich von selbst.
- Ist das VPN schon an, öffnet sich die App sofort, ohne UAC.
- Die App öffnet sich als normaler Benutzer, nicht als Administrator.
- Ist „VPN trennen, wenn die darüber geöffneten Apps geschlossen werden"
  aktiviert und das Fenster geschlossen, schaltet sich das VPN etwa 30 s nach
  dem Schließen der Apps ab.

### Apps, die mit dem System starten

Aktiviere im App-Dialog (**App hinzufügen…** oder **Bearbeiten**) **Mit
Windows öffnen**. Bei der Anmeldung verbindet vpn_specify das VPN und öffnet
alle markierten Apps, ohne das Fenster zu zeigen. In der Liste tragen sie das
Abzeichen „startet mit dem System".

- **Keine UAC bei der Anmeldung**: vpn_specify erstellt eine geplante Aufgabe,
  `vpn_specify\Start apps in VPN` (in der Aufgabenplanung), die
  `vpn_specify.exe autostart` 15 s nach der Anmeldung ausführt, bereits mit
  Administratorrechten. Die Apps öffnen sich als normaler Benutzer.
- Unter Linux heißt die Option **Mit dem System öffnen**. Der Eintrag ist
  `~/.config/autostart/vpn_specify.desktop` und läuft 10 s nach der
  Anmeldung. OpenVPN-VPNs fragen dann nach dem Administratorpasswort.
- Es gibt nur einen Eintrag; er wird beim Markieren der ersten App erstellt
  und entfernt, wenn keine mehr markiert ist. Er wird bei jedem Start von
  vpn_specify aktualisiert und folgt so der Programmdatei, wenn sie
  verschoben oder aktualisiert wird.
- Es ist immer nur ein VPN aktiv, daher müssen die markierten Apps **dasselbe
  VPN** nutzen. Das Fenster warnt sonst.
- Direkt nach der Anmeldung kann das Netzwerk etwas brauchen. Jede App wird
  bis zu 4-mal mit Pausen versucht. Ein abgelehntes Passwort oder fehlende
  Anmeldedaten werden nicht wiederholt. Was sich nicht öffnen ließ, erscheint
  in einer Meldung.
- Das Deinstallationsprogramm entfernt die Aufgabe.

Es ist immer nur ein VPN aktiv. Erfasse keine allgemeinen Namen wie
`cmd.exe`: Jeder Prozess mit diesem Namen käme ins VPN.

Datenordner (Schaltfläche **Datenordner**): `configs\`, `settings.json` (Apps
und Einstellungen), `language` und `vpn_specify.log`. Er liegt bei einer
Installation in `%LOCALAPPDATA%\vpn_specify`, im portablen Modus in `data\`
neben der exe.

Starte die installierte Version und eine portable Kopie nicht gleichzeitig:
Beide nutzen dieselben lokalen Ports (10799–10801).

## Unterstützte Formate

| Konfiguration | Funktionsweise | Adminrechte? |
|---|---|---|
| **WireGuard** (`.conf`) | Der Tunnel läuft komplett im Prozess (wireguard-go + gVisor-TCP/IP-Stack). Es wird kein Netzwerkadapter erstellt. | Nein |
| **OpenVPN** (`.ovpn`) | Nutzt das installierte `openvpn`, blockiert aber alles, was Routing oder DNS des Systems ändern würde (redirect-gateway, Routen, DNS, block-outside-dns). Der Proxy bindet seine Sockets an den Tunnel, und genau das leitet den Verkehr durchs VPN. | Ja (OpenVPN erstellt einen Adapter) |

Fehlt OpenVPN, bietet vpn_specify beim Verbinden die Installation an: Unter Windows lädt es das offizielle Installationsprogramm herunter (geprüft mit einem festen SHA-256) und installiert es still, ohne die OpenVPN-Oberfläche; unter Linux installiert es das Paket `openvpn` der Distribution (apt, dnf, yum, zypper, pacman oder apk). Sie können es auch selbst installieren.

## Verwendung

```powershell
# 1. Verbinden (Terminal offen lassen; Strg+C trennt)
vpn_specify-cli up C:\vpns\arbeit.ovpn

# 2. In einem anderen Terminal Apps im VPN öffnen
vpn_specify-cli run chrome                       # separates Chrome, nur es im VPN
vpn_specify-cli run firefox
vpn_specify-cli run msedge https://intranet.firma
vpn_specify-cli run curl https://ifconfig.me
vpn_specify-cli run git clone https://git.intern/repo.git
vpn_specify-cli run powershell                   # Shell, deren Befehle den Proxy nutzen

# Verbinden und gleich eine App öffnen
vpn_specify-cli up wg0.conf -- chrome

# Prüfen: direkte IP vs. IP über das VPN, per TCP und UDP
vpn_specify-cli status
```

### Erfassung pro Prozess (`-capture`): ALLES einer App durchs VPN

Für Apps, die Proxys ignorieren oder eigenes UDP nutzen (Discord-Sprache,
Spiele, native Apps), fängt `-capture` die Pakete des Prozesses anhand des
Programmnamens ab. In der App muss nichts eingestellt werden:

```powershell
# Terminal als Administrator
vpn_specify-cli up arbeit.ovpn -capture discord.exe
# danach Discord ganz normal öffnen (Startmenü-Verknüpfung)
```

- **TCP** läuft über einen transparenten Proxy; **UDP** (Sprache, Video) über
  Sockets im VPN; das **IPv6** des Prozesses wird blockiert, damit die App
  IPv4 über das VPN nutzt.
- Andere Programme sind nicht betroffen, aber jedes ausgehende Paket läuft
  durch `vpn_specify`, was im Test etwa 10 % Geschwindigkeit kostete.
- War die App schon offen, werden ihre TCP-Verbindungen getrennt, damit sie
  sich über das VPN neu verbinden. Am sichersten ist es, die App nach `up` zu
  öffnen.
- Wiederholbar: `-capture discord.exe -capture steam.exe`.
- Funktioniert mit WireGuard und OpenVPN.
- Namen (DNS), die die App über Windows auflöst, nutzen weiterhin das normale
  DNS des PCs: Der DNS-Server sieht *welche* Domains, der Verkehr läuft aber
  durchs VPN.

Voraussetzungen: Administrator und die Dateien von [WinDivert 2.2.2](https://github.com/basil00/WinDivert/releases/tag/v2.2.2)
(`WinDivert.dll` und `WinDivert64.sys`, x64) in `.\windivert\` oder neben der
`.exe`. Windows Defender stuft WinDivert oft als unerwünscht ein, weil auch
Schadsoftware es nutzt; gib den Ordner unter Windows-Sicherheit > Ausschlüsse
frei. Während der Erfassung gibt es eine temporäre Firewall-Regel,
„vpn_specify capture", die beim Beenden entfernt wird.

### UDP

Es gibt zwei Wege:

1. **SOCKS5 UDP** (`127.0.0.1:10800`, UDP ASSOCIATE): für Apps, die UDP über
   SOCKS5 unterstützen.
2. **Portweiterleitung `-L`**: funktioniert mit **jedem** Programm, auch mit
   solchen, die keinen Proxy kennen. Der lokale Port wird zu einer Adresse im
   VPN:

```powershell
vpn_specify-cli up arbeit.ovpn -L udp:15353=10.0.0.5:53 -L tcp:13389=interner-server:3389
# jetzt: internes DNS unter 127.0.0.1:15353 und internes RDP unter 127.0.0.1:13389
mstsc /v:127.0.0.1:13389
```

`-L` akzeptiert `[tcp|udp:]lokalerPort=Host:Port` (oder eine lokale
`IP:Port`) und ist wiederholbar. Bei UDP erhält jede lokale Quelle ihre
eigene Sitzung; Sitzungen ohne Antwort für 2 Minuten werden geschlossen.

### Nützliche Optionen

- `up -v`: vollständiges Protokoll (auch das von OpenVPN).
- `up -dns 10.0.0.53`: erzwingt das DNS im VPN.
- `up -socks 127.0.0.1:20800 -http 127.0.0.1:20801`: andere Ports, z. B. für zwei VPNs gleichzeitig. Bei `run` dieselben Ports verwenden.
- `run -profile kunde2 chrome`: separates Browserprofil (Cookies, Anmeldungen).
- `run -chromium <Electron-App>`: erzwingt die Proxy-Flags von Chromium.
- `run -detach <Programm>`: wartet nicht auf das Ende des Programms.

## Wie jede App ins VPN kommt

- **Chrome / Edge / Brave / Vivaldi**: `--proxy-server=socks5://...`, ein
  eigenes Profil unter `%LOCALAPPDATA%\vpn_specify\profiles\` (sonst würde
  sich das Fenster im bereits laufenden Chrome öffnen, außerhalb des VPN),
  entferntes DNS und WebRTC nur über den Proxy.
- **Firefox**: eigenes Profil mit SOCKS5-Proxy, entferntem DNS und
  deaktiviertem DoH.
- **Jedes andere Programm**: die Variablen `HTTP_PROXY`, `HTTPS_PROXY` und
  `ALL_PROXY`. Funktioniert mit curl, git, npm, pip, Go, .NET HttpClient und
  vielen weiteren.

Namen (DNS) werden **im VPN** aufgelöst, daher funktionieren interne Namen
(`intranet.firma`) und gelangen nicht zum DNS deines Netzwerks.

## Einschränkungen

- **Apps, die Proxys ignorieren**: `-capture` verwenden (oder `-L` für feste Adressen).
- Chrome und Firefox senden kein UDP (QUIC/HTTP3) über SOCKS; sie nutzen TCP,
  was normal funktioniert.
- UDP und der OpenVPN-Modus funktionieren nur mit **IPv4**.
- Wird `vpn_specify` beendet, endet `openvpn` mit (Job Object unter Windows,
  SIGTERM unter Linux), aber die Route mit Metrik 9999 kann am getrennten
  TAP-Adapter bleiben. Sie wirkt sich nicht aus; ein normales Strg+C entfernt
  alles.
- Der Proxy lauscht nur auf `127.0.0.1`, aber **jedes Programm auf deinem PC**
  kann ihn nutzen, solange `up` läuft.

## Linux

Unter Linux funktionieren die grafische Oberfläche und die Terminal-Befehle
(`up`, `run`, `status`, `update` und `-L`), mit WireGuard und OpenVPN, auf
denselben Ports wie unter Windows. Installationsprogramm und `-capture` gibt
es weiterhin nur unter Windows.

```sh
tar -xzf vpn_specify-<Version>-linux-amd64.tar.gz   # oder -linux-arm64
sudo install vpn_specify-<Version>-linux-amd64/vpn_specify /usr/local/bin/
vpn_specify                                # öffnet die grafische Oberfläche
```

### Grafische Oberfläche unter Linux

Starte `vpn_specify` ohne Argumente. Nutze beim ersten Mal **Einstellungen >
„vpn_specify" zum Anwendungsmenü hinzufügen**, um das Symbol im Menü zu
haben. Das Fenster ist dasselbe wie unter Windows, mit diesen Unterschieden:

- **Fenster**: Es ist dieselbe Seite wie unter Windows, bereitgestellt von
  vpn_specify selbst auf `127.0.0.1` (zufälliger Port, geschützt durch ein
  zufälliges Token). Sie öffnet sich als App-Fenster (ohne Tabs und
  Adressleiste) von Chrome, Chromium, Edge, Brave oder Vivaldi, mit einem
  eigenen Profil. Ohne diese öffnet sie sich als Tab im Standardbrowser.
  Funktioniert mit X11 und Wayland.
- **Ohne root**: Die Oberfläche läuft als normaler Benutzer. Beim Verbinden
  eines **OpenVPN**-VPN fragt das System nach dem Administratorpasswort
  (`pkexec` von polkit), nur für `openvpn`. Dieses läuft in einem kleinen
  Hilfsprozess, der es beim Trennen oder Schließen von vpn_specify beendet.
  Sockets ohne root an den Tunnel zu binden, erfordert Kernel 5.7 oder neuer.
  WireGuard fragt nach keinem Passwort.
- **App hinzufügen** listet die Apps des Anwendungsmenüs (`.desktop`-Dateien,
  auch flatpak und snap) mit Suche. **Datei auswählen…** nutzt zenity oder
  kdialog; ohne sie gib den Pfad im Feld Programm ein.
- **Nur Proxy-Modus**: Es gibt keine vollständige Erfassung. Browser,
  Chromium/Electron-Apps (Discord, Slack, VS Code…) und Programme, die
  `HTTP_PROXY` beachten, kommen ins VPN. Discord-Sprache/UDP und Spiele nicht.
- **Verknüpfung erstellen** schreibt eine `.desktop`-Datei in
  `~/.local/share/applications/` (erscheint in der Menüsuche neben der
  originalen App) und, falls aktiviert, auf den Desktop.
- **Anmeldedaten**: Das Passwort wird (AES-GCM) mit einem Schlüssel in
  `~/.local/share/vpn_specify/secret.key` (Rechte 0600) verschlüsselt,
  getrennt von `settings.json`. Geschützt ist es durch die Dateirechte, wie
  ein SSH-Schlüssel, nicht durch den Schlüsselbund des Systems. Unter Windows
  gespeicherte Passwörter (DPAPI) müssen neu eingegeben werden.
- **Daten**: `~/.config/vpn_specify/` (`configs/`, `settings.json`,
  `language`, `vpn_specify.log`).

Der Ablauf ist derselbe wie unter Windows. Beim Öffnen einer Verknüpfung mit
ausgeschaltetem VPN zeigt das Fenster den Fortschritt, verbindet, öffnet die
App und schließt sich. Bei geschlossenem Fenster schaltet sich das VPN etwa
30 s nach dem Schließen der darüber geöffneten Apps ab.

### Terminal unter Linux

```sh
vpn_specify up wg0.conf                    # WireGuard: ohne root
sudo vpn_specify up arbeit.ovpn            # OpenVPN: braucht root und das Paket openvpn

# in einem anderen Terminal, als normaler Benutzer
vpn_specify run chrome                     # findet google-chrome-stable, chromium...
vpn_specify run firefox
vpn_specify run curl https://ifconfig.me
vpn_specify status
```

Unterschiede zu Windows:

- **OpenVPN**: Unter Linux reicht es nicht, den Socket an die Tunnel-IP zu
  binden, damit er über den Tunnel hinausgeht. Deshalb bindet der Proxy seine
  Sockets mit `SO_BINDTODEVICE` an die `tun`-Schnittstelle und setzt deren
  `rp_filter` auf *loose*. Ohne das würden Distributionen mit striktem
  `rp_filter` (Fedora, RHEL) die Antworten verwerfen. Routen und DNS des
  Systems bleiben unverändert, wie unter Windows. Wird `vpn_specify` beendet,
  erhält `openvpn` SIGTERM und beendet sich ebenfalls.
- **`sudo vpn_specify up ... -- Programm`**: Das Programm wird als der
  Benutzer geöffnet, der `sudo` aufgerufen hat, nicht als root. Das
  Browserprofil liegt in dessen Home-Verzeichnis. Am einfachsten ist es
  trotzdem, `run` in einem anderen Terminal ohne `sudo` zu nutzen.
- **Browserprofile**: in `~/.config/vpn_specify/profiles/`. Als Snap
  installierte Firefox- und Chromium-Versionen (Standard unter Ubuntu) können
  keine versteckten Ordner im Home lesen; für sie liegt das Profil in
  `~/snap/<app>/common/vpn_specify/profiles/`.
- **Apps, die Proxys ignorieren**: ohne `-capture` `-L` für feste Adressen
  verwenden.

## Android

Installieren Sie `vpn_specify-<Version>-android-arm64.apk` (Android 8 oder neuer;
erlauben Sie bei Nachfrage die Installation aus dieser Quelle). Android hat VPN pro
App eingebaut, daher nutzt die App keinen Proxy: Der gesamte TCP- und UDP-Verkehr
der gewählten Apps, auch Sprache, läuft durch den Tunnel, ohne Root. Der Rest des
Telefons bleibt in Ihrem normalen Netzwerk.

1. **Datei importieren…**: Wählen Sie eine WireGuard-`.conf` oder eine OpenVPN-`.ovpn`.
2. **Apps auswählen…**: Markieren Sie die Apps, die das VPN nutzen, zum Beispiel Discord.
3. **Verbinden**, und bestätigen Sie beim ersten Mal die VPN-Berechtigung von Android.

Android kann nicht erkennen, wann eine App geöffnet wird. Daher gibt es zwei Wege,
wie eine App das VPN nutzt:

- **VPN eingeschaltet lassen** (empfohlen): Die App ist immer im VPN, egal wie sie
  geöffnet wird, und sonst nichts. **Beim Start des Telefons verbinden** erledigt das
  bei jedem Start. In den VPN-Einstellungen des Systems sorgt **Durchgehend aktives
  VPN** für den automatischen Neustart durch Android, und **Verbindungen ohne VPN
  blockieren** verhindert, dass die gewählten Apps außerhalb davon kommunizieren
  (das blockiert auch den Rest des Telefons).
- **Verknüpfung**: Erstellt ein Symbol „Discord (VPN)“ auf dem Startbildschirm, das
  verbindet und dann die App öffnet. Das normale Symbol schaltet das VPN nicht ein.

Einschränkungen unter Android:

- OpenVPN: Zertifikate und Schlüssel müssen in der `.ovpn` enthalten sein, TAP wird nicht unterstützt und ein privater Schlüssel mit Passwort noch nicht. Direktiven, die Programme starten würden, werden entfernt.
- Android hält nur ein VPN aktiv: Ein anderes zu starten schaltet dieses aus.
- Wenn Sie Apps oder VPN während der Verbindung ändern, wird der Tunnel neu
  erstellt, was die gewählten Apps kurz unterbricht.

Die Sprache folgt dem System und lässt sich unter **Einstellungen** ändern. Die App
sucht auf GitHub nach neuen Versionen, und Android fragt bei jedem Update nach.

## Updates

Beim Öffnen der Oberfläche prüft vpn_specify die neueste Version in den
[GitHub-Releases](https://github.com/D3T3F/vpn_specify/releases). Gibt es eine
neuere, erscheint oben im Fenster ein Hinweis:

- **Aktualisieren**: lädt das Paket herunter, prüft den SHA-256, trennt,
  schließt sich, installiert und öffnet sich in der neuen Version wieder.
- **Nicht jetzt**: blendet den Hinweis bis zum nächsten Start aus.
- **Diese Version überspringen**: keine Hinweise mehr zu dieser Version, nur
  zur nächsten.
- **Neuigkeiten**: zeigt den Text des Releases.

Unter **Einstellungen** lässt sich die Prüfung beim Start abschalten und
sofort prüfen (**Nach Updates suchen**). Im Terminal:

```sh
vpn_specify update           # zeigt die neue Version und fragt, ob sie installiert werden soll
vpn_specify update -check    # prüft nur
vpn_specify update -y        # installiert ohne Rückfrage
vpn_specify version
```

So wird die neue Version installiert:

- **Windows, installiert**: Das `vpn_specify-setup.exe` des Releases läuft im
  Update-Modus. Es nutzt denselben Ordner, behält vorhandene Verknüpfungen
  und fragt bei Bedarf nach der UAC.
- **Windows, portabel**: Dasselbe Installationsprogramm ersetzt nur die
  Dateien des Ordners, ohne Registry-Einträge oder Verknüpfungen.
- **Linux**: Die Programmdatei wird an Ort und Stelle ersetzt. Erfordert der
  Ordner root (z. B. `/usr/local/bin`), fragt das System nach dem Passwort
  (`pkexec`). Vor dem Ersetzen prüft vpn_specify, dass die neue Programmdatei
  läuft und die richtige Version meldet.

Sicherheit: Ein Download wird nur von
`github.com/D3T3F/vpn_specify/releases/download/` akzeptiert und nur, wenn
sein SHA-256 mit dem übereinstimmt, den GitHub für die Datei angibt (oder mit
einer `SHA256SUMS`-Datei im Release). Ohne Hash wird nichts installiert. Das
schützt vor einem unterwegs beschädigten oder manipulierten Download, nicht
vor jemandem mit Zugriff auf das GitHub-Konto; dafür müssten die Pakete
signiert werden. Builds ohne Version (`dev`) suchen nie nach Updates.

## Sprachen

Oberfläche, Installationsprogramm, Meldungen und Terminal-Hilfe gibt es auf:
English, Português (Brasil), Español, Français, Deutsch, Русский, 简体中文,
日本語, हिन्दी und العربية (mit Layout von rechts nach links).

- **Standard**: die Systemsprache, sofern sie dabei ist (Anzeigesprache von
  Windows; unter Linux `LANGUAGE`, `LC_ALL`, `LC_MESSAGES` oder `LANG`),
  sonst Englisch.
- **Auswählen**: oben im Windows-Installationsprogramm oder später unter
  **Einstellungen > Sprache** (das Fenster wechselt sofort). Die Wahl wird im
  Datenordner gespeichert (Datei `language`) und gilt auch für die
  Terminal-Befehle. Die stille Installation akzeptiert `/lang=<Code>`.
- Technische Protokolle und die Hilfe zu den einzelnen Terminal-Optionen
  bleiben auf Englisch.

Möchtest du eine Übersetzung verbessern oder eine Sprache hinzufügen? Siehe
[Adding or improving a translation](../README.md#adding-or-improving-a-translation).

## Lizenz

Das Android-APK enthält OpenVPN (GPL-2.0), daher wird das APK als Ganzes unter der GPL-2.0 verteilt. Der Code dieses Projekts bleibt MIT, und der Quellcode jedes APK ist dieses Repository beim Tag der jeweiligen Version, mit `android/openvpn/build.sh`. Die Lizenzen finden Sie in der App unter **Einstellungen > Lizenzen**.

[MIT](../LICENSE). Der Text erscheint im Programm (Einstellungen > Lizenz
anzeigen), im Installationsprogramm, in den Eigenschaften der `.exe` und liegt
dem Paket als `LICENSE.txt` bei. Komponenten von Drittanbietern behalten ihre
eigenen Lizenzen: WinDivert (LGPL-3.0 / GPL-2.0) wird unverändert
weitergegeben, und die Lizenzen aller Komponenten liegen im Ordner
`licenses\` des Pakets.

Das Installationsprogramm ist nicht digital signiert: Beim Öffnen kann
Windows SmartScreen anzeigen („Der Computer wurde durch Windows geschützt" >
Weitere Informationen > Trotzdem ausführen), wie die `README.txt` im Paket
erklärt.
