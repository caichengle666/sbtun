# sbtun

轻量级 sing-box TUN 客户端，为 Windows 和 Linux 提供桌面 GUI，同时保留适合脚本与服务器环境的 CLI。

sbtun 负责节点、订阅、路由、规则集、健康检查和运行状态；代理内核由 sing-box 提供。用户配置与 sing-box 运行配置分离，不需要手写底层 JSON。

## 主要特性

- **桌面与命令行双入口**：GUI 和 CLI 共用配置与运行状态。
- **常用协议支持**：VLESS、VMess、Trojan、Shadowsocks、Hysteria2、Hysteria、TUIC、AnyTLS、WireGuard、SOCKS 和 HTTP。
- **节点管理**：点击节点即可切换；粘贴多行链接或订阅导入（安卓可扫码导入）、订阅定时自动更新（默认每 24 小时，可调或关闭）和一键更新、手动编辑；按协议或订阅分组，测试后按延迟排序，批量导出与删除，单个节点可显示分享链接二维码供手机扫码导入。
- **三种路由模式 + 自己的规则**：智能分流、全局代理、全局直连；自己的分流规则和规则集在智能、全局下优先生效。
- **规则集管理**：.srs、Clash/Surge 规则列表、域名列表都能加，每个规则集可选去向；导入 Clash 配置或 ACL4SSR 模板时引用的列表自动变成规则集。
- **真实可用性检查**：区分 Ping、TCP/UDP 端口探测与实际代理 URL 测试。开启 TUN 时，节点服务器地址不经过 TUN、节点域名用国内 DNS 直接解析，所以 Ping 和端口探测始终直连各节点，不受当前节点影响。
- **运行时切换与容错**：运行中切换节点；当前节点失效时自动切换到延迟最低的可用节点，可随时关闭。
- **云端备份**：通过 WebDAV（坚果云、Nextcloud、Alist、NAS 等）备份手动节点、订阅地址、规则和设置，可加密，可在内容变化时自动备份。
- **流量分析**：按域名或关键词查看经过 TUN 的 HTTP/HTTPS 请求，并保存为 JSON。
- **便携发布包**：GitHub Actions 构建 Windows、Linux、macOS 的 amd64 与 arm64 版本。
- **三种主题**：深色、浅色和毛玻璃，右上角按钮循环切换；毛玻璃效果在各平台表现一致。
- **程序更新**：启动时及每 6 小时检查 GitHub Releases，有新版本时右上角显示提示。桌面版点提示或左下角“更新程序”即可下载、校验 SHA-256、替换程序文件，确认后重启生效（也可下次打开时再生效），代理开着时重启后自动重连；sing-box、规则和配置保持不变。安卓版点提示打开下载页。

## 界面预览

![运行概览](docs/screenshots/sbtun-01-overview.png)
![节点管理](docs/screenshots/sbtun-02-nodes.png)
![路由模式](docs/screenshots/sbtun-03-routing.png)
![分流规则](docs/screenshots/sbtun-04-rules.png)
![流量分析](docs/screenshots/sbtun-05-traffic.png)

以上为深色主题下的五个导航页面。截图取自空配置环境，因此没有真实节点、流量和曲线；实际使用时各页面会显示实时数据。

---

## 快速开始

### Windows

1. 在本仓库的 **Releases** 页面下载 `sbtun-windows-amd64.zip` 或 `sbtun-windows-arm64.zip`。
2. 完整解压压缩包，不要只复制 `sbtun.exe`。
3. 使用管理员权限运行：

```powershell
.\sbtun.exe
```

命令行入口：

```powershell
.\sbtun-cli.exe help
.\sbtun-cli.exe status
```

Windows 发布包包含 sing-box、Wintun DLL 和默认分流规则集。TUN 模式需要管理员权限。

### Linux

1. 在本仓库的 **Releases** 页面下载对应架构的 `.tar.gz` 发布包。
2. 解压后使用启动脚本：

```bash
chmod +x start-sbtun.sh
./start-sbtun.sh
```

