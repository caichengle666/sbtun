package mobile

import (
	"errors"
	"testing"
)

type fakePlatform struct {
	openErr error
	closed  int
}

func (p *fakePlatform) OpenTun(bool) (int32, error) { return 42, p.openErr }
func (p *fakePlatform) CloseTun() error             { p.closed++; return nil }

// 建立 VPN 或启动 tun2socks 失败时必须关掉已拉起的前台服务；成功时不关。
func TestOpenVPNClosesServiceOnFailure(t *testing.T) {
	p := &fakePlatform{openErr: errors.New("establish 返回 null")}
	if err := openVPN(p, false, func(int) error { t.Fatal("OpenTun 失败后不应启动 netstack"); return nil }); err == nil || p.closed != 1 {
		t.Fatalf("OpenTun 失败: err=%v closed=%d", err, p.closed)
	}
	p = &fakePlatform{}
	if err := openVPN(p, false, func(fd int) error { return errors.New("netstack 失败") }); err == nil || p.closed != 1 {
		t.Fatalf("netstack 失败: err=%v closed=%d", err, p.closed)
	}
	p = &fakePlatform{}
	got := 0
	if err := openVPN(p, true, func(fd int) error { got = fd; return nil }); err != nil || p.closed != 0 || got != 42 {
		t.Fatalf("成功路径: err=%v closed=%d fd=%d", err, p.closed, got)
	}
}
