//go:build android

// Package mobile 是安卓 APK 调用的 Go 接口（gomobile bind 生成 aar）。
//
// 界面沿用桌面版的网页，网页通过 Call 调用 app.App 的方法，和桌面版 Wails 绑定一样。
// VPN 由安卓 VpnService 建立，TUN 流量经 tun2socks 转给 sing-box 的本地入站。
package mobile

import (
	"context"
	"errors"
	"net/url"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/xjasonlyu/tun2socks/v2/core"
	"github.com/xjasonlyu/tun2socks/v2/core/device"
	"github.com/xjasonlyu/tun2socks/v2/core/device/fdbased"
	"github.com/xjasonlyu/tun2socks/v2/proxy/socks5"
	"github.com/xjasonlyu/tun2socks/v2/tunnel"
	"gvisor.dev/gvisor/pkg/tcpip/stack"

	"github.com/caichengle666/sbtun/app"
	"github.com/caichengle666/sbtun/core/tun"
)

var (
	initOnce sync.Once
	platform Platform

	vpnMu      sync.Mutex
	vpnRunning bool
	tunDevice  device.Device
	tunStack   *stack.Stack
)

// Init 在应用启动时调用一次。dataDir 是应用私有目录，binary 是 APK 里带的 sing-box。
func Init(dataDir, binary string, p Platform) {
	vpnMu.Lock()
	platform = p
	vpnMu.Unlock()
	initOnce.Do(func() {
		instance = app.New()
		tun.Platform = vpnPlatform{}
		instance.StartupMobile(context.Background(), dataDir, binary)
	})
}

// Running 报告 VPN 是否已建立。
func Running() bool {
	vpnMu.Lock()
	defer vpnMu.Unlock()
	return vpnRunning
}

type vpnPlatform struct{}

func (vpnPlatform) Start() error {
	vpnMu.Lock()
	defer vpnMu.Unlock()
	if vpnRunning {
		return nil // 重载配置时 VPN 不断开
	}
	if platform == nil {
		return errors.New("安卓 VPN 未初始化")
	}
	cfg, err := instance.LoadConfig()
	if err != nil {
		return err
	}
	if err := openVPN(platform, cfg.IPv6Enabled, startNetstack); err != nil {
		return err
	}
	vpnRunning = true
	return nil
}

func (vpnPlatform) Stop() error {
	vpnMu.Lock()
	defer vpnMu.Unlock()
	if !vpnRunning {
		return nil
	}
	vpnRunning = false
	stopNetstack() // 会关闭 TUN 文件描述符
	if platform != nil {
		return platform.CloseTun()
	}
	return nil
}

// startNetstack 用 gVisor 用户态协议栈接管 TUN，把 TCP/UDP 连接转给 sing-box 的 SOCKS5 入站。
// 不用 tun2socks 的 engine 包，因为它出错时直接退出进程。
// fd 交给 fdbased 设备之前出错要自己关掉，否则 VPN 接口一直被占着。
func startNetstack(fd int) error {
	owned := false
	defer func() {
		if !owned {
			_ = syscall.Close(fd)
		}
	}()
	proxyURL, err := url.Parse(instance.MobileProxyURL())
	if err != nil {
		return err
	}
	password, _ := proxyURL.User.Password()
	proxy, err := socks5.New(proxyURL.Host, proxyURL.User.Username(), password)
	if err != nil {
		return err
	}
	dev, err := fdbased.Open(strconv.Itoa(fd), tunMTU, 0)
	if err != nil {
		return err
	}
	owned = true // 之后由 dev.Close 负责关闭 fd
	tunnel.T().SetProxy(proxy)
	tunnel.T().SetUDPTimeout(time.Minute)
	st, err := core.CreateStack(&core.Config{LinkEndpoint: dev, TransportHandler: tunnel.T()})
	if err != nil {
		dev.Close()
		return err
	}
	tunDevice, tunStack = dev, st
	return nil
}

func stopNetstack() {
	if tunDevice != nil {
		tunDevice.Close()
	}
	if tunStack != nil {
		tunStack.Close()
		tunStack.Wait()
	}
	tunDevice, tunStack = nil, nil
}

// tunMTU 要和安卓 VpnService.Builder.setMtu 一致。
const tunMTU = 1500

// MTU 给安卓读取，保证两边一致。
func MTU() int { return tunMTU }
