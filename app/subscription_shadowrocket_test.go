package app

import (
	"encoding/base64"
	"reflect"
	"testing"
)

func TestParseShadowrocketLinks(t *testing.T) {
	vless := "vless://bm9uZTozMGU0OTQzYi03MzdlLTRhNzYtYTk2Mi03MGE3NzdiN2JhMzhAY2YuMTEwMC5jY3d1LmNjOjQ0Mw?path=/vless-tunnel&remarks=amo-sandbox-%E4%BC%98%E9%80%891&obfsParam=amo-sandbox.021800.xyz&obfs=websocket&tls=1&peer=amo-sandbox.021800.xyz&udp=1"
	n, err := parseSubscriptionLine(vless)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"uuid": "30e4943b-737e-4a76-a962-70a777b7ba38", "transport_type": "ws", "transport_host": "amo-sandbox.021800.xyz", "transport_path": "/vless-tunnel", "tls": "true", "server_name": "amo-sandbox.021800.xyz"}
	if n.Protocol != "vless" || n.Server != "cf.1100.ccwu.cc" || n.Port != 443 || n.Name != "amo-sandbox-优选1" || !reflect.DeepEqual(n.Settings, want) {
		t.Fatalf("vless=%+v", n)
	}

	auth := base64.RawURLEncoding.EncodeToString([]byte("auto:11111111-2222-3333-4444-555555555555@1.2.3.4:8443"))
	vmess := "vmess://" + auth + "?remarks=vm&obfs=grpc&path=svc&tls=1&peer=a.com&alterId=0&obfsParam=%7B%22Host%22%3A%22h.com%22%7D&allowInsecure=1"
	n, err = parseSubscriptionLine(vmess)
	if err != nil {
		t.Fatal(err)
	}
	want = map[string]string{"uuid": "11111111-2222-3333-4444-555555555555", "alter_id": "0", "security": "auto", "transport_type": "grpc", "transport_host": "h.com", "transport_service_name": "svc", "tls": "true", "server_name": "a.com", "insecure": "true"}
	if n.Protocol != "vmess" || n.Server != "1.2.3.4" || n.Port != 8443 || n.Name != "vm" || !reflect.DeepEqual(n.Settings, want) {
		t.Fatalf("vmess=%+v", n)
	}

	reality := "vless://" + base64.StdEncoding.EncodeToString([]byte("none:u@[2001:db8::1]:443")) + "?remarks=r&tls=1&pbk=KEY&sid=ab&xtls=2&peer=www.apple.com#name"
	n, err = parseSubscriptionLine(reality)
	if err != nil {
		t.Fatal(err)
	}
	if n.Server != "2001:db8::1" || n.Name != "name" || n.Settings["flow"] != "xtls-rprx-vision" || n.Settings["reality_public_key"] != "KEY" || n.Settings["reality_short_id"] != "ab" {
		t.Fatalf("reality=%+v", n)
	}
}
