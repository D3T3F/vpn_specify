vpn_specify {{VERSION}}
=======================

Connects to a VPN only for the apps you choose. The rest of the computer
stays on your normal network.

Available in English, Português (Brasil), Español, Français, Deutsch,
Русский, 简体中文, 日本語, हिन्दी and العربية. The installer lets you
pick the language; you can change it later in Preferences.
Documentation in every language: https://github.com/D3T3F/vpn_specify


INSTALL
-------

1. Run "vpn_specify-setup.exe" (Windows asks for Administrator permission).
2. Choose the language, the folder (default: C:\Program Files\vpn_specify)
   and whether you want desktop and Start menu shortcuts.
3. Click Install.

The installer downloads nothing: the program is already inside it.

To uninstall: Settings > Apps > Installed apps > vpn_specify > Uninstall.
You choose whether to keep or delete your data.


REQUIREMENTS
------------

- Windows 10 or 11, 64-bit.
- Microsoft Edge WebView2 Runtime (included in Windows 11 and in updated
  Windows 10).
- For OpenVPN VPNs (.ovpn): OpenVPN 2.6 or newer installed
  (https://openvpn.net/community-downloads/). WireGuard VPNs (.conf) need
  nothing extra.


WHERE THE DATA LIVES
--------------------

Imported VPNs, registered apps, preferences and the chosen language are in
%LOCALAPPDATA%\vpn_specify. Passwords are protected by Windows (DPAPI): only
your account on this computer can read them.


"WINDOWS PROTECTED YOUR PC" WARNING
-----------------------------------

The installer has no digital signature (a paid certificate). So when you open
a file downloaded from the internet, Windows (SmartScreen) may show the blue
"Windows protected your PC" screen. This is NOT a detected virus and needs no
antivirus exclusion:

  1. Click "More info".
  2. Click "Run anyway".

If Windows blocks the file before that: right-click the .zip > Properties >
check "Unblock" > OK, and extract it again.


ANTIVIRUS
---------

Full capture (voice/UDP of apps like Discord) uses the WinDivert driver,
which ships with the program and has a valid digital signature. At the last
check Windows Defender reported nothing, but some antivirus products may flag
WinDivert as "unwanted", because malware uses it too. Only if that happens,
allow the installation folder in Windows Security > Virus & threat
protection > Manage settings > Exclusions.


LICENSE
-------

vpn_specify is free software, distributed under the MIT license (file
LICENSE.txt): you may use, copy, modify and redistribute it, keeping the
copyright notice. It is provided "as is", without warranty.

The "licenses" folder has the licenses of the bundled third-party components
(WinDivert, wireguard-go, gVisor, go-webview2 and Go libraries).
