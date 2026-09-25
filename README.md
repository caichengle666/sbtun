# sbtun

轻量级 sing-box TUN 客户端，提供 Windows 和 Linux 桌面 GUI，同时提供跨平台 CLI。

当前版本：`0.2.4`

## 功能

- 基于 sing-box 的 TUN 网络代理
- 节点和订阅管理
- 支持 VLESS、VMess、Trojan、Shadowsocks、Hysteria2、SOCKS、HTTP 等常用节点类型
- 智能分流、全局代理、全局直连和自定义规则
- 自定义规则优先于内置分流规则
- 国内域名和国内 IP 规则集自动更新
- 节点 Ping、TCP/UDP 端口和 URL 健康测试
- 多节点批量测试，单个节点完成后立即显示结果
- 节点失效检测和自动切换
- Windows 托盘运行、主题切换、复制粘贴和右键编辑操作
- HTTP/HTTPS 请求分析器，可保存分析结果到 JSON
- CLI 管理节点、路由规则、抓包分析和运行状态
- GitHub Actions 自动构建 Windows/Linux 发布包

## 支持平台

| 平台 | GUI | CLI | 架构 |
| --- | --- | --- | --- |
| Windows | 支持 | 支持 | amd64、arm64 |
| Linux | 支持 | 支持 | amd64、arm64 |
| macOS | 暂不提供构建 | 暂不提供构建 | - |

Windows GUI 使用 Wintun。Linux GUI 使用 GTK/WebKitGTK，TUN 模式通常需要 root 权限或系统授权工具。

## 下载

打开 GitHub Releases，按操作系统和架构下载对应压缩包。

发布包会自动包含或准备以下运行资源：

- 最新 sing-box
- Windows Wintun DLL
- 国内域名和国内 IP 规则集
- Linux GUI 所需的 WebKitGTK 运行库及辅助进程
- 对应平台的 GUI、CLI 和启动脚本

## GUI 使用

1. 启动 `sbtun`。
2. 在“节点”页面导入节点链接、订阅，或使用协议字段手动添加节点。
3. 在“路由模式”中选择智能分流、全局代理、全局直连或自定义规则。
4. 检查规则集状态，必要时更新规则集。
5. 点击启动 TUN。
6. 需要停止时点击“关闭 TUN”，关闭窗口不会把运行中的 TUN 状态伪装成已停止。

启动状态只有在 sing-box、TUN 和必要的网络检查成功后才会显示为运行中。启动失败时会显示错误并清理已启动的进程。

## 路由模式

### 智能分流

国内域名和国内 IP 直连，其余流量通过当前代理节点。

### 全局代理

除本机保留地址等必要例外外，流量通过代理节点。

### 全局直连

流量直接连接，不经过代理节点。

### 自定义规则

按用户规则控制流量。自定义规则命中后优先使用自定义处理方式，其他规则不会覆盖已经命中的自定义规则。

规则支持域名后缀、域名关键词、完整域名、IP/CIDR 等匹配对象，并可选择直连或代理。

## 节点健康测试

节点卡片中的测试会根据协议选择合适的方法：

- Ping：测试基础网络连通性。
- TCPing：适用于 TCP 服务端口。
- UDP：适用于 Hysteria2 等 UDP 服务端口。
- URL：通过临时 sing-box 出站访问测试地址，验证节点是否真的能代理请求。

Hysteria2 不使用 TCPing 代替 UDP 检查。URL 测试失败可能来自节点、DNS、目标站点或当前网络环境，不能只根据 Ping 结果判断节点可用。

## HTTP/HTTPS 流量分析

流量分析用于查看经过 TUN 的 HTTP/HTTPS 请求，不是通用 PCAP 抓包器。

### 使用方式

1. 打开“流量分析”。
2. 在分析规则输入框中填写域名或关键词，每行一条。
3. 如果不知道目标域名，可以输入单独一行 `*`，表示分析所有经过 TUN 的 HTTP/HTTPS 请求。
4. 点击“保存并应用”，再产生网络流量。
5. HTTPS 请求需要先安装程序生成的根证书。
6. 请求记录会保存到运行目录下的 `runtime-data/capture/capture.json`。

支持的 CLI 示例：

```powershell
# 分析指定域名或关键词
./sbtun-cli capture run example.com

# 分析全部 HTTP/HTTPS 请求，* 建议加引号
./sbtun-cli capture run '*'

# 启用分析规则
./sbtun-cli capture enable '*'

# 查看分析状态和已保存记录
./sbtun-cli capture status
./sbtun-cli capture list
./sbtun-cli capture show 1

# 安装或卸载 HTTPS 分析证书
./sbtun-cli capture cert install
./sbtun-cli capture cert uninstall

# 删除已保存记录
./sbtun-cli capture clear
```

Linux/macOS shell 使用相同命令格式，将 `./sbtun-cli` 替换为实际文件路径即可。

### 分析范围和限制

