# vpn_specify

[English](../README.md) · [Português (Brasil)](README.pt-BR.md) · [Español](README.es.md) · [Français](README.fr.md) · [Deutsch](README.de.md) · [Русский](README.ru.md) · [简体中文](README.zh-CN.md) · **日本語** · [हिन्दी](README.hi.md) · [العربية](README.ar.md)

**選んだアプリだけ**を VPN に接続します。コンピューターのその他の通信は普段の
ネットワークのままです。

```
            ┌──────────────── vpn_specify ────────────────┐
 chrome ──► │ SOCKS5 プロキシ 127.0.0.1:10800 ┐            │
 curl   ──► │ HTTP プロキシ   127.0.0.1:10801 ┴─► VPN トンネル ─► VPN のネットワーク
            └─────────────────────────────────────────────┘
 PC のその他 ─────────────────────────────────────────────────► 通常のインターネット
```

バックグラウンドサービスはありません。Windows では同じコードから 2 つの実行
ファイルを作ります。

- `vpn_specify.exe`: グラフィカルインターフェイス（コンソールなし）。
- `vpn_specify-cli.exe`: ターミナルのコマンド（`up`、`run`、`status`、`update`）。

Linux では実行ファイルは `vpn_specify` の 1 つだけで、グラフィカルインター
フェイスとターミナルのコマンドの両方を備えています（[Linux](#linux) を参照）。

インターフェイス、インストーラー、メッセージは **10 言語**に対応しています
（[言語](#言語) を参照）。

> これは利用ガイドです。ビルド、リリースの公開、コード構成については
> [英語の README](../README.md#building) を参照してください。

## インストール

[releases](https://github.com/D3T3F/vpn_specify/releases) から
`vpn_specify-<バージョン>-win64.zip` をダウンロードして展開し、
`vpn_specify-setup.exe` を実行します。

- インストーラーのウィンドウ上部で言語を選びます。以降アプリはその言語を使い
  ます（後で**設定**から変更できます）。
- 既定のフォルダー: `C:\Program Files\vpn_specify`（**参照…** で別の場所も選べます）。
- オプション: デスクトップとスタートメニューのショートカット。
- 何もダウンロードしません。プログラムと WinDivert はインストーラーに含まれて
  います。
- インストーラーをもう一度実行すると、既存のインストールを更新します（同じ
  フォルダー）。
- アンインストール: 設定 > アプリ > インストールされているアプリ >
  vpn_specify。データを削除するか残すかを選べます。アプリが作成した
  ショートカット（例: `DiscordVPN`）も削除されます。

クリックなし（スクリプト用）:

```powershell
vpn_specify-setup.exe /silent [/dir=D:\Apps] [/nodesktop] [/nostartmenu] [/lang=ja]
"C:\Program Files\vpn_specify\vpn_specify.exe" uninstall /silent [/keepdata]
```

インストールせずに使うこともできます（ポータブルモード）。`vpn_specify.exe` の
隣に `data\` フォルダー（または `portable` という空ファイル）を置くだけです。

## グラフィカルインターフェイス

`vpn_specify.exe` を開きます。Windows が管理者の許可を求めます。

1. **VPN > ファイルをインポート…**: `.ovpn`/`.conf` がデータフォルダーにコピー
   されます。VPN にログインが必要な場合は**認証情報**を使います。パスワード
   は DPAPI で保存され、あなたの Windows アカウントだけが読み取れます。
   **編集**では VPN の名前変更（使っているアプリも追従します）とファイル内容
   の変更ができます。先にその VPN を切断しておく必要があります。
2. **VPN を使うアプリ > アプリを追加…**: `.exe` またはスタートメニューの
   ショートカット（例: Discord）を選びます。更新プログラム付きのアプリ
   （`Update.exe --processStart Discord.exe`）では、正しいプロセス
   （`discord.exe`）が自動的に判別されます。
   - *完全キャプチャ*（既定）: 音声/UDP を含むすべての通信（WinDivert）。
   - *プロキシ*: TCP のみ。ブラウザーと Chromium アプリ向け。ドライバー不要。
3. **ショートカットを作成**: たとえば `DiscordVPN` をスタートメニュー（必要
   ならデスクトップにも）に作成します。元の Discord の隣に Windows の検索で
   表示されます。

ショートカットを開いたとき:

- VPN がオフなら、UAC が 1 回表示されます。ウィンドウが進行状況を表示し、
  接続してアプリを開き、自動で閉じます。
- VPN がすでにオンなら、UAC なしですぐにアプリが開きます。
- アプリは管理者ではなく通常のユーザーとして開きます。
- 「VPN で開いたアプリがすべて閉じたら VPN を切断する」をオンにしてウィンドウ
  を閉じておくと、アプリを閉じてから約 30 秒後に VPN が切断されます。

### システムと一緒に起動するアプリ

アプリのダイアログ（**アプリを追加…** または **編集**）で **Windows と一緒に
開く** をオンにします。サインインすると、vpn_specify が VPN に接続し、チェック
したアプリをすべてウィンドウを表示せずに開きます。一覧では「システムと一緒に
起動」というバッジが付きます。

- **サインイン時に UAC なし**: vpn_specify はタスクスケジューラーにタスク
  `vpn_specify\Start apps in VPN` を作成し、サインインの 15 秒後に管理者権限で
  `vpn_specify.exe autostart` を実行します。アプリは通常のユーザーとして
  開きます。
- Linux ではオプション名は **システムと一緒に開く** です。エントリーは
  `~/.config/autostart/vpn_specify.desktop` にあり、ログインの 10 秒後に実行
  されます。OpenVPN の VPN はその時点で管理者パスワードを求めます。
- エントリーは 1 つだけで、最初のアプリをチェックしたときに作成され、チェック
  がなくなると削除されます。vpn_specify の起動のたびに更新されるので、実行
  ファイルが移動や更新されても追従します。
- 同時に有効にできる VPN は 1 つだけなので、チェックしたアプリは**同じ VPN**
  を使う必要があります。そうでない場合はウィンドウが警告します。
- サインイン直後はネットワークの準備に時間がかかることがあります。各アプリは
  間隔を空けて最大 4 回試行します。拒否されたパスワードや不足している認証情報
  は再試行しません。開けなかったものはメッセージで表示されます。
- アンインストーラーはタスクを削除します。

同時に有効にできる VPN は 1 つだけです。`cmd.exe` のような汎用的な名前は
キャプチャしないでください。同じ名前のプロセスがすべて VPN に入ってしまいます。

データフォルダー（**データフォルダー** ボタン）: `configs\`、`settings.json`
（アプリと設定）、`language`、`vpn_specify.log`。インストール版では
`%LOCALAPPDATA%\vpn_specify`、ポータブルモードでは exe の隣の `data\` です。

インストール版とポータブル版を同時に実行しないでください。どちらも同じローカル
ポート（10799–10801）を使います。

## 対応形式

| 設定 | しくみ | 管理者権限が必要？ |
|---|---|---|
| **WireGuard**（`.conf`） | トンネルはプロセス内で完結します（wireguard-go + gVisor の TCP/IP スタック）。ネットワークアダプターは作成しません。 | いいえ |
| **OpenVPN**（`.ovpn`） | インストール済みの `openvpn` を使いますが、システムのルーティングや DNS を変更するもの（redirect-gateway、ルート、DNS、block-outside-dns）はすべてブロックします。プロキシが自分のソケットをトンネルにバインドすることで、通信が VPN を通ります。 | はい（OpenVPN がアダプターを作成） |

OpenVPN がない場合、vpn_specify は接続時にインストールを提案します。Windows では公式インストーラーをダウンロードし(固定の SHA-256 で検証)、OpenVPN の GUI なしでサイレントインストールします。Linux ではディストリビューションの `openvpn` パッケージ(apt、dnf、yum、zypper、pacman、apk)をインストールします。自分でインストールすることもできます。

## 使い方

```powershell
# 1. 接続（このターミナルは開いたままに。Ctrl+C で切断）
vpn_specify-cli up C:\vpns\work.ovpn

# 2. 別のターミナルで、VPN 内でアプリを開く
vpn_specify-cli run chrome                       # 別の Chrome。VPN を通るのはこれだけ
vpn_specify-cli run firefox
vpn_specify-cli run msedge https://intranet.company
vpn_specify-cli run curl https://ifconfig.me
vpn_specify-cli run git clone https://git.internal/repo.git
vpn_specify-cli run powershell                   # 中のコマンドがプロキシを使うシェル

# 接続してすぐにアプリを開く
vpn_specify-cli up wg0.conf -- chrome

# 確認: 直接の IP と VPN 経由の IP（TCP と UDP）
vpn_specify-cli status
```

### プロセス単位のキャプチャ（`-capture`）: アプリの通信をすべて VPN に

プロキシを無視するアプリや独自の UDP を使うアプリ（Discord の音声、ゲーム、
ネイティブアプリ）では、`-capture` が実行ファイル名でプロセスのパケットを
横取りします。アプリ側の設定は不要です。

```powershell
# 管理者として実行したターミナル
vpn_specify-cli up work.ovpn -capture discord.exe
# その後、いつもどおり Discord を開く（スタートメニューのショートカット）
```

- **TCP** は透過プロキシ経由、**UDP**（音声、ビデオ）は VPN 内のソケット経由
  です。プロセスの **IPv6** はブロックされ、アプリは VPN 経由で IPv4 を使い
  ます。
- ほかのプログラムには影響しませんが、送信パケットはすべて `vpn_specify` を
  通るため、テストでは速度が約 10% 低下しました。
- アプリがすでに開いていた場合は、VPN 経由で再接続させるために TCP 接続を
  切断します。`up` の後にアプリを開くのが確実です。
- 繰り返し指定できます: `-capture discord.exe -capture steam.exe`。
- WireGuard と OpenVPN の両方で使えます。
- アプリが Windows 経由で解決する名前（DNS）は、引き続き PC の通常の DNS を
  使います。DNS サーバーには*どの*ドメインかは見えますが、通信は VPN を通り
  ます。

必要なもの: 管理者権限と [WinDivert 2.2.2](https://github.com/basil00/WinDivert/releases/tag/v2.2.2)
のファイル（`WinDivert.dll` と `WinDivert64.sys`、x64）を `.\windivert\` か
`.exe` の隣に置くこと。WinDivert はマルウェアにも使われるため、Windows
Defender が望ましくないソフトとして検出することがあります。Windows セキュリティ
> 除外でフォルダーを許可してください。キャプチャ中は一時的なファイアウォール
規則「vpn_specify capture」があり、終了時に削除されます。

### UDP

方法は 2 つあります。

1. **SOCKS5 UDP**（`127.0.0.1:10800`、UDP ASSOCIATE）: SOCKS5 経由の UDP に
   対応したアプリ向け。
2. **ポート転送 `-L`**: プロキシを使えないプログラムを含め、**どんな**
   プログラムでも使えます。ローカルポートが VPN 内のアドレスになります。

```powershell
vpn_specify-cli up work.ovpn -L udp:15353=10.0.0.5:53 -L tcp:13389=internal-server:3389
# これで内部 DNS は 127.0.0.1:15353、内部 RDP は 127.0.0.1:13389
mstsc /v:127.0.0.1:13389
```

`-L` は `[tcp|udp:]ローカルポート=ホスト:ポート`（またはローカルの
`ip:ポート`）を受け付け、繰り返し指定できます。UDP では送信元ごとに専用の
セッションを持ち、2 分間応答がないセッションは閉じられます。

### 便利なオプション

- `up -v`: 完全なログ（OpenVPN のログも含む）。
- `up -dns 10.0.0.53`: VPN 内で使う DNS を指定します。
- `up -socks 127.0.0.1:20800 -http 127.0.0.1:20801`: 別のポート。たとえば 2 つの VPN を同時に使う場合。`run` でも同じポートを指定します。
- `run -profile client2 chrome`: 別のブラウザープロファイル（Cookie、ログイン）。
- `run -chromium <Electron アプリ>`: Chromium のプロキシ用フラグを強制します。
- `run -detach <プログラム>`: プログラムの終了を待ちません。

## 各アプリが VPN に入るしくみ

- **Chrome / Edge / Brave / Vivaldi**: `--proxy-server=socks5://...`、
  `%LOCALAPPDATA%\vpn_specify\profiles\` の専用プロファイル（そうしないと、
  すでに起動している Chrome でウィンドウが開き、VPN の外になります）、
  リモート DNS、WebRTC はプロキシ経由のみ。
- **Firefox**: SOCKS5 プロキシ、リモート DNS、DoH 無効の専用プロファイル。
- **その他のプログラム**: `HTTP_PROXY`、`HTTPS_PROXY`、`ALL_PROXY` 環境変数。
  curl、git、npm、pip、Go、.NET HttpClient など多くのプログラムで使えます。

名前（DNS）は **VPN 内**で解決されるため、内部の名前（`intranet.company`）
も使え、あなたのネットワークの DNS には漏れません。

## 制限事項

- **プロキシを無視するアプリ**: `-capture`（または固定アドレスなら `-L`）を
  使ってください。
- Chrome と Firefox は SOCKS 経由で UDP（QUIC/HTTP3）を送りません。TCP を使い、
  問題なく動作します。
- UDP と OpenVPN モードは **IPv4** のみ対応です。
- `vpn_specify` が強制終了されると `openvpn` も終了します（Windows では Job
  Object、Linux では SIGTERM）が、メトリック 9999 のルートが切断された TAP
  アダプターに残ることがあります。影響はなく、通常の Ctrl+C ですべて削除され
  ます。
- プロキシは `127.0.0.1` でのみ待ち受けますが、`up` の実行中は **PC 上のどの
  プログラム**でも使えます。

## Linux

Linux ではグラフィカルインターフェイスとターミナルのコマンド（`up`、`run`、
`status`、`update`、`-L`）が、WireGuard と OpenVPN で、Windows と同じポートで
使えます。インストーラーと `-capture` は引き続き Windows 専用です。

```sh
tar -xzf vpn_specify-<バージョン>-linux-amd64.tar.gz   # または -linux-arm64
sudo install vpn_specify-<バージョン>-linux-amd64/vpn_specify /usr/local/bin/
vpn_specify                                # グラフィカルインターフェイスを開く
```

### Linux のグラフィカルインターフェイス

引数なしで `vpn_specify` を実行します。初回は **設定 > 「vpn_specify」を
アプリケーションメニューに追加** でメニューにアイコンを追加します。ウィンドウ
は Windows と同じで、次の違いがあります。

- **ウィンドウ**: Windows と同じページを、vpn_specify 自身が `127.0.0.1`
  （ランダムなポート、ランダムなトークンで保護）で提供します。Chrome、
  Chromium、Edge、Brave、Vivaldi のアプリウィンドウ（タブやアドレスバーなし）
  として、専用プロファイルで開きます。どれもない場合は既定のブラウザーの
  タブで開きます。X11 と Wayland に対応しています。
- **root 不要**: インターフェイスは通常のユーザーとして動作します。
  **OpenVPN** の VPN に接続するときだけ、システムが `openvpn` のために管理者
  パスワード（polkit の `pkexec`）を求めます。openvpn は小さな補助プロセスの
  中で動き、切断時や vpn_specify の終了時に停止されます。root なしでソケット
  をトンネルにバインドするにはカーネル 5.7 以降が必要です。WireGuard では
  パスワードは一切不要です。
- **アプリを追加** では、アプリケーションメニューのアプリ（flatpak と snap を
  含む `.desktop` ファイル）を検索付きで一覧表示します。**ファイルを選択…**
  は zenity または kdialog を使います。どちらもない場合は「プログラム」欄に
  パスを入力してください。
- **プロキシモードのみ**: 完全キャプチャはありません。VPN に入るのは
  ブラウザー、Chromium/Electron アプリ（Discord、Slack、VS Code…）、
  `HTTP_PROXY` に従うプログラムです。Discord の音声/UDP やゲームは入りません。
- **ショートカットを作成** は `~/.local/share/applications/` に `.desktop`
  ファイルを書き込み（元のアプリの隣にメニュー検索で表示されます）、チェック
  すればデスクトップにも作成します。
- **認証情報**: パスワードは `~/.local/share/vpn_specify/secret.key`
  （パーミッション 0600）の鍵で暗号化（AES-GCM）され、`settings.json` とは
  別に保存されます。SSH 鍵と同様にファイルのパーミッションで保護され、
  システムのキーリングは使いません。Windows で保存したパスワード（DPAPI）は
  入力し直す必要があります。
- **データ**: `~/.config/vpn_specify/`（`configs/`、`settings.json`、
  `language`、`vpn_specify.log`）。

動作の流れは Windows と同じです。VPN がオフの状態でショートカットを開くと、
ウィンドウが進行状況を表示し、接続してアプリを開き、閉じます。ウィンドウを
閉じている場合、VPN で開いたアプリがすべて閉じてから約 30 秒後に VPN が切断
されます。

### Linux のターミナル

```sh
vpn_specify up wg0.conf                    # WireGuard: root 不要
sudo vpn_specify up work.ovpn              # OpenVPN: root と openvpn パッケージが必要

# 別のターミナルで、通常のユーザーとして
vpn_specify run chrome                     # google-chrome-stable、chromium などを探します
vpn_specify run firefox
vpn_specify run curl https://ifconfig.me
vpn_specify status
```

Windows との違い:

- **OpenVPN**: Linux では、ソケットをトンネルの IP にバインドするだけでは
  トンネルから出ていきません。そのためプロキシは `SO_BINDTODEVICE` でソケット
  を `tun` インターフェイスにバインドし、そのインターフェイスの `rp_filter` を
  *loose* にします。そうしないと、`rp_filter` が厳格なディストリビューション
  （Fedora、RHEL）では応答が破棄されます。Windows と同様、システムのルートと
  DNS は変更しません。`vpn_specify` が強制終了されると、`openvpn` も SIGTERM を
  受けて終了します。
- **`sudo vpn_specify up ... -- プログラム`**: プログラムは root ではなく、
  `sudo` を実行したユーザーとして開きます。ブラウザーのプロファイルはその
  ユーザーのホームに作られます。それでも、別のターミナルで `sudo` なしで
  `run` を使うのがいちばん簡単です。
- **ブラウザーのプロファイル**: `~/.config/vpn_specify/profiles/` にあります。
  snap でインストールした Firefox と Chromium（Ubuntu の既定）はホームの隠し
  フォルダーを読めないため、その場合は `~/snap/<app>/common/vpn_specify/profiles/`
  に置きます。
- **プロキシを無視するアプリ**: `-capture` がないので、固定アドレスには `-L`
  を使ってください。

## Android

`vpn_specify-<バージョン>-android-arm64.apk` をインストールします(Android 8 以降。求められたらこの提供元からのインストールを許可してください)。Android にはアプリごとの VPN が組み込まれているため、プロキシは使いません。選択したアプリの TCP と UDP の全トラフィック(音声を含む)がトンネルを通り、root は不要です。スマートフォンの他の部分は通常のネットワークのままです。

1. **ファイルをインポート…**: WireGuard の `.conf` または OpenVPN の `.ovpn` を選びます。
2. **アプリを選択…**: VPN を使うアプリ(Discord など)にチェックを入れます。
3. **接続** を押し、初回は Android の VPN 権限を許可します。

Android はアプリが開かれたことを検知できないため、アプリに VPN を使わせる方法は 2 つあります。

- **VPN をオンのままにする**(推奨): どのように開いてもそのアプリは常に VPN 内にあり、他のものは入りません。**スマートフォンの起動時に接続** で起動のたびにこれを行います。システムの VPN 設定では、**常時接続 VPN** で Android が自動的に再起動し、**VPN なしの接続をブロック** で選択したアプリが VPN の外に出なくなります(スマートフォンの他の部分もブロックされます)。
- **ショートカット**: ホーム画面に「Discord (VPN)」アイコンを作成します。接続してからアプリを開きます。通常のアイコンでは VPN はオンになりません。

Android での制限:

- 選択したアプリが VPN の外に出ることはありません。VPN が内部ネットワークにしか届かない場合、そのアプリではインターネットのサイトを開けません。
- OpenVPN: 証明書と鍵は `.ovpn` の中に必要です。TAP は非対応で、パスワード付きの秘密鍵もまだサポートされていません。プログラムを実行するディレクティブは削除されます。
- Android で有効にできる VPN は一度に 1 つです。別の VPN を開始するとこの VPN はオフになります。
- 接続中にアプリや VPN を変更するとトンネルが再作成され、選択したアプリが一瞬中断します。

言語はシステムに従い、**設定** で変更できます。アプリは GitHub で新しいバージョンを探し、更新のたびに Android が確認を求めます。

## 更新

インターフェイスを開くと、vpn_specify は [GitHub のリリース](https://github.com/D3T3F/vpn_specify/releases)
で最新バージョンを確認します。新しいバージョンがあれば、ウィンドウ上部に
お知らせが表示されます。

- **更新**: パッケージをダウンロードして SHA-256 を検証し、切断して終了し、
  インストールしてから新しいバージョンで再び開きます。
- **後で**: 次にアプリを開くまでお知らせを隠します。
- **このバージョンをスキップ**: このバージョンのお知らせは表示せず、次の
  バージョンだけ通知します。
- **新機能**: リリースの説明を表示します。

**設定** では起動時の確認をオフにしたり、すぐに確認したり（**更新を確認**）
できます。ターミナルでは:

```sh
vpn_specify update           # 新しいバージョンを表示し、インストールするか確認します
vpn_specify update -check    # 確認のみ
vpn_specify update -y        # 確認なしでインストール
vpn_specify version
```

新しいバージョンのインストール方法:

- **Windows（インストール版）**: リリースの `vpn_specify-setup.exe` が更新
  モードで実行されます。同じフォルダーを使い、既存のショートカットを残し、
  必要なら UAC を求めます。
- **Windows（ポータブル版）**: 同じインストーラーがフォルダーのファイルだけを
  置き換えます。レジストリやショートカットは作りません。
- **Linux**: 実行ファイルをその場で置き換えます。フォルダーに root が必要な
  場合（例: `/usr/local/bin`）は、システムがパスワードを求めます（`pkexec`）。
  置き換える前に、新しい実行ファイルが動作し正しいバージョンを返すことを
  確認します。

セキュリティ: ダウンロードは `github.com/D3T3F/vpn_specify/releases/download/`
からのものだけを受け付け、その SHA-256 が GitHub がファイルに示す値（または
リリース内の `SHA256SUMS`）と一致する場合に限ります。ハッシュがなければ何も
インストールしません。これは途中で壊れたり改ざんされたりしたダウンロードを
防ぎますが、GitHub アカウントにアクセスできる人に対しては無力です。それには
パッケージへの署名が必要です。バージョンのないビルド（`dev`）は更新を確認
しません。

## 言語

インターフェイス、インストーラー、メッセージ、ターミナルのヘルプは次の言語に
対応しています: English、Português (Brasil)、Español、Français、Deutsch、
Русский、简体中文、日本語、हिन्दी、العربية（右から左のレイアウト）。

- **既定**: システムの言語が上記に含まれていればその言語（Windows の表示言語。
  Linux では `LANGUAGE`、`LC_ALL`、`LC_MESSAGES`、`LANG`）、それ以外は英語。
- **選択**: Windows インストーラーの上部、または後から **設定 > 言語**（ウィン
  ドウはすぐに切り替わります）。選択はデータフォルダー（`language` ファイル）
  に保存され、ターミナルのコマンドにも適用されます。サイレントインストール
  では `/lang=<コード>` を指定できます。
- 技術的なログと、ターミナルの各オプションのヘルプは英語のままです。

翻訳の改善や言語の追加をしたい場合は
[Adding or improving a translation](../README.md#adding-or-improving-a-translation)
を参照してください。

## ライセンス

Android の APK には OpenVPN(GPL-2.0)が含まれるため、APK 全体は GPL-2.0 で配布されます。このプロジェクトのコードは MIT のままで、各 APK のソースコードは、そのバージョンのタグにあるこのリポジトリと `android/openvpn/build.sh` です。ライセンスはアプリの**設定 > ライセンス**で確認できます。

[MIT](../LICENSE)。ライセンス文はプログラム（設定 > ライセンスを表示）、
インストーラー、`.exe` のプロパティに表示され、パッケージには `LICENSE.txt` と
して含まれます。サードパーティーのコンポーネントはそれぞれのライセンスに従い
ます。WinDivert（LGPL-3.0 / GPL-2.0）は改変せずに配布しており、すべての
コンポーネントのライセンスはパッケージの `licenses\` フォルダーにあります。

インストーラーにはデジタル署名がありません。開くと Windows が SmartScreen
（「Windows によって PC が保護されました」> 詳細情報 > 実行）を表示することが
あります。詳しくはパッケージの `README.txt` を参照してください。
