package app

import (
	"testing"

	"github.com/caichengle666/sbtun/config"
)

func TestParseNewProtocolLinks(t *testing.T) {
	cases := []struct {
		link     string
		protocol string
		id       string
		want     map[string]string
	}{
		{
			link:     "tuic://00000000-0000-0000-0000-000000000001:secret@tuic.example.com:443?congestion_control=bbr&udp_relay_mode=quic&alpn=h3&sni=sni.example.com&allow_insecure=1#TUIC",
			protocol: "tuic", id: "tuic-tuic.example.com:443",
			want: map[string]string{"uuid": "00000000-0000-0000-0000-000000000001", "password": "secret", "congestion_control": "bbr", "udp_relay_mode": "quic", "alpn": "h3", "server_name": "sni.example.com", "insecure": "true"},
		},
		{
			link:     "anytls://secret@anytls.example.com:8443/?sni=sni.example.com&insecure=1&fp=chrome#AnyTLS",
			protocol: "anytls", id: "anytls-anytls.example.com:8443",
			want: map[string]string{"password": "secret", "server_name": "sni.example.com", "insecure": "true", "utls_fingerprint": "chrome"},
		},
		{
			link:     "wg://GBI17Q5In1YIWiw67IH5PS18N%2BFuH%2FxIren5DUH11WY%3D@wg.example.com:51820?publickey=fvrhwBf3ofI/oWcIZPIySYyB1SqHv/dOM8kTVGxYlS0=&address=172.16.0.2/32,fd01::2/128&reserved=1,2,3&mtu=1280#WG",
			protocol: "wireguard", id: "wireguard-wg.example.com:51820",
			want: map[string]string{"private_key": "GBI17Q5In1YIWiw67IH5PS18N+FuH/xIren5DUH11WY=", "public_key": "fvrhwBf3ofI/oWcIZPIySYyB1SqHv/dOM8kTVGxYlS0=", "local_address": "172.16.0.2/32,fd01::2/128", "reserved": "1,2,3", "mtu": "1280"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.protocol, func(t *testing.T) {
			nodes, err := parseSubscription(tc.link)
			if err != nil {
				t.Fatal(err)
			}
			node := nodes[0]
			if node.Protocol != tc.protocol || node.ID != tc.id || node.Port == 0 {
				t.Fatalf("unexpected node %+v", node)
			}
			assertSettings(t, node, tc.want)
		})
	}
}

func TestParseClashSubscription(t *testing.T) {
	body := `
mixed-port: 7890
proxies:
  - {name: SS, type: ss, server: ss.example.com, port: 8388, cipher: aes-256-gcm, password: secret, plugin: obfs, plugin-opts: {mode: tls, host: bing.com}}
  - name: VMess
    type: vmess
    server: vmess.example.com
    port: 443
    uuid: 00000000-0000-0000-0000-000000000001
    alterId: 0
    cipher: auto
    tls: true
    servername: sni.example.com
    network: ws
    ws-opts: {path: /ws, headers: {Host: cdn.example.com}}
  - name: VLESS
    type: vless
    server: vless.example.com
    port: 443
    uuid: 00000000-0000-0000-0000-000000000002
    flow: xtls-rprx-vision
    servername: www.microsoft.com
    client-fingerprint: chrome
    reality-opts: {public-key: pubkey, short-id: abcd}
  - {name: Trojan, type: trojan, server: trojan.example.com, port: 443, password: secret, sni: sni.example.com, skip-cert-verify: true, network: grpc, grpc-opts: {grpc-service-name: svc}}
  - {name: Hy2, type: hysteria2, server: hy2.example.com, port: 443, password: secret, sni: sni.example.com, up: "50 Mbps", down: "200 Mbps", obfs: salamander, obfs-password: o}
  - {name: TUIC, type: tuic, server: tuic.example.com, port: 443, uuid: 00000000-0000-0000-0000-000000000003, password: secret, alpn: [h3], congestion-controller: bbr, udp-relay-mode: native}
  - {name: AnyTLS, type: anytls, server: anytls.example.com, port: 443, password: secret, sni: sni.example.com, client-fingerprint: chrome}
  - {name: WG, type: wireguard, server: wg.example.com, port: 51820, ip: 172.16.0.2, ipv6: fd01::2, private-key: priv, public-key: pub, reserved: [1, 2, 3], mtu: 1280}
  - {name: Socks, type: socks5, server: socks.example.com, port: 1080, username: u, password: p}
  - {name: Unknown, type: snell, server: snell.example.com, port: 1}
  - {name: XHTTP, type: vless, server: x.example.com, port: 443, uuid: u, network: xhttp}
  - {name: ShadowTLS, type: ss, server: st.example.com, port: 443, cipher: aes-128-gcm, password: p, plugin: shadow-tls}
  - {name: PlainVMess, type: vmess, server: pv.example.com, port: 80, uuid: u, alterId: 0, cipher: auto, servername: ignored.example.com}
proxy-groups: []
`
	if !looksLikeClashYAML(body) {
		t.Fatal("clash yaml not detected")
	}
	nodes, err := parseClashSubscription(body)
	if err != nil {
		t.Fatal(err)
	}
	byName := nodesByName(nodes)
	if len(nodes) != 10 {
		t.Fatalf("got %d nodes: %+v", len(nodes), nodes)
	}
	assertSettings(t, byName["SS"], map[string]string{"method": "aes-256-gcm", "plugin": "obfs-local", "plugin_opts": "obfs=tls;obfs-host=bing.com"})
	assertSettings(t, byName["VMess"], map[string]string{"uuid": "00000000-0000-0000-0000-000000000001", "alter_id": "0", "security": "auto", "tls": "true", "server_name": "sni.example.com", "transport_type": "ws", "transport_path": "/ws", "transport_host": "cdn.example.com"})
	assertSettings(t, byName["VLESS"], map[string]string{"flow": "xtls-rprx-vision", "tls": "true", "server_name": "www.microsoft.com", "utls_fingerprint": "chrome", "reality_public_key": "pubkey", "reality_short_id": "abcd"})
	assertSettings(t, byName["Trojan"], map[string]string{"tls": "true", "insecure": "true", "transport_type": "grpc", "transport_service_name": "svc"})
	assertSettings(t, byName["Hy2"], map[string]string{"sni": "sni.example.com", "up_mbps": "50 Mbps", "obfs_type": "salamander", "obfs_password": "o"})
	assertSettings(t, byName["TUIC"], map[string]string{"alpn": "h3", "congestion_control": "bbr", "udp_relay_mode": "native"})
	assertSettings(t, byName["AnyTLS"], map[string]string{"password": "secret", "server_name": "sni.example.com", "utls_fingerprint": "chrome"})
	assertSettings(t, byName["WG"], map[string]string{"local_address": "172.16.0.2,fd01::2", "reserved": "1,2,3", "mtu": "1280"})
	if _, ok := byName["PlainVMess"].Settings["server_name"]; ok {
		t.Fatalf("servername without tls must be dropped: %v", byName["PlainVMess"].Settings)
	}
	if byName["Socks"].Protocol != "socks" || byName["SS"].ID != "ss-ss.example.com-8388" || byName["VMess"].ID != "vmess-vmess.example.com-443" || byName["TUIC"].ID != "tuic-tuic.example.com:443" {
		t.Fatalf("unexpected protocol or id: %+v", nodes)
	}
}

func TestParseSingBoxSubscription(t *testing.T) {
	body := `{
  "outbounds": [
    {"type": "vless", "tag": "VLESS", "server": "vless.example.com", "server_port": 443, "uuid": "u", "flow": "xtls-rprx-vision",
     "tls": {"enabled": true, "server_name": "sni.example.com", "utls": {"enabled": true, "fingerprint": "chrome"}, "reality": {"enabled": true, "public_key": "pk", "short_id": "sid"}},
     "transport": {"type": "ws", "path": "/ws", "headers": {"Host": "cdn.example.com"}}},
    {"type": "hysteria2", "tag": "Hy2", "server": "hy2.example.com", "server_port": 443, "password": "p", "up_mbps": 50, "server_ports": ["2000:3000"], "obfs": {"type": "salamander", "password": "o"}, "tls": {"enabled": true, "server_name": "sni.example.com"}},
    {"type": "tuic", "tag": "TUIC", "server": "tuic.example.com", "server_port": 443, "uuid": "u", "password": "p", "congestion_control": "bbr", "tls": {"enabled": true, "alpn": ["h3"]}},
    {"type": "anytls", "tag": "AnyTLS", "server": "anytls.example.com", "server_port": 443, "password": "p", "tls": {"enabled": true, "insecure": true}},
    {"type": "selector", "tag": "proxy", "outbounds": ["VLESS"]},
    {"type": "direct", "tag": "direct"}
  ],
  "endpoints": [
    {"type": "wireguard", "tag": "WG", "address": ["172.16.0.2/32"], "private_key": "priv", "mtu": 1280,
     "peers": [{"address": "wg.example.com", "port": 51820, "public_key": "pub", "reserved": [1, 2, 3], "allowed_ips": ["0.0.0.0/0"]}]}
  ]
}`
	nodes, err := parseSingBoxSubscription(body)
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 5 {
		t.Fatalf("got %d nodes: %+v", len(nodes), nodes)
	}
	byName := nodesByName(nodes)
	assertSettings(t, byName["VLESS"], map[string]string{"tls": "true", "server_name": "sni.example.com", "utls_fingerprint": "chrome", "reality_public_key": "pk", "reality_short_id": "sid", "transport_type": "ws", "transport_path": "/ws", "transport_host": "cdn.example.com"})
	assertSettings(t, byName["Hy2"], map[string]string{"sni": "sni.example.com", "up_mbps": "50", "server_ports": "2000:3000", "obfs_type": "salamander"})
	assertSettings(t, byName["TUIC"], map[string]string{"congestion_control": "bbr", "alpn": "h3"})
	assertSettings(t, byName["AnyTLS"], map[string]string{"insecure": "true"})
	assertSettings(t, byName["WG"], map[string]string{"private_key": "priv", "public_key": "pub", "local_address": "172.16.0.2/32", "reserved": "1,2,3", "allowed_ips": "0.0.0.0/0", "mtu": "1280"})
	if byName["WG"].Server != "wg.example.com" || byName["WG"].Port != 51820 {
		t.Fatalf("wireguard peer not used as server: %+v", byName["WG"])
	}
}

func nodesByName(nodes []config.Node) map[string]config.Node {
	out := map[string]config.Node{}
	for _, n := range nodes {
		out[n.Name] = n
	}
	return out
}

func assertSettings(t *testing.T, node config.Node, want map[string]string) {
	t.Helper()
	for key, value := range want {
		if node.Settings[key] != value {
			t.Fatalf("%s: setting %s=%q want %q (all: %v)", node.Name, key, node.Settings[key], value, node.Settings)
		}
	}
}
