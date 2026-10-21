package mobile

import "fmt"

// Platform 由安卓实现：OpenTun 建立 VPN 并返回 TUN 文件描述符（所有权交给 Go），CloseTun 关闭 VPN 服务。
type Platform interface {
	OpenTun(ipv6 bool) (int32, error)
	CloseTun() error
}

// openVPN 建立 VPN 并交给 start 接管文件描述符。OpenTun 会先拉起前台服务和"已连接"通知，
// 所以任何一步失败都要 CloseTun 收掉服务，不能留下实际没有连接的通知。
func openVPN(p Platform, ipv6 bool, start func(fd int) error) error {
	fd, err := p.OpenTun(ipv6)
	if err != nil {
		_ = p.CloseTun()
		return fmt.Errorf("建立 VPN 失败: %w", err)
	}
	if err := start(int(fd)); err != nil {
		_ = p.CloseTun()
		return fmt.Errorf("启动 tun2socks 失败: %w", err)
	}
	return nil
}