脚本会优先通过 `pkexec` 获取权限，不可用时尝试 `sudo`。命令行入口：

```bash
chmod +x sbtun-cli
./sbtun-cli help
```

> [!NOTE]
> Linux GUI 依赖 GTK3/WebKitGTK。官方发布包带有配套运行库和启动脚本；只复制 GUI 主程序可能导致 `libwebkit2gtk-4.0.so.37` 缺失。

### macOS

1. 在本仓库的 **Releases** 页面下载 `sbtun-macos-arm64.zip`（Apple 芯片）或 `sbtun-macos-amd64.zip`（Intel）。
2. 解压后把 `sbtun.app` 拖进“应用程序”。
3. 发布包没有苹果开发者签名，第一次打开前在终端执行：

```bash
xattr -dr com.apple.quarantine /Applications/sbtun.app
```

4. 打开 sbtun，输入管理员密码。创建 TUN 网卡和修改系统 DNS 都需要管理员权限，每次启动都会询问。

命令行入口在 App 里面：

```bash
sudo /Applications/sbtun.app/Contents/MacOS/sbtun-cli help
```

> [!NOTE]
> macOS 上开启 TUN 时，sbtun 会把所有网络服务的 DNS 临时设为 `172.18.0.2`，由 TUN 接管 DNS，关闭 TUN 或退出时恢复原设置。如果程序被强制结束导致无法上网，重新打开 sbtun，或运行 `sudo sbtun-cli run` 后再停止，就会恢复 DNS。macOS 版没有菜单栏图标，关闭窗口即退出并关闭 TUN。

### Android

1. 在 **Releases** 页面下载 APK：大多数手机用 `sbtun-android-arm64-v8a.apk`，老的 32 位手机用 `armeabi-v7a`，不确定就用 `universal`（体积大一些）。需要 Android 7.0 及以上。
2. 安装后打开，点概览页中间的连接按钮，第一次会弹出系统的 VPN 授权，允许即可。
3. 连接后通知栏会常驻“sbtun 已连接”，可以直接在通知里断开。

安卓版界面和桌面版一样（节点、订阅、路由模式、分流规则、自动切换、云端备份），手机竖屏用底部标签栏，平板和横屏用侧栏。没有流量分析（抓包需要安装证书）和 WebRTC 防泄漏开关，内核随 APK 一起更新。

自己构建时，在仓库 Settings → Secrets and variables → Actions 里添加 `ANDROID_KEYSTORE_BASE64`（PKCS12 密钥库的 base64）和 `ANDROID_KEYSTORE_PASSWORD`，之后每次构建签名一致，新版可以直接覆盖安装。别名默认 `sbtun`，可用 `ANDROID_KEY_ALIAS`、`ANDROID_KEY_PASSWORD` 另行指定。没配置时用临时签名，升级前要先卸载旧版。

> [!NOTE]
> 安卓上 VPN 由系统创建，sbtun 用 tun2socks 把流量转给内置的 sing-box。sbtun 自己（包括订阅更新、节点测速、云端备份）不走 VPN，直接联网。

### 首次使用

1. 打开“节点管理”，导入单个节点链接、订阅地址，或手动填写协议参数。导入的订阅会显示在节点列表上方，默认每 24 小时自动拉取最新节点（可改间隔或关闭），也可随时一键更新。安卓在“粘贴链接导入”里点“扫码”，用摄像头扫节点或订阅二维码直接导入。
2. 选择一个节点并执行健康测试。
3. 在“路由模式”中选择所需策略。
4. 检查规则集状态，必要时执行更新。
5. 点击“开启 TUN”。

只有 sing-box 进程、TUN 和必要的就绪检查全部成功后，界面才会显示“服务运行中”。启动失败时会报告错误并清理已启动的进程。

---

## 支持平台

