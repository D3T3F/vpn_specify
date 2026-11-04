# vpn_specify

[English](../README.md) · [Português (Brasil)](README.pt-BR.md) · [Español](README.es.md) · **Français** · [Deutsch](README.de.md) · [Русский](README.ru.md) · [简体中文](README.zh-CN.md) · [日本語](README.ja.md) · [हिन्दी](README.hi.md) · [العربية](README.ar.md)

Se connecte à un VPN **uniquement pour les apps que vous choisissez**. Le reste
de l’ordinateur reste sur votre réseau normal.

```
            ┌──────────────── vpn_specify ────────────────┐
 chrome ──► │ proxy SOCKS5 127.0.0.1:10800 ┐               │
 curl   ──► │ proxy HTTP   127.0.0.1:10801 ┴──► tunnel VPN ┼──► réseau du VPN
            └─────────────────────────────────────────────┘
 reste du PC ─────────────────────────────────────────────────► internet normal
```

Aucun service en arrière-plan. Sous Windows, il y a deux exécutables issus du
même code :

- `vpn_specify.exe` : interface graphique (sans console).
- `vpn_specify-cli.exe` : les commandes du terminal (`up`, `run`, `status`, `update`).

Sous Linux, il n’y a qu’un exécutable, `vpn_specify`, avec l’interface
graphique et le terminal (voir [Linux](#linux)).

L’interface, le programme d’installation et les messages sont disponibles en
**10 langues** (voir [Langues](#langues)).

> Ceci est le guide d’utilisation. Pour compiler, publier des versions et
> découvrir la structure du code, voir le [README en anglais](../README.md#building).

## Installation

Téléchargez `vpn_specify-<version>-win64.zip` depuis les
[releases](https://github.com/D3T3F/vpn_specify/releases), extrayez-le et
lancez `vpn_specify-setup.exe` :

- Choisissez la langue en haut de la fenêtre d’installation ; l’app
  l’utilisera ensuite (vous pourrez la changer dans **Préférences**).
- Dossier par défaut : `C:\Program Files\vpn_specify` (choisissez-en un autre avec **Parcourir…**).
- Options : raccourcis sur le bureau et dans le menu Démarrer.
- Rien n’est téléchargé : le programme et WinDivert sont dans le programme d’installation.
- Relancer le programme d’installation met à jour l’installation existante (même dossier).
- Désinstaller : Paramètres > Applications > Applications installées >
  vpn_specify. Vous choisissez de supprimer ou de garder les données. Les
  raccourcis créés par l’app (ex. `DiscordVPN`) sont aussi supprimés.

Sans clic (pour les scripts) :

```powershell
vpn_specify-setup.exe /silent [/dir=D:\Apps] [/nodesktop] [/nostartmenu] [/lang=fr]
"C:\Program Files\vpn_specify\vpn_specify.exe" uninstall /silent [/keepdata]
```

Fonctionne aussi sans installation (mode portable) : il suffit d’un dossier
`data\` (ou d’un fichier vide nommé `portable`) à côté de `vpn_specify.exe`.

## Interface graphique

Ouvrez `vpn_specify.exe`. Windows demande l’autorisation d’administrateur.

1. **VPN > Importer un fichier…** : le `.ovpn`/`.conf` est copié dans le
   dossier de données. Si le VPN demande une connexion, utilisez
   **Identifiants** : le mot de passe est stocké avec DPAPI, seul votre compte
   Windows peut le lire. **Modifier** permet de renommer le VPN (les apps qui
   l’utilisent suivent) et de changer le contenu du fichier ; ce VPN doit
   d’abord être déconnecté.
2. **Apps dans le VPN > Ajouter une app…** : choisissez le `.exe` ou un
   raccourci du menu Démarrer (ex. Discord). Pour les apps avec mise à jour
   intégrée (`Update.exe --processStart Discord.exe`), le bon processus
   (`discord.exe`) est détecté automatiquement.
   - *Capture complète* (par défaut) : tout le trafic, y compris voix/UDP (WinDivert).
   - *Proxy* : TCP uniquement, pour les navigateurs et les apps Chromium ; aucun pilote requis.
3. **Créer un raccourci** : crée par exemple `DiscordVPN` dans le menu
   Démarrer (et, si vous le souhaitez, sur le bureau). Il apparaît dans la
   recherche Windows à côté du Discord d’origine.

À l’ouverture d’un raccourci :

- Si le VPN n’est pas actif, l’UAC apparaît une fois. Une fenêtre affiche la
  progression, se connecte, ouvre l’app et se ferme toute seule.
- Si le VPN est déjà actif, l’app s’ouvre aussitôt, sans UAC.
- L’app s’ouvre en tant qu’utilisateur normal, pas en administrateur.
- Avec « Déconnecter le VPN quand les apps ouvertes avec lui sont fermées »
  coché et la fenêtre fermée, le VPN se coupe environ 30 s après la fermeture
  des apps.

### Apps qui démarrent avec le système

Dans la fenêtre de l’app (**Ajouter une app…** ou **Modifier**), cochez
**Ouvrir avec Windows**. À l’ouverture de session, vpn_specify connecte le
VPN et ouvre toutes les apps cochées, sans afficher la fenêtre. Dans la
liste, elles portent l’étiquette « démarre avec le système ».

- **Pas d’UAC à l’ouverture de session** : vpn_specify crée une tâche
  planifiée, `vpn_specify\Start apps in VPN` (dans le Planificateur de
  tâches), qui lance `vpn_specify.exe autostart` 15 s après l’ouverture de
  session, déjà avec les privilèges d’administrateur. Les apps s’ouvrent en
  utilisateur normal.
- Sous Linux, l’option est **Ouvrir avec le système**. L’entrée est
  `~/.config/autostart/vpn_specify.desktop` et s’exécute 10 s après
  l’ouverture de session. Les VPN OpenVPN demandent alors le mot de passe
  administrateur.
- Il n’y a qu’une entrée, créée quand la première app est cochée et supprimée
  quand il n’y en a plus. Elle est actualisée à chaque démarrage de
  vpn_specify, elle suit donc l’exécutable s’il est déplacé ou mis à jour.
- Un seul VPN est actif à la fois : les apps cochées doivent utiliser le
  **même VPN**. La fenêtre vous prévient sinon.
- Juste après l’ouverture de session, le réseau peut tarder. Chaque app est
  tentée jusqu’à 4 fois, avec des pauses. Un mot de passe refusé ou des
  identifiants manquants ne sont pas retentés. Ce qui ne s’ouvre pas est
  indiqué dans un message.
- Le programme de désinstallation supprime la tâche.

Un seul VPN est actif à la fois. Évitez de capturer des noms génériques comme
`cmd.exe` : tous les processus de ce nom entreraient dans le VPN.

Dossier de données (bouton **Dossier de données**) : `configs\`,
`settings.json` (apps et préférences), `language` et `vpn_specify.log`. Il se
trouve dans `%LOCALAPPDATA%\vpn_specify` une fois installé, ou dans `data\` à
côté de l’exe en mode portable.

N’exécutez pas la version installée et une copie portable en même temps :
toutes deux utilisent les mêmes ports locaux (10799–10801).

## Formats pris en charge

| Config | Fonctionnement | Droits admin ? |
|---|---|---|
| **WireGuard** (`.conf`) | Le tunnel tourne entièrement dans le processus (wireguard-go + pile TCP/IP gVisor). Aucune carte réseau n’est créée. | Non |
| **OpenVPN** (`.ovpn`) | Utilise l’`openvpn` installé, mais bloque tout ce qui modifierait le routage ou le DNS du système (redirect-gateway, routes, DNS, block-outside-dns). Le proxy lie ses sockets au tunnel, c’est ce qui fait passer le trafic par le VPN. | Oui (OpenVPN crée une carte) |

Si OpenVPN manque, vpn_specify propose de l'installer à la connexion : sous Windows, il télécharge le programme d'installation officiel (vérifié avec un SHA-256 fixe) et l'installe silencieusement, sans l'interface d'OpenVPN ; sous Linux, il installe le paquet `openvpn` de la distribution (apt, dnf, yum, zypper, pacman ou apk). Vous pouvez aussi l'installer vous-même.

## Utilisation

```powershell
# 1. Se connecter (gardez ce terminal ouvert ; Ctrl+C déconnecte)
vpn_specify-cli up C:\vpns\travail.ovpn

# 2. Dans un autre terminal, ouvrir des apps dans le VPN
vpn_specify-cli run chrome                       # Chrome séparé, seul lui dans le VPN
vpn_specify-cli run firefox
vpn_specify-cli run msedge https://intranet.entreprise
vpn_specify-cli run curl https://ifconfig.me
vpn_specify-cli run git clone https://git.interne/depot.git
vpn_specify-cli run powershell                   # shell dont les commandes utilisent le proxy

# Se connecter et ouvrir une app aussitôt
vpn_specify-cli up wg0.conf -- chrome

# Vérifier : IP directe ou via le VPN, en TCP et en UDP
vpn_specify-cli status
```

### Capture par processus (`-capture`) : TOUT le trafic d’une app via le VPN

Pour les apps qui ignorent le proxy ou utilisent leur propre UDP (voix
Discord, jeux, apps natives), `-capture` intercepte les paquets du processus
d’après le nom de l’exécutable. Rien à configurer dans l’app :

```powershell
# terminal en administrateur
vpn_specify-cli up travail.ovpn -capture discord.exe
# puis ouvrez Discord normalement (raccourci du menu Démarrer)
```

- **TCP** passe par un proxy transparent ; **UDP** (voix, vidéo) par des
  sockets dans le VPN ; l’**IPv6** du processus est bloqué pour que l’app
  utilise IPv4 via le VPN.
- Les autres programmes ne sont pas affectés, mais chaque paquet sortant passe
  par `vpn_specify`, ce qui a coûté environ 10 % de débit lors des tests.
- Si l’app était déjà ouverte, ses connexions TCP sont coupées pour se
  reconnecter via le VPN. Le plus sûr est d’ouvrir l’app après le `up`.
- Répétable : `-capture discord.exe -capture steam.exe`.
- Fonctionne avec WireGuard et OpenVPN.
- Les noms (DNS) que l’app résout via Windows utilisent toujours le DNS normal
  du PC : le serveur DNS voit *quels* domaines, mais le trafic passe par le VPN.

Prérequis : administrateur et les fichiers de [WinDivert 2.2.2](https://github.com/basil00/WinDivert/releases/tag/v2.2.2)
(`WinDivert.dll` et `WinDivert64.sys`, x64) dans `.\windivert\` ou à côté du
`.exe`. Windows Defender signale souvent WinDivert comme indésirable, car des
logiciels malveillants l’utilisent aussi ; autorisez le dossier dans Sécurité
Windows > Exclusions. Pendant la capture, une règle de pare-feu temporaire,
« vpn_specify capture », est supprimée à la sortie.

### UDP

Deux possibilités :

1. **SOCKS5 UDP** (`127.0.0.1:10800`, UDP ASSOCIATE) : pour les apps qui
   prennent en charge l’UDP via SOCKS5.
2. **Redirection de port `-L`** : fonctionne avec **n’importe quel**
   programme, même ceux qui ne savent pas utiliser un proxy. Le port local
   devient une adresse dans le VPN :

```powershell
vpn_specify-cli up travail.ovpn -L udp:15353=10.0.0.5:53 -L tcp:13389=serveur-interne:3389
# désormais : DNS interne sur 127.0.0.1:15353 et RDP interne sur 127.0.0.1:13389
mstsc /v:127.0.0.1:13389
```

`-L` accepte `[tcp|udp:]portLocal=hôte:port` (ou un `ip:port` local) et est
répétable. En UDP, chaque source locale a sa propre session ; les sessions
sans réponse pendant 2 minutes sont fermées.

### Options utiles

- `up -v` : journal complet (y compris celui d’OpenVPN).
- `up -dns 10.0.0.53` : impose le DNS utilisé dans le VPN.
- `up -socks 127.0.0.1:20800 -http 127.0.0.1:20801` : d’autres ports, par exemple pour deux VPN à la fois. Utilisez les mêmes ports avec `run`.
- `run -profile client2 chrome` : profil de navigateur séparé (cookies, sessions).
- `run -chromium <app Electron>` : impose les options de proxy de Chromium.
- `run -detach <programme>` : n’attend pas la fin du programme.

## Comment chaque app entre dans le VPN

- **Chrome / Edge / Brave / Vivaldi** : `--proxy-server=socks5://...`, un
  profil dédié dans `%LOCALAPPDATA%\vpn_specify\profiles\` (sinon la fenêtre
  s’ouvrirait dans le Chrome déjà lancé, hors du VPN), DNS distant et WebRTC
  uniquement via le proxy.
- **Firefox** : profil dédié avec proxy SOCKS5, DNS distant et DoH désactivé.
- **Tout autre programme** : les variables `HTTP_PROXY`, `HTTPS_PROXY` et
  `ALL_PROXY`. Fonctionne avec curl, git, npm, pip, Go, .NET HttpClient et
  bien d’autres.

Les noms (DNS) sont résolus **dans le VPN** : les noms internes
(`intranet.entreprise`) fonctionnent et ne fuient pas vers le DNS de votre
réseau.

## Limites

- **Apps qui ignorent le proxy** : utilisez `-capture` (ou `-L` pour des adresses fixes).
- Chrome et Firefox n’envoient pas d’UDP (QUIC/HTTP3) via SOCKS ; ils
  utilisent TCP, qui fonctionne normalement.
- L’UDP et le mode OpenVPN ne fonctionnent qu’en **IPv4**.
- Si `vpn_specify` est tué, `openvpn` s’arrête avec lui (Job Object sous
  Windows, SIGTERM sous Linux), mais la route de métrique 9999 peut rester
  sur la carte TAP déconnectée. Elle n’a aucun effet ; un Ctrl+C normal
  supprime tout.
- Le proxy n’écoute que sur `127.0.0.1`, mais **n’importe quel programme de
  votre PC** peut l’utiliser tant que `up` est lancé.

## Linux

Sous Linux, l’interface graphique et les commandes du terminal (`up`, `run`,
`status`, `update` et `-L`) fonctionnent, avec WireGuard et OpenVPN, sur les
mêmes ports que sous Windows. Le programme d’installation et `-capture`
restent réservés à Windows.

```sh
tar -xzf vpn_specify-<version>-linux-amd64.tar.gz   # ou -linux-arm64
sudo install vpn_specify-<version>-linux-amd64/vpn_specify /usr/local/bin/
vpn_specify                                # ouvre l’interface graphique
```

### Interface graphique sous Linux

Lancez `vpn_specify` sans argument. La première fois, utilisez **Préférences
> Ajouter « vpn_specify » au menu des applications** pour avoir l’icône dans
le menu. La fenêtre est la même que sous Windows, avec ces différences :

- **Fenêtre** : c’est la même page que sous Windows, servie par vpn_specify
  lui-même sur `127.0.0.1` (port aléatoire, protégé par un jeton aléatoire).
  Elle s’ouvre comme fenêtre d’application (sans onglets ni barre d’adresse)
  de Chrome, Chromium, Edge, Brave ou Vivaldi, avec un profil séparé. Sans
  aucun d’eux, elle s’ouvre dans un onglet du navigateur par défaut.
  Fonctionne avec X11 et Wayland.
- **Sans root** : l’interface tourne en utilisateur normal. Lors de la
  connexion d’un VPN **OpenVPN**, le système demande le mot de passe
  administrateur (`pkexec`, de polkit) uniquement pour `openvpn`. Celui-ci
  tourne dans un petit processus auxiliaire qui l’arrête quand vous vous
  déconnectez ou fermez vpn_specify. Lier les sockets au tunnel sans root
  nécessite le noyau 5.7 ou plus récent. WireGuard ne demande aucun mot de
  passe.
- **Ajouter une app** liste les apps du menu des applications (fichiers
  `.desktop`, y compris flatpak et snap), avec recherche. **Choisir un
  fichier…** utilise zenity ou kdialog ; sans eux, saisissez le chemin dans le
  champ Programme.
- **Mode proxy uniquement** : pas de capture complète. Les navigateurs, les
  apps Chromium/Electron (Discord, Slack, VS Code…) et les programmes qui
  respectent `HTTP_PROXY` entrent dans le VPN. La voix/UDP de Discord et les
  jeux, non.
- **Créer un raccourci** écrit un `.desktop` dans
  `~/.local/share/applications/` (visible dans la recherche du menu, à côté
  de l’app d’origine) et, si coché, sur le bureau.
- **Identifiants** : le mot de passe est chiffré (AES-GCM) avec une clé dans
  `~/.local/share/vpn_specify/secret.key` (droits 0600), séparée de
  `settings.json`. Il est protégé par les droits du fichier, comme une clé
  SSH, pas par le trousseau du système. Les mots de passe enregistrés sous
  Windows (DPAPI) doivent être ressaisis.
- **Données** : `~/.config/vpn_specify/` (`configs/`, `settings.json`,
  `language`, `vpn_specify.log`).

Le cycle de vie est le même que sous Windows. Ouvrir un raccourci avec le VPN
éteint affiche la progression, connecte, ouvre l’app et ferme la fenêtre.
Fenêtre fermée, le VPN se coupe environ 30 s après la fermeture des apps
ouvertes avec lui.

### Terminal sous Linux

```sh
vpn_specify up wg0.conf                    # WireGuard : sans root
sudo vpn_specify up travail.ovpn           # OpenVPN : nécessite root et le paquet openvpn

# dans un autre terminal, en utilisateur normal
vpn_specify run chrome                     # trouve google-chrome-stable, chromium...
vpn_specify run firefox
vpn_specify run curl https://ifconfig.me
vpn_specify status
```

Différences avec Windows :

- **OpenVPN** : sous Linux, lier le socket à l’IP du tunnel ne suffit pas
  pour qu’il sorte par le tunnel. Le proxy lie donc ses sockets à
  l’interface `tun` avec `SO_BINDTODEVICE` et met le `rp_filter` de cette
  interface en mode *loose*. Sans cela, les distributions avec un
  `rp_filter` strict (Fedora, RHEL) rejetteraient les réponses. Les routes et
  le DNS du système restent intacts, comme sous Windows. Si `vpn_specify` est
  tué, `openvpn` reçoit SIGTERM et s’arrête aussi.
- **`sudo vpn_specify up ... -- programme`** : le programme est ouvert en tant
  que l’utilisateur qui a lancé `sudo`, pas en root. Le profil du navigateur
  va dans son dossier personnel. Le plus simple reste toutefois d’utiliser
  `run` dans un autre terminal, sans `sudo`.
- **Profils de navigateur** : dans `~/.config/vpn_specify/profiles/`. Firefox
  et Chromium installés en snap (par défaut sous Ubuntu) ne peuvent pas lire
  les dossiers cachés du dossier personnel ; pour eux, le profil va dans
  `~/snap/<app>/common/vpn_specify/profiles/`.
- **Apps qui ignorent le proxy** : sans `-capture`, utilisez `-L` pour des
  adresses fixes.

## Android

Installez `vpn_specify-<version>-android-arm64.apk` (Android 8 ou plus récent ;
autorisez l'installation depuis cette source si demandé). Android intègre le VPN
par application : l'app n'utilise donc pas de proxy. Tout le trafic TCP et UDP des
applications choisies, voix comprise, passe par le tunnel, sans root. Le reste du
téléphone reste sur votre réseau normal.

1. **Importer un fichier…** : choisissez un `.conf` WireGuard ou un `.ovpn` OpenVPN.
2. **Choisir les applications…** : cochez celles qui utilisent le VPN, Discord par exemple.
3. **Connecter**, et acceptez l'autorisation VPN d'Android la première fois.

Android ne peut pas détecter l'ouverture d'une application ; il y a donc deux façons
de faire utiliser le VPN à une application :

- **Laisser le VPN activé** (recommandé) : l'application est toujours dans le VPN,
  quelle que soit la façon de l'ouvrir, et rien d'autre ne l'est. **Se connecter au
  démarrage du téléphone** le fait à chaque démarrage. Dans les paramètres VPN du
  système, **VPN permanent** le fait redémarrer par Android, et **Bloquer les
  connexions sans VPN** empêche les applications choisies de sortir en dehors (cela
  bloque aussi le reste du téléphone).
- **Raccourci** : crée une icône « Discord (VPN) » sur l'écran d'accueil, qui connecte
  puis ouvre l'application. L'icône normale n'active pas le VPN.

Limites sur Android :

- Une application choisie ne sort jamais en dehors du VPN. Si le VPN n'atteint qu'un réseau interne, les sites internet ne s'ouvriront pas dans cette application.
- OpenVPN : les certificats et clés doivent être dans le `.ovpn`, TAP n'est pas pris en charge et une clé privée avec mot de passe ne l'est pas encore. Les directives qui lanceraient des programmes sont supprimées.
- Android ne garde qu'un seul VPN actif : en lancer un autre désactive celui-ci.
- Changer les applications ou le VPN pendant la connexion recrée le tunnel, ce qui
  interrompt un instant les applications choisies.

La langue suit celle du système et peut être changée dans **Paramètres**. L'app
cherche les nouvelles versions sur GitHub, et Android demande de confirmer chaque
mise à jour.

## Mises à jour

À l’ouverture de l’interface, vpn_specify vérifie la dernière version dans
les [releases GitHub](https://github.com/D3T3F/vpn_specify/releases). S’il y
en a une plus récente, un avis apparaît en haut de la fenêtre :

- **Mettre à jour** : télécharge le paquet, vérifie son SHA-256, se
  déconnecte, se ferme, installe et se rouvre dans la nouvelle version.
- **Pas maintenant** : masque l’avis jusqu’à la prochaine ouverture.
- **Ignorer cette version** : plus d’avis pour cette version, seulement pour
  la suivante.
- **Nouveautés** : affiche le texte de la release.

Dans **Préférences**, vous pouvez désactiver la vérification au démarrage et
vérifier tout de suite (**Rechercher des mises à jour**). Dans le terminal :

```sh
vpn_specify update           # affiche la nouvelle version et propose de l’installer
vpn_specify update -check    # vérifie seulement
vpn_specify update -y        # installe sans demander
vpn_specify version
```

Installation de la nouvelle version :

- **Windows, installé** : le `vpn_specify-setup.exe` de la release s’exécute
  en mode mise à jour. Il garde le même dossier, conserve les raccourcis
  existants et demande l’UAC si nécessaire.
- **Windows, portable** : le même programme d’installation remplace seulement
  les fichiers du dossier, sans registre ni raccourcis.
- **Linux** : l’exécutable est remplacé sur place. Si le dossier exige root
  (ex. `/usr/local/bin`), le système demande le mot de passe (`pkexec`). Avant
  le remplacement, vpn_specify vérifie que le nouvel exécutable fonctionne et
  indique la bonne version.

Sécurité : un téléchargement n’est accepté que depuis
`github.com/D3T3F/vpn_specify/releases/download/` et si son SHA-256
correspond à celui que GitHub indique pour le fichier (ou à un `SHA256SUMS`
dans la release). Sans hachage, rien n’est installé. Cela protège contre un
téléchargement corrompu ou altéré en chemin, pas contre quelqu’un ayant accès
au compte GitHub ; il faudrait pour cela signer les paquets. Les builds sans
version (`dev`) ne recherchent jamais de mises à jour.

## Langues

L’interface, le programme d’installation, les messages et l’aide du terminal
sont disponibles en : English, Português (Brasil), Español, Français,
Deutsch, Русский, 简体中文, 日本語, हिन्दी et العربية (mise en page de droite à
gauche).

- **Par défaut** : la langue du système si elle en fait partie (langue
  d’affichage de Windows ; sous Linux, `LANGUAGE`, `LC_ALL`, `LC_MESSAGES` ou
  `LANG`), sinon l’anglais.
- **Choisir** : en haut du programme d’installation Windows ou, ensuite, dans
  **Préférences > Langue** (la fenêtre change aussitôt). Le choix est
  enregistré dans le dossier de données (fichier `language`) et s’applique
  aussi aux commandes du terminal. L’installation silencieuse accepte
  `/lang=<code>`.
- Les journaux techniques et l’aide de chaque option du terminal restent en
  anglais.

Envie d’améliorer une traduction ou d’ajouter une langue ? Voir
[Adding or improving a translation](../README.md#adding-or-improving-a-translation).

## Licence

L'APK Android contient OpenVPN (GPL-2.0) : l'APK dans son ensemble est donc distribué sous GPL-2.0. Le code de ce projet reste sous MIT, et le code source de chaque APK est ce dépôt au tag de sa version, avec `android/openvpn/build.sh`. Les licences se trouvent dans l'application, sous **Paramètres > Licences**.

[MIT](../LICENSE). Le texte apparaît dans le programme (Préférences > Voir la
licence), dans le programme d’installation, dans les propriétés des `.exe` et
est fourni dans le paquet sous le nom `LICENSE.txt`. Les composants tiers
gardent leurs propres licences : WinDivert (LGPL-3.0 / GPL-2.0) est distribué
sans modification, et les licences de tous les composants se trouvent dans le
dossier `licenses\` du paquet.

Le programme d’installation n’est pas signé numériquement : à l’ouverture,
Windows peut afficher SmartScreen (« Windows a protégé votre ordinateur » >
Informations complémentaires > Exécuter quand même), comme l’explique le
`README.txt` du paquet.