- HTTP 请求可以记录请求头和有限大小的请求/响应正文。
- HTTPS 请求需要安装并信任 sbtun 根证书，且应用必须使用可被代理分析的 TCP HTTPS 流量。
- HTTP/3 或 QUIC 使用 UDP 443 时不会直接解析为 HTTP；全量 HTTP/HTTPS 分析会引导其回退到 TCP HTTPS。
- SSH、游戏协议、任意 TCP/UDP、自定义二进制协议不会变成 HTTP 请求记录。
- `*` 表示全部 HTTP/HTTPS 分析，不表示所有网络协议，也不等于导出原始 PCAP。
- 抓包记录是 JSON 文件，保存后可以复制、分析或直接删除。

## CLI

直接运行 CLI 或使用不完整参数时，会显示帮助信息。

```text
sbtun-cli help
sbtun-cli version
```

常用命令：

```text
run                         启动 TUN
start                       启动 TUN
stop                        停止 TUN
status                      查看运行状态

nodes                       列出节点
info <编号>                 查看节点详情
add-node <链接或订阅>       添加节点或订阅
switch <编号>               切换当前节点
test [编号...]              测试一个或多个节点
del <编号>                  删除节点
edit <编号>                 交互式编辑节点

route                       查看当前路由模式
add-rule                    交互式添加自定义规则
rules list                  列出规则集
rules add <名称> <SRS_URL>  添加规则集
rules edit <ID> <名称> <URL> 编辑规则集
rules delete <ID>           删除规则集
rules update <ID>           更新指定规则集
rules update-all            更新全部规则集

capture run <规则>          启动流量分析
capture enable <规则>       启用流量分析
capture disable             停止流量分析
capture status              查看分析状态
capture list                列出请求记录
capture show <编号>         查看请求详情
capture clear               删除抓包记录
capture cert install        安装分析证书
capture cert uninstall      卸载分析证书
```

`edit` 需要交互式终端。节点测试会按完成顺序输出结果，不必等待全部节点结束才看到第一个结果。

## 数据目录

运行数据默认放在可执行文件旁边的 `runtime-data` 目录：

```text
runtime-data/
├── config.json                  用户配置
├── runtime/                     sing-box 运行配置
├── rules/                       规则集文件
└── capture/
    ├── capture.json             HTTP/HTTPS 分析记录
    ├── ca.crt                   分析根证书
    └── ca.key                   分析根证书私钥
```

不要把包含私钥的 `runtime-data/capture` 目录上传到公共仓库。

## Windows 运行

解压后使用管理员权限运行 GUI：

```powershell
.\sbtun.exe
```

CLI：

```powershell
.\sbtun-cli.exe help
.\sbtun-cli.exe status
```

如果启用 TUN 时提示网卡不存在，请确认 Wintun DLL 与程序位数一致，并使用管理员权限启动。

## Linux 运行

GUI 发布包推荐使用启动脚本，它会优先尝试 `pkexec`，不可用时再使用 `sudo`：

```bash
chmod +x start-sbtun.sh
./start-sbtun.sh
```

CLI：

```bash
chmod +x sbtun-cli
./sbtun-cli help
```

如果系统提示 `webkit2gtk-4.0` 或 `libwebkit2gtk-4.0.so.37` 缺失，请使用 GitHub Releases 中与发行包配套的启动脚本和运行库，不要只复制 GUI 主程序。

## 从源码构建

### 环境要求

- Go 1.25 或更高版本
- Node.js 22 或更高版本
- Wails CLI 2.15 或更高版本
- Linux GUI 构建需要 GTK3、WebKitGTK 4.0 和 `pkg-config`

### GUI

```bash
wails build
```

### CLI

```bash
go build -tags cli -o sbtun-cli .
```

Linux 交叉编译 CLI 示例：

```bash
GOOS=linux GOARCH=amd64 go build -tags cli -o sbtun-cli-linux-amd64 .
GOOS=linux GOARCH=arm64 go build -tags cli -o sbtun-cli-linux-arm64 .
```

GUI 构建需要目标平台的 Wails、GTK/WebKitGTK 和系统打包环境；Windows 和 Linux 的完整发布包由 GitHub Actions 生成。

## GitHub Actions 发布

仓库工作流会分别构建 Windows amd64/arm64、Linux amd64/arm64 的 GUI 和 CLI，并准备对应运行资源。构建时会从官方发布接口获取最新 sing-box，Windows 获取最新 Wintun，规则集使用 SRS 文件。

推送 `0.*` 或 `v*` 格式的 tag 后，工作流会构建压缩包并创建 GitHub Release。普通分支推送只进行对应的构建检查，不会自动替换已有 release。

## 开发与验证

提交修改前建议执行：

```bash
go test ./...
go test -tags cli ./...
go vet ./...
go vet -tags cli ./...
```

项目坚持以下原则：

- TUN 未真实启动时不显示“运行中”。
- sing-box 配置先校验，再启动；失败时清理并恢复状态。
- 用户配置和 sing-box runtime 配置分离。
- 自定义规则优先级明确，不依赖前端显示顺序。
- 网络测试区分 TCP、UDP 和实际代理 URL 测试。

## License

当前仓库未声明独立开源许可证。分发或二次开发前，请以仓库中的许可证文件和第三方组件许可证为准。