| 平台 | GUI | CLI | 架构 |
| --- | --- | --- | --- |
| Windows | 支持 | 支持 | amd64、arm64 |
| Linux | 支持 | 支持 | amd64、arm64 |
| macOS | 支持 | 支持 | Intel（amd64）、Apple 芯片（arm64） |
| Android 7.0+ | 支持 | — | arm64-v8a、armeabi-v7a、x86_64 |

## 支持的节点类型

| 协议 | 链接/订阅导入 | 手动配置 |
| --- | --- | --- |
| VLESS | 支持 | 支持 |
| VMess | 支持 | 支持 |
| Trojan | 支持 | 支持 |
| Shadowsocks | 支持 | 支持 |
| Hysteria2 | 支持 | 支持 |
| Hysteria（第一版） | 支持（`hysteria://`） | 支持 |
| TUIC | 支持 | 支持 |
| AnyTLS | 支持 | 支持 |
| WireGuard | 支持（`wireguard://` / `wg://`） | 支持 |
| SOCKS | 支持 | 支持 |
| HTTP/HTTPS | 支持 | 支持 |

VLESS、VMess 和 Trojan 可配置 TLS、SNI、ALPN、uTLS 及 WebSocket、HTTP/2（`h2`）、gRPC、HTTPUpgrade 等传输参数，`security=none` 的链接不会开启 TLS；Hysteria2 支持端口跳跃（含 `host:443,20000-30000` 多端口写法、省略端口默认 443）、`user:pass` 认证与 Salamander 混淆等参数；Shadowsocks-2022 链接按 SIP002 的百分号编码写法导入导出；`https://` 代理链接按开启 TLS 的 HTTP 代理处理；TUIC 支持拥塞控制与 UDP 转发模式；WireGuard 按 sing-box 1.11+ 的 endpoint 方式生成。

VLESS、VMess、Trojan 链接也能识别 Shadowrocket 导出的写法（`peer=`、`obfs=websocket`、`obfsParam=` 等参数）。Hysteria 第一版只支持 udp 协议（faketcp、wechat-video 会跳过），没写上下行带宽时按上行 20 Mbps、下行 100 Mbps。

订阅地址除了常见的 base64 分享链接列表，也能直接识别 Clash / Mihomo YAML（`proxies:`）、sing-box JSON（`outbounds` / `endpoints`）、SIP008（Shadowsocks 的 JSON 订阅）和 Surge / Loon / Quantumult X 的节点配置行，自动挑出其中支持的节点，分组、规则等内容会被忽略。单独一行 Surge / Loon / Quantumult X 节点配置也可以直接粘贴导入。

下载订阅时依次用 Clash Meta、sing-box、v2rayN 的 User-Agent，哪个拿到节点就用哪个（很多机场按 UA 决定返回什么格式）；网络不通时不会重复尝试。订阅更新后，如果当前节点的参数变了，会继续选中同名（或同地址）的节点。自动更新在 sbtun 运行期间每 10 分钟检查一次，到期的订阅才下载；下载失败保留原节点，一小时后再试。

机场在订阅响应头里给出流量和到期信息（`subscription-userinfo`）时，订阅卡片和 `sbtun subs` 会显示剩余流量、总量和到期日；已过期、7 天内到期或剩余流量不足 10% 时醒目提示。每次更新订阅时刷新。

sing-box 不支持的内容会跳过：ShadowsocksR、Snell、xhttp / kcp 等传输、带 TLS 的 SOCKS、TUIC v4、shadow-tls 等 SS 插件。生成运行配置时每个节点会先做一遍校验，参数明显不合法（如 2022 密钥长度不对、REALITY 公钥无效）的备用节点会被跳过，不影响当前节点启动。

---

## 路由模式

| 模式 | 行为 | 适用场景 |
| --- | --- | --- |
| 智能分流 | 国内直连，国外代理 | 日常使用 |
| 全局代理 | 除局域网外全部代理 | 临时全局代理 |
| 全局直连 | 全部直连，只保留屏蔽规则 | 排障或暂停代理 |

模式只决定内置的国内/国外规则集用不用、匹配不到时走代理还是直连。自己的规则不分模式：在智能和全局下优先生效，在全局直连下只有去向为“阻断”的生效。旧版的“自定义”模式会自动改为全局代理，行为不变。

