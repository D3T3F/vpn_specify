# vpn_specify

[English](../README.md) · [Português (Brasil)](README.pt-BR.md) · [Español](README.es.md) · [Français](README.fr.md) · [Deutsch](README.de.md) · [Русский](README.ru.md) · **简体中文** · [日本語](README.ja.md) · [हिन्दी](README.hi.md) · [العربية](README.ar.md)

**只为你选择的应用**连接 VPN。电脑的其余部分仍使用你的普通网络。

```
            ┌──────────────── vpn_specify ────────────────┐
 chrome ──► │ SOCKS5 代理 127.0.0.1:10800 ┐                │
 curl   ──► │ HTTP 代理   127.0.0.1:10801 ┴──► VPN 隧道 ───┼──► VPN 网络
            └─────────────────────────────────────────────┘
 电脑其余部分 ────────────────────────────────────────────────► 普通互联网
```

没有后台服务。在 Windows 上，同一份代码生成两个可执行文件：

- `vpn_specify.exe`：图形界面（无控制台）。
- `vpn_specify-cli.exe`：终端命令（`up`、`run`、`status`、`update`）。

在 Linux 上只有一个可执行文件 `vpn_specify`，同时包含图形界面和终端命令
（见 [Linux](#linux)）。

界面、安装程序和提示信息支持 **10 种语言**（见[语言](#语言)）。

> 这是使用指南。关于编译、发布版本和代码结构，请参阅
> [英文 README](../README.md#building)。

## 安装

从 [releases](https://github.com/D3T3F/vpn_specify/releases) 下载
`vpn_specify-<版本>-win64.zip`，解压后运行 `vpn_specify-setup.exe`：

- 在安装窗口顶部选择语言；之后应用会使用该语言（以后可在**首选项**中更改）。
- 默认文件夹：`C:\Program Files\vpn_specify`（可通过**浏览…**选择其他位置）。
- 选项：桌面快捷方式和开始菜单快捷方式。
- 不会下载任何内容：程序和 WinDivert 都已包含在安装程序中。
- 再次运行安装程序会更新现有安装（同一文件夹）。
- 卸载：设置 > 应用 > 已安装的应用 > vpn_specify。你可以选择删除还是保留
  数据。应用创建的快捷方式（例如 `DiscordVPN`）也会被删除。

无人值守（用于脚本）：

```powershell
vpn_specify-setup.exe /silent [/dir=D:\Apps] [/nodesktop] [/nostartmenu] [/lang=zh-CN]
"C:\Program Files\vpn_specify\vpn_specify.exe" uninstall /silent [/keepdata]
```

也可以不安装直接使用（便携模式）：只需在 `vpn_specify.exe` 旁边放一个
`data\` 文件夹（或一个名为 `portable` 的空文件）。

## 图形界面

打开 `vpn_specify.exe`。Windows 会请求管理员权限。

1. **VPN > 导入文件…**：`.ovpn`/`.conf` 会被复制到数据文件夹。如果 VPN 需要
   登录，请使用**凭据**：密码通过 DPAPI 保存，只有你的 Windows 账户能读取。
   **编辑**可以重命名 VPN（使用它的应用会随之更新）并修改文件内容；需要先
   断开该 VPN。
2. **VPN 中的应用 > 添加应用…**：选择 `.exe` 或开始菜单中的快捷方式（例如
   Discord）。对于带更新程序的应用（`Update.exe --processStart Discord.exe`），
   会自动识别正确的进程（`discord.exe`）。
   - *完全捕获*（默认）：全部流量，包括语音/UDP（WinDivert）。
   - *代理*：仅 TCP，适用于浏览器和 Chromium 应用；无需驱动。
3. **创建快捷方式**：例如在开始菜单（如需要也可在桌面）创建 `DiscordVPN`。
   它会出现在 Windows 搜索中，位于原版 Discord 旁边。

打开快捷方式时：

- 如果 VPN 未开启，UAC 会出现一次。一个窗口显示进度、连接、打开应用后自动
  关闭。
- 如果 VPN 已开启，应用会立即打开，不会出现 UAC。
- 应用以普通用户身份打开，而不是以管理员身份。
- 如果勾选了“通过 VPN 打开的应用关闭后断开 VPN”且窗口已关闭，应用关闭约
  30 秒后 VPN 会自动断开。

### 随系统启动的应用

在应用对话框（**添加应用…**或**编辑**）中勾选**随 Windows 启动**。登录时，
vpn_specify 会连接 VPN 并打开所有勾选的应用，不显示窗口。在列表中它们带有
“随系统启动”标记。

- **登录时没有 UAC**：vpn_specify 会创建一个计划任务
  `vpn_specify\Start apps in VPN`（在任务计划程序中），在登录 15 秒后以管理员
  权限运行 `vpn_specify.exe autostart`。应用仍以普通用户身份打开。
- 在 Linux 上该选项是**随系统启动**。启动项位于
  `~/.config/autostart/vpn_specify.desktop`，在登录 10 秒后运行。此时
  OpenVPN VPN 会要求输入管理员密码。
- 只有一个启动项：勾选第一个应用时创建，没有勾选的应用时删除。每次
  vpn_specify 启动时都会刷新它，因此可执行文件移动或更新后它也会跟随。
- 同一时间只能有一个 VPN 处于活动状态，所以勾选的应用必须使用**同一个
  VPN**。否则窗口会给出提示。
- 刚登录时网络可能还没准备好。每个应用最多尝试 4 次，中间有间隔。密码被
  拒绝或缺少凭据时不会重试。未能打开的应用会在消息中列出。
- 卸载程序会删除该任务。

同一时间只能有一个 VPN 处于活动状态。不要捕获 `cmd.exe` 这类通用名称：所有
同名进程都会进入 VPN。

数据文件夹（**数据文件夹**按钮）：`configs\`、`settings.json`（应用和首选
项）、`language` 和 `vpn_specify.log`。安装版位于
`%LOCALAPPDATA%\vpn_specify`，便携模式下位于 exe 旁边的 `data\`。

不要同时运行安装版和便携版：两者使用相同的本地端口（10799–10801）。

## 支持的格式

| 配置 | 工作方式 | 需要管理员权限？ |
|---|---|---|
| **WireGuard**（`.conf`） | 隧道完全在进程内运行（wireguard-go + gVisor TCP/IP 协议栈）。不会创建网络适配器。 | 否 |
| **OpenVPN**（`.ovpn`） | 使用已安装的 `openvpn`，但会阻止一切会更改系统路由或 DNS 的内容（redirect-gateway、路由、DNS、block-outside-dns）。代理把自己的套接字绑定到隧道上，正是这一点让流量走 VPN。 | 是（OpenVPN 会创建适配器） |

## 用法

```powershell
# 1. 连接（保持此终端打开；Ctrl+C 断开）
vpn_specify-cli up C:\vpns\work.ovpn

# 2. 在另一个终端中，在 VPN 内打开应用
vpn_specify-cli run chrome                       # 独立的 Chrome，只有它走 VPN
vpn_specify-cli run firefox
vpn_specify-cli run msedge https://intranet.company
vpn_specify-cli run curl https://ifconfig.me
vpn_specify-cli run git clone https://git.internal/repo.git
vpn_specify-cli run powershell                   # 其中的命令都使用代理

# 连接并立即打开一个应用
vpn_specify-cli up wg0.conf -- chrome

# 检查：直连 IP 与经 VPN 的 IP（TCP 和 UDP）
vpn_specify-cli status
```

### 按进程捕获（`-capture`）：应用的全部流量都走 VPN

对于忽略代理或使用自身 UDP 的应用（Discord 语音、游戏、原生应用），
`-capture` 会按可执行文件名截获该进程的数据包。应用本身无需任何配置：

```powershell
# 以管理员身份运行的终端
vpn_specify-cli up work.ovpn -capture discord.exe
# 然后像平常一样打开 Discord（开始菜单快捷方式）
```

- **TCP** 经透明代理；**UDP**（语音、视频）经 VPN 内的套接字；进程的
  **IPv6** 会被阻止，使应用通过 VPN 使用 IPv4。
- 其他程序不受影响，但每个出站数据包都会经过 `vpn_specify`，测试中约损失
  10% 的速度。
- 如果应用已经打开，它的 TCP 连接会被断开，以便通过 VPN 重新连接。最稳妥
  的做法是在 `up` 之后再打开应用。
- 可重复使用：`-capture discord.exe -capture steam.exe`。
- 支持 WireGuard 和 OpenVPN。
- 应用通过 Windows 解析的名称（DNS）仍使用电脑的普通 DNS：DNS 服务器能看到
  *哪些*域名，但流量走 VPN。

要求：管理员权限，以及 [WinDivert 2.2.2](https://github.com/basil00/WinDivert/releases/tag/v2.2.2)
的文件（`WinDivert.dll` 和 `WinDivert64.sys`，x64），放在 `.\windivert\` 或
`.exe` 旁边。Windows Defender 经常把 WinDivert 标记为不需要的软件，因为恶意
软件也会使用它；请在 Windows 安全中心 > 排除项中允许该文件夹。捕获期间会有
一条临时防火墙规则“vpn_specify capture”，退出时删除。

### UDP

有两种方式：

1. **SOCKS5 UDP**（`127.0.0.1:10800`，UDP ASSOCIATE）：适用于支持通过
   SOCKS5 传输 UDP 的应用。
2. **端口转发 `-L`**：适用于**任何**程序，即使是不会使用代理的程序。本地
   端口会变成 VPN 内的一个地址：

```powershell
vpn_specify-cli up work.ovpn -L udp:15353=10.0.0.5:53 -L tcp:13389=internal-server:3389
# 现在：内部 DNS 在 127.0.0.1:15353，内部 RDP 在 127.0.0.1:13389
mstsc /v:127.0.0.1:13389
```

`-L` 接受 `[tcp|udp:]本地端口=主机:端口`（或本地 `ip:端口`），可重复使用。
对于 UDP，每个本地来源都有自己的会话；2 分钟无响应的会话会被关闭。

### 常用选项

- `up -v`：完整日志（包括 OpenVPN 的日志）。
- `up -dns 10.0.0.53`：强制指定 VPN 内使用的 DNS。
- `up -socks 127.0.0.1:20800 -http 127.0.0.1:20801`：使用其他端口，例如同时运行两个 VPN。`run` 时使用相同端口。
- `run -profile client2 chrome`：独立的浏览器配置文件（Cookie、登录状态）。
- `run -chromium <Electron 应用>`：强制使用 Chromium 的代理参数。
- `run -detach <程序>`：不等待程序结束。

## 各应用如何进入 VPN

- **Chrome / Edge / Brave / Vivaldi**：`--proxy-server=socks5://...`，在
  `%LOCALAPPDATA%\vpn_specify\profiles\` 下使用独立配置文件（否则窗口会在
  已运行的 Chrome 中打开，不经过 VPN），DNS 远程解析，WebRTC 只走代理。
- **Firefox**：独立配置文件，使用 SOCKS5 代理、远程 DNS，并关闭 DoH。
- **其他任何程序**：`HTTP_PROXY`、`HTTPS_PROXY` 和 `ALL_PROXY` 环境变量。
  适用于 curl、git、npm、pip、Go、.NET HttpClient 等许多程序。

名称（DNS）在 **VPN 内**解析，因此内部名称（`intranet.company`）可以使用，
也不会泄露给你所在网络的 DNS。

## 限制

- **忽略代理的应用**：使用 `-capture`（或用 `-L` 转发固定地址）。
- Chrome 和 Firefox 不会通过 SOCKS 发送 UDP（QUIC/HTTP3）；它们会改用
  TCP，可正常工作。
- UDP 和 OpenVPN 模式仅支持 **IPv4**。
- 如果 `vpn_specify` 被强制结束，`openvpn` 也会随之结束（Windows 上通过 Job
  Object，Linux 上通过 SIGTERM），但度量值为 9999 的路由可能会留在已断开的
  TAP 适配器上。它不影响任何东西；正常按 Ctrl+C 会清除一切。
- 代理只监听 `127.0.0.1`，但在 `up` 运行期间，**你电脑上的任何程序**都可以
  使用它。

## Linux

在 Linux 上，图形界面和终端命令（`up`、`run`、`status`、`update` 和 `-L`）
都可使用，支持 WireGuard 和 OpenVPN，端口与 Windows 相同。安装程序和
`-capture` 仍仅限 Windows。

```sh
tar -xzf vpn_specify-<版本>-linux-amd64.tar.gz   # 或 -linux-arm64
sudo install vpn_specify-<版本>-linux-amd64/vpn_specify /usr/local/bin/
vpn_specify                                # 打开图形界面
```

### Linux 上的图形界面

不带参数运行 `vpn_specify`。首次使用时，通过**首选项 > 将“vpn_specify”添加
到应用程序菜单**把图标放进菜单。窗口与 Windows 相同，区别如下：

- **窗口**：与 Windows 上是同一个页面，由 vpn_specify 自己在 `127.0.0.1`
  提供（随机端口，用随机令牌保护）。它以应用窗口形式（无标签页和地址栏）在
  Chrome、Chromium、Edge、Brave 或 Vivaldi 中打开，使用独立的配置文件。如果
  这些都没有，则在默认浏览器的标签页中打开。支持 X11 和 Wayland。
- **无需 root**：界面以普通用户身份运行。连接 **OpenVPN** VPN 时，系统只为
  `openvpn` 请求管理员密码（polkit 的 `pkexec`）。它运行在一个小的辅助进程
  中，在你断开连接或关闭 vpn_specify 时将其结束。不使用 root 将套接字绑定到
  隧道需要内核 5.7 或更高版本。WireGuard 不需要任何密码。
- **添加应用**会列出应用程序菜单中的应用（`.desktop` 文件，包括 flatpak 和
  snap），并支持搜索。**选择文件…**使用 zenity 或 kdialog；如果都没有，请在
  “程序”字段中直接输入路径。
- **仅代理模式**：没有完全捕获。浏览器、Chromium/Electron 应用（Discord、
  Slack、VS Code…）以及遵循 `HTTP_PROXY` 的程序会进入 VPN。Discord 语音/UDP
  和游戏不会。
- **创建快捷方式**会在 `~/.local/share/applications/` 中写入一个 `.desktop`
  文件（出现在菜单搜索中，位于原应用旁边），如勾选也会写到桌面。
- **凭据**：密码使用 `~/.local/share/vpn_specify/secret.key`（权限 0600）
  中的密钥加密（AES-GCM），与 `settings.json` 分开存放。它靠文件权限保护，
  就像 SSH 密钥一样，而不是靠系统密钥环。在 Windows 上保存的密码（DPAPI）
  需要重新输入。
- **数据**：`~/.config/vpn_specify/`（`configs/`、`settings.json`、
  `language`、`vpn_specify.log`）。

生命周期与 Windows 相同。在 VPN 关闭时打开快捷方式，窗口会显示进度、连接、
打开应用并关闭。窗口关闭时，通过 VPN 打开的应用全部关闭约 30 秒后，VPN 会
断开。

### Linux 上的终端

```sh
vpn_specify up wg0.conf                    # WireGuard：无需 root
sudo vpn_specify up work.ovpn              # OpenVPN：需要 root 和 openvpn 软件包

# 在另一个终端中，以普通用户身份
vpn_specify run chrome                     # 会找到 google-chrome-stable、chromium...
vpn_specify run firefox
vpn_specify run curl https://ifconfig.me
vpn_specify status
```

与 Windows 的区别：

- **OpenVPN**：在 Linux 上，仅把套接字绑定到隧道 IP 并不能让流量从隧道出
  去。因此代理会用 `SO_BINDTODEVICE` 把套接字绑定到 `tun` 接口，并把该接口
  的 `rp_filter` 设为 *loose*。否则，使用严格 `rp_filter` 的发行版（Fedora、
  RHEL）会丢弃回复。系统路由和 DNS 保持不变，与 Windows 一样。如果
  `vpn_specify` 被强制结束，`openvpn` 会收到 SIGTERM 并一起退出。
- **`sudo vpn_specify up ... -- 程序`**：程序以调用 `sudo` 的用户身份打开，
  而不是 root。浏览器配置文件放在该用户的主目录中。不过最简单的做法仍是在
  另一个终端中不带 `sudo` 运行 `run`。
- **浏览器配置文件**：位于 `~/.config/vpn_specify/profiles/`。以 snap 方式
  安装的 Firefox 和 Chromium（Ubuntu 默认）无法读取主目录中的隐藏文件夹，
  因此它们的配置文件放在 `~/snap/<app>/common/vpn_specify/profiles/`。
- **忽略代理的应用**：没有 `-capture` 时，请用 `-L` 转发固定地址。

## 更新

打开界面时，vpn_specify 会在 [GitHub releases](https://github.com/D3T3F/vpn_specify/releases)
中检查最新版本。如果有更新的版本，窗口顶部会出现提示：

- **更新**：下载软件包、校验 SHA-256、断开连接、关闭、安装，然后以新版本
  重新打开。
- **以后再说**：隐藏提示，直到下次打开应用。
- **跳过此版本**：不再提示此版本，只提示下一个版本。
- **新功能**：显示发布说明。

在**首选项**中可以关闭启动时检查，也可以立即检查（**检查更新**）。在终端中：

```sh
vpn_specify update           # 显示新版本并询问是否安装
vpn_specify update -check    # 只检查
vpn_specify update -y        # 不询问直接安装
vpn_specify version
```

新版本的安装方式：

- **Windows 安装版**：发布中的 `vpn_specify-setup.exe` 以更新模式运行。它
  使用相同文件夹，保留现有快捷方式，必要时请求 UAC。
- **Windows 便携版**：同一个安装程序只替换文件夹中的文件，不写注册表，也不
  创建快捷方式。
- **Linux**：就地替换可执行文件。如果文件夹需要 root（例如
  `/usr/local/bin`），系统会请求密码（`pkexec`）。替换前，vpn_specify 会检查
  新的可执行文件能否运行并报告正确的版本。

安全性：只接受来自 `github.com/D3T3F/vpn_specify/releases/download/` 的下载，
并且其 SHA-256 必须与 GitHub 为该文件提供的值（或发布中的 `SHA256SUMS`
文件）一致。没有哈希值就不会安装任何东西。这能防止下载途中被损坏或篡改，
但无法防范能访问该 GitHub 账户的人；要做到这一点需要对软件包签名。没有版本
号的构建（`dev`）永远不会检查更新。

## 语言

界面、安装程序、提示信息和终端帮助支持：English、Português (Brasil)、
Español、Français、Deutsch、Русский、简体中文、日本語、हिन्दी 和 العربية
（从右到左排版）。

- **默认**：如果系统语言在上述列表中，就使用系统语言（Windows 显示语言；
  Linux 上依次为 `LANGUAGE`、`LC_ALL`、`LC_MESSAGES` 或 `LANG`），否则使用
  英语。
- **选择**：在 Windows 安装程序顶部选择，或之后在**首选项 > 语言**中更改
  （窗口立即切换）。选择会保存在数据文件夹中（`language` 文件），也适用于
  终端命令。静默安装可使用 `/lang=<代码>`。
- 技术日志和终端各个选项的帮助保持英文。

想改进翻译或添加新语言？请参阅
[Adding or improving a translation](../README.md#adding-or-improving-a-translation)。

## 许可证

[MIT](../LICENSE)。许可证文本显示在程序中（首选项 > 查看许可证）、安装程序
中、`.exe` 的属性中，并以 `LICENSE.txt` 随软件包提供。第三方组件保留各自的
许可证：WinDivert（LGPL-3.0 / GPL-2.0）以未修改的形式分发，所有组件的许可证
都在软件包的 `licenses\` 文件夹中。

安装程序没有数字签名：打开时 Windows 可能会显示 SmartScreen（“Windows 已保护
你的电脑” > 更多信息 > 仍要运行），软件包中的 `README.txt` 对此有说明。