匹配顺序（命中即停）：手动 DNS 过滤 → 分流规则 → 自己添加的规则集 → 内置国内/国外规则集（仅智能分流）→ 局域网直连 → 兜底。

分流规则支持：

- 域名后缀
- 域名关键词
- 完整域名
- IP/CIDR
- 端口

每条规则、每个自己添加的规则集的去向可以是：

- `proxy`（当前节点）、`direct`、`block`；
- 指定节点 `node:<ID>`；
- 节点组 `group:<ID>`、订阅 `sub:<ID>`、地区 `region:US`、协议 `proto:hysteria2`：组里自动用延迟最低的节点，节点挂了自动换下一个。

用户规则优先于内置分流规则。指定的节点、节点组被删除，或组里没有可用节点时，这条规则暂走当前节点，界面会标红提示。改规则只会重载 sing-box，不会重建 TUN。

**节点组**：按机场（订阅）、地区、协议、名称包含 / 排除组合挑节点。同一项里选多个是“任一”，不同项之间是“同时满足”，不选的项不限；订阅更新后组里的节点自动增减。

**节点地区**：第一次测速成功时会经该节点查一次落地 IP 和归属地（之后不再重复查，节点参数变了才重查）；还没测过的节点按名字里的国旗、地名推测。节点列表可以按地区筛选、分组。

**批量导入**除了本软件的 `proxy,domain_suffix,example.com` 写法，也认 Clash / Mihomo（含 YAML 的 `- ` 列表）、Surge、Quantumult X 的 `DOMAIN-SUFFIX,openai.com,策略` 规则行。策略是 `DIRECT` / `REJECT` 时对应直连 / 阻断；和节点组名、节点名、订阅名、协议（如 `hy2`）相同，或能认出地区（如 `🇺🇸 美国`）时走对应的节点；其余策略（如 `🚀 节点选择`）走当前节点。引用外部列表的写法（Clash 的 `RULE-SET` + `rule-providers`、Surge 的 `RULE-SET` / `DOMAIN-SET`、subconverter / ACL4SSR 模板的 `ruleset=`）会把每个列表下载成一个规则集，策略换成它的去向，在“规则”页整份更新、整份修改；`GEOIP`、`MATCH` 等没有对应的规则会跳过。

### 网络选项

- **节点失效自动切换**（默认开启）：TUN 运行时每 30 秒测试一次当前节点，间隔 5 秒连续两次失败才判定失效。随后并发测试其他节点（最多 4 个同时测），优先选同一订阅里延迟最低的；刚失效的节点 5 分钟内不会再被选中。找不到可用节点时检查间隔逐步拉长到 5 分钟。切换后界面会弹出提示，状态栏显示“已自动切换”，托盘提示也会写明从哪个节点切到哪个。后台测出的延迟会同步到节点列表。托盘菜单和 `sbtun auto-switch on|off` 也能开关。
- **IPv6**：检测到可用 IPv6 时加入 TUN 地址；不可用时自动使用 IPv4。
- **WebRTC 防泄漏**：启用更严格的 TUN 路由，减少 WebRTC 直连暴露地址的风险。
- **诊断信息**：显示配置节点与 sing-box 实际 selector，用于排查状态不同步。

### 云端备份（WebDAV）

节点页右上角“云端备份”。填 WebDAV 地址、用户名和密码即可，坚果云、Nextcloud、Alist、群晖等都能用（坚果云地址填 `https://dav.jianguoyun.com/dav/sbtun/`，密码用“第三方应用密码”）。

- **备份内容**：手动添加的节点、订阅地址、分流规则、规则集列表和各项设置。订阅里的节点不备份，恢复时按订阅地址重新拉取；拉取失败的订阅保留本机已有的节点。
- **加密**：填了加密密码，备份用 AES-256-GCM 加密（文件后缀 `.sbtun`），恢复时要用同一个密码；不填则是明文 JSON，里面有节点密码。
- **自动备份**：开启后 sbtun 运行时每分钟检查一次，只有手动节点、订阅地址、规则或设置变了才上传，并且等改动停下一分钟后再传。切换节点、订阅节点更新都不算变化。
- **恢复**：从列表里选一份，整体覆盖本机配置（本机的 WebDAV 设置保留）。云端保留最近 10 份，文件名带时间和电脑名。

---

## 规则集与广告过滤

sbtun 可以管理分流规则集：

- 默认包含国内域名、国内 IP 和非国内域名规则集，只在智能分流下使用。
- 可以添加 `.srs`，也可以直接填 Clash / Surge 规则列表、每行一个域名或 IP 段的列表、AdGuard `||example.com^` 列表、hosts 文件的链接，文本列表会转换成 sing-box 源格式（`.json`）。
- 自己添加的规则集默认去向是阻断（广告过滤），也可以改成代理、直连或节点组、地区等；整份生效、整份更新，想让其中某个网站例外，就加一条分流规则，它优先。
- 单个规则集更新失败时保留旧文件，避免把可用规则替换成损坏文件。
- 停用只改变启用状态；删除会同时移除记录和本地文件。
- 手动 DNS 过滤规则优先级最高。

添加外部规则集时，请填写可直接下载的文件地址（GitHub 用 raw 链接），而不是仓库首页。广告过滤基于域名规则，不修改网页 CSS，也无法覆盖所有应用内广告；实际效果取决于上游列表。

---

## 节点健康测试

sbtun 不把 Ping 结果直接当作节点可用：

- **Ping**：检查目标主机基础连通性。
- **TCPing**：检查 TCP 协议节点的服务端口。
- **UDP**：检查 Hysteria2、TUIC、WireGuard 等 UDP 节点是否可发送数据。
- **URL**：通过 sing-box 临时出站或运行中的 selector 发起实际代理请求。

节点最终健康状态以 URL 测试为主。URL 失败也可能来自 DNS、测试目标或当前网络环境，建议结合各项结果判断。

界面中每个节点右侧显示延迟：优先使用 URL 延迟，没有 URL 结果时用 TCPing；绿、黄、红分别对应低于 300 ms、低于 800 ms 和更慢或不可用，鼠标悬停可看各项明细。没有勾选节点时，“测试全部”会测试当前筛选出的所有节点（同时最多 4 个）。

---

## HTTP/HTTPS 流量分析

流量分析器按域名或关键词记录经过 TUN 的 HTTP/HTTPS 请求。输入单独一行 `*` 可以分析全部 HTTP/HTTPS 流量。

响应边转发边记录，SSE 和大文件下载不会因为分析而延迟；记录先保存在内存中，大约每秒批量写入一次 `capture.json`，停止 TUN 时会写入剩余记录。

```bash
# 分析指定域名或关键词，并前台启动 TUN
./sbtun-cli capture run example.com

# 分析全部 HTTP/HTTPS 请求
./sbtun-cli capture run '*'

# 保存规则、查看状态与记录
./sbtun-cli capture enable '*'
./sbtun-cli capture status
./sbtun-cli capture list
./sbtun-cli capture show 1

# 安装或卸载 HTTPS 分析证书
./sbtun-cli capture cert install
./sbtun-cli capture cert uninstall

# 删除已保存记录
./sbtun-cli capture clear
```

### 能分析什么

- HTTP 请求头及有限大小的请求/响应正文。
- 安装并信任 sbtun 根证书后的 TCP HTTPS 流量。
- 命中指定域名、关键词或 `*` 规则的请求。

### 不能分析什么

- 原始 PCAP 数据。
- SSH、游戏协议或任意自定义 TCP/UDP 协议。
- 未回退到 TCP 的 HTTP/3/QUIC 流量。
- 使用证书固定且拒绝本地分析证书的应用。

> [!WARNING]
> HTTPS 分析会在本机生成并使用根证书。只应在你拥有或获准调试的设备和流量上启用。`runtime-data/capture/ca.key` 是私钥，不要分享、提交到 Git 或上传到公共位置。

---

## CLI

直接运行 `sbtun-cli` 或执行 `sbtun-cli help` 可以查看完整帮助。

### 运行与状态

```text
run                             前台启动 TUN 与代理
start                           同 run
stop                            停止运行中的实例
status                          查看当前状态
version                         查看 sbtun 版本并检查是否有新版本
```

### 节点管理

```text
nodes                           列出节点
info <编号>                     查看节点详情
add-node <节点链接或订阅地址>   导入节点
switch <编号>                   切换节点
test [编号...]                  测试 Ping、端口和 URL 延迟，省略编号则测试全部（最多 4 个并行）
export [编号...]                导出分享链接，省略编号则导出全部
qr <编号>                       在终端显示节点分享链接二维码
del <编号...>                   删除一个或多个节点
edit [编号]                     交互式编辑节点
edit <编号> <节点链接>          用链接覆盖节点参数
subs                            列出订阅
subs update [编号]              更新订阅，省略编号则更新全部
subs delete <编号>              删除订阅及其全部节点
subs auto <小时|off>            订阅自动更新间隔（默认 24 小时），off 关闭
```

### 路由与规则

```text
route <smart|global|direct>     自己的规则在 smart、global 下优先，direct 下只保留阻断
auto-switch <on|off>
rules list
rules add <名称> <链接> [去向]  .srs、Clash/Surge 规则列表或域名列表；去向默认 block
rules edit <ID> <名称> <链接> [去向]
rules action <ID> <去向>        只改去向，写法同 custom add
rules delete <ID>
rules enable|disable <ID>       启用或停用规则集
rules restore                   恢复默认规则集
rules update <ID>
rules update-all
add-rule <链接|文件|文本>       批量导入分流规则；引用的 RULE-SET 列表各自成为规则集
custom                          列出自定义分流规则
custom add <类型> <内容> <去向> 类型 domain_suffix/domain_keyword/domain/ip_cidr/port；去向 proxy/direct/block，
                                或节点组名、节点名、订阅名、协议（hy2）、地区（美国 / US）
groups                          列出节点组
groups add <名称> [条件...]     条件 region=US,JP sub=订阅名 proto=hy2,vless keyword=IPLC exclude=过期
groups delete <编号>
custom edit <编号> <类型> <内容> <去向>  修改分流规则，参数同 add
custom delete <编号>
filter                          列出 DNS 过滤规则
filter add <类型> <内容>        添加 DNS 过滤（阻断）规则
filter edit <编号> <类型> <内容>
filter delete <编号>
```

### 网络选项

```text
set ipv6 <on|off>               IPv6
set webrtc <on|off>             WebRTC 防泄漏
set diagnostics <on|off>        诊断信息
diag                            查看运行诊断
```

### 云端备份

```text
backup set <地址> <用户名> <密码> [加密密码]   设置并测试 WebDAV
backup                          立即备份
backup list                     列出云端备份
backup restore <编号>           用云端备份覆盖本机配置
backup delete <编号>            删除一份云端备份
backup auto <on|off>            内容有变化时自动备份（sbtun 运行时生效）
backup clear                    清除 WebDAV 设置
```

### 程序与内核更新

```text
upgrade                         更新 sbtun 程序本体到最新版
kernel version                  查看 sing-box 版本
kernel update                   更新到官方最新稳定版
```

运行中修改节点、路由或规则时，sbtun 会同步配置并重载运行时。更新 sing-box 前需要先停止当前实例。`upgrade` 只替换 sbtun 程序文件（Windows/Linux 为 sbtun、sbtun-cli 等，macOS 为整个 sbtun.app），已打开的界面或 `sbtun run` 重新启动后生效。

---

## 数据与目录

sbtun 使用便携式目录结构，配置和运行数据默认保存在可执行文件旁边。macOS 例外：App 可能运行在只读位置，sing-box、规则集和运行数据统一保存在 `/Library/Application Support/sbtun/`，GUI 和 CLI 共用，首次运行时从 App 里复制内置的 sing-box 和规则集。

```text
sbtun/
├── sbtun(.exe)                 GUI
├── sbtun-cli(.exe)             CLI
├── sing-box(.exe)              代理内核
├── rules/
│   ├── sets.json               规则集状态
│   └── *.srs / *.json          规则集文件（文本列表转换后为 .json）
└── runtime-data/
    ├── config.json             用户配置
    ├── webdav-backup.json      上次自动备份的内容指纹
    ├── local-auth.json         本地 clash API 与分析代理的随机凭据
    ├── runtime/                sing-box 运行配置
    └── capture/
        ├── capture.json        HTTP/HTTPS 分析记录
        ├── ca.crt              分析根证书
        └── ca.key              分析根证书私钥
```

> [!IMPORTANT]
> `runtime-data/config.json` 可能包含节点地址、认证参数和订阅相关信息。备份时请妥善保管，不要把 `runtime-data/` 提交到公开仓库。

---

## 从源码构建

### 环境要求

- Go 1.25 或更高版本
- Node.js 22 或更高版本
- Wails CLI 2.15 或更高版本
- Linux GUI：GTK3、WebKitGTK 4.x 和 `pkg-config`

### 构建 GUI

```bash
go install github.com/wailsapp/wails/v2/cmd/wails@v2.15.0
wails build
```

Wails 会按照 `wails.json` 安装并构建前端依赖。

> [!NOTE]
> Ubuntu 24.04 的软件源只提供 WebKitGTK 4.1，不再提供 Wails 默认查找的 4.0。需要安装 `libwebkit2gtk-4.1-dev`，构建时加上 tag：
>
> ```bash
> sudo apt install libgtk-3-dev libwebkit2gtk-4.1-dev pkg-config
> wails build -tags webkit2_41
> ```
>
> Ubuntu 22.04 及更早版本使用 `libwebkit2gtk-4.0-dev`，直接 `wails build` 即可。

### 构建 CLI

```bash
go build -tags cli -o sbtun-cli .
```

Linux 交叉编译 CLI：

```bash
GOOS=linux GOARCH=amd64 go build -tags cli -o sbtun-cli-linux-amd64 .
GOOS=linux GOARCH=arm64 go build -tags cli -o sbtun-cli-linux-arm64 .
```

GUI 构建依赖目标平台的 Wails 和系统图形库。完整便携发布包建议交给仓库中的 GitHub Actions 工作流构建。

---

## 开发与验证

提交前建议执行：

```bash
go test ./...
go test -tags cli ./...
go vet ./...
go vet -tags cli ./...
```

项目实现遵循这些原则：

- TUN 未真实就绪时，不显示为“运行中”。
- sing-box 配置先生成、校验，再启动。
- 启动或重载失败时，清理运行状态并尽可能回滚配置。
- 用户配置与 sing-box 运行配置分离。
- 节点测试区分 ICMP、TCP、UDP 和实际代理请求。
- GUI 与 CLI 使用相同的核心逻辑。

### 技术栈

- Go
- Wails 2
- Vite
- sing-box
- goproxy（HTTP/HTTPS 流量分析）

---

## 发布

仓库工作流构建以下发布包：

- Windows amd64 / arm64
- Linux amd64 / arm64
- GUI 与 CLI
- 对应平台的 sing-box、规则集及必要运行资源

推送 `v*` 或 `0.*` 格式的 tag 会触发 GitHub Release；普通分支推送只执行构建检查。

## License

MIT License，详见 [LICENSE](LICENSE) 文件。

第三方组件（sing-box、Wails、Wintun 等）仍分别受其自身许可证约束。界面里的国旗使用 [Twemoji](https://github.com/twitter/twemoji) 图形（经 [country-flag-emoji-polyfill](https://github.com/talkjs/country-flag-emoji-polyfill) 打包成字体），按 CC-BY 4.0 许可使用。
