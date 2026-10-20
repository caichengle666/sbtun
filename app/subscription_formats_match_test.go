package app

import (
	"testing"

	"github.com/caichengle666/sbtun/config"
)

// 同一批节点分别写成分享链接、Clash YAML、sing-box JSON，解析结果应该完全一致。
var formatSampleLinks = []string{
	"vless://uuid1@cf.example:443?security=tls&sni=t.example&host=t.example&type=ws&path=%2Fv&fp=chrome&alpn=h2&allowInsecure=1&flow=xtls-rprx-vision&packetEncoding=xudp#ws",
	"vless://uuid1@r.example:443?security=reality&pbk=PBK&sid=ab&sni=www.a.com&type=grpc&serviceName=svc&fp=chrome#grpc",
	"vless://uuid1@h.example:443?security=tls&type=httpupgrade&host=h.example&path=%2Fu#hu",
	"trojan://pw@t.example:443?sni=s.example&type=ws&host=w.example&path=%2Ft&allowInsecure=1&alpn=h2&fp=chrome#trojan",
	"trojan://pw@t.example:443?sni=s.example&type=grpc&serviceName=g#trojang",
	"hysteria2://pw@h.example:443?sni=s.example&obfs=salamander&obfs-password=op&insecure=1&alpn=h3#hy2",
	"hysteria://h1.example:443?protocol=udp&auth=pw&peer=s.example&upmbps=30&downmbps=200&obfs=xplus&obfsParam=op&mport=2000-3000&insecure=1&alpn=h3#hy1",
	"tuic://uuid:pw@t.example:443?congestion_control=bbr&udp_relay_mode=quic&sni=s&alpn=h3&allow_insecure=1#tuic",
	"anytls://pw@a.example:443?sni=s&fp=chrome&insecure=1#anytls",
	"wireguard://PRIV@w.example:51820?publickey=PUB&address=172.16.0.2/32&reserved=1,2,3&mtu=1280&presharedkey=PSK#wg",
	"socks5://user:pass@s.example:1080#socks",
	"http://user:pass@p.example:8080#http",
	"ss://YWVzLTEyOC1nY206cGFzcw@s.example:8388/?plugin=obfs-local%3Bobfs%3Dhttp%3Bobfs-host%3Do.example#ss",
}

const formatSampleClash = `proxies:
  - {name: ws, type: vless, server: cf.example, port: 443, uuid: uuid1, tls: true, servername: t.example, client-fingerprint: chrome, alpn: [h2], skip-cert-verify: true, flow: xtls-rprx-vision, packet-encoding: xudp, network: ws, ws-opts: {path: /v, headers: {Host: t.example}}}
  - {name: grpc, type: vless, server: r.example, port: 443, uuid: uuid1, tls: true, servername: www.a.com, client-fingerprint: chrome, reality-opts: {public-key: PBK, short-id: ab}, network: grpc, grpc-opts: {grpc-service-name: svc}}
  - {name: hu, type: vless, server: h.example, port: 443, uuid: uuid1, tls: true, network: ws, ws-opts: {path: /u, headers: {Host: h.example}, v2ray-http-upgrade: true}}
  - {name: trojan, type: trojan, server: t.example, port: 443, password: pw, sni: s.example, skip-cert-verify: true, alpn: [h2], client-fingerprint: chrome, network: ws, ws-opts: {path: /t, headers: {Host: w.example}}}
  - {name: trojang, type: trojan, server: t.example, port: 443, password: pw, sni: s.example, network: grpc, grpc-opts: {grpc-service-name: g}}
  - {name: hy2, type: hysteria2, server: h.example, port: 443, password: pw, sni: s.example, obfs: salamander, obfs-password: op, skip-cert-verify: true, alpn: [h3]}
  - {name: hy1, type: hysteria, server: h1.example, port: 443, ports: 2000-3000, auth-str: pw, sni: s.example, up: "30 Mbps", down: "200 Mbps", obfs: op, protocol: udp, skip-cert-verify: true, alpn: [h3]}
  - {name: tuic, type: tuic, server: t.example, port: 443, uuid: uuid, password: pw, congestion-controller: bbr, udp-relay-mode: quic, sni: s, alpn: [h3], skip-cert-verify: true}
  - {name: anytls, type: anytls, server: a.example, port: 443, password: pw, sni: s, client-fingerprint: chrome, skip-cert-verify: true}
  - {name: wg, type: wireguard, private-key: PRIV, ip: 172.16.0.2/32, mtu: 1280, peers: [{server: w.example, port: 51820, public-key: PUB, pre-shared-key: PSK, reserved: [1, 2, 3]}]}
  - {name: socks, type: socks5, server: s.example, port: 1080, username: user, password: pass}
  - {name: http, type: http, server: p.example, port: 8080, username: user, password: pass}
  - {name: ss, type: ss, server: s.example, port: 8388, cipher: aes-128-gcm, password: pass, plugin: obfs, plugin-opts: {mode: http, host: o.example}}
`

const formatSampleSingBox = `{"outbounds": [
 {"type": "vless", "tag": "ws", "server": "cf.example", "server_port": 443, "uuid": "uuid1", "flow": "xtls-rprx-vision", "packet_encoding": "xudp",
  "tls": {"enabled": true, "server_name": "t.example", "insecure": true, "alpn": ["h2"], "utls": {"enabled": true, "fingerprint": "chrome"}},
  "transport": {"type": "ws", "path": "/v", "headers": {"Host": ["t.example"]}}},
 {"type": "vless", "tag": "grpc", "server": "r.example", "server_port": 443, "uuid": "uuid1",
  "tls": {"enabled": true, "server_name": "www.a.com", "utls": {"enabled": true, "fingerprint": "chrome"}, "reality": {"enabled": true, "public_key": "PBK", "short_id": "ab"}},
  "transport": {"type": "grpc", "service_name": "svc"}},
 {"type": "vless", "tag": "hu", "server": "h.example", "server_port": 443, "uuid": "uuid1", "tls": {"enabled": true},
  "transport": {"type": "httpupgrade", "host": "h.example", "path": "/u"}},
 {"type": "trojan", "tag": "trojan", "server": "t.example", "server_port": 443, "password": "pw",
  "tls": {"enabled": true, "server_name": "s.example", "insecure": true, "alpn": ["h2"], "utls": {"enabled": true, "fingerprint": "chrome"}},
  "transport": {"type": "ws", "path": "/t", "headers": {"Host": "w.example"}}},
 {"type": "trojan", "tag": "trojang", "server": "t.example", "server_port": 443, "password": "pw", "tls": {"enabled": true, "server_name": "s.example"},
  "transport": {"type": "grpc", "service_name": "g"}},
 {"type": "hysteria2", "tag": "hy2", "server": "h.example", "server_port": 443, "password": "pw", "obfs": {"type": "salamander", "password": "op"},
  "tls": {"enabled": true, "server_name": "s.example", "insecure": true, "alpn": ["h3"]}},
 {"type": "hysteria", "tag": "hy1", "server": "h1.example", "server_port": 443, "server_ports": ["2000:3000"], "auth_str": "pw", "up_mbps": 30, "down": "200 Mbps", "obfs": "op",
  "tls": {"enabled": true, "server_name": "s.example", "insecure": true, "alpn": ["h3"]}},
 {"type": "tuic", "tag": "tuic", "server": "t.example", "server_port": 443, "uuid": "uuid", "password": "pw", "congestion_control": "bbr", "udp_relay_mode": "quic",
  "tls": {"enabled": true, "server_name": "s", "insecure": true, "alpn": ["h3"]}},
 {"type": "anytls", "tag": "anytls", "server": "a.example", "server_port": 443, "password": "pw",
  "tls": {"enabled": true, "server_name": "s", "insecure": true, "utls": {"enabled": true, "fingerprint": "chrome"}}},
 {"type": "socks", "tag": "socks", "server": "s.example", "server_port": 1080, "version": "5", "username": "user", "password": "pass"},
 {"type": "http", "tag": "http", "server": "p.example", "server_port": 8080, "username": "user", "password": "pass"},
 {"type": "shadowsocks", "tag": "ss", "server": "s.example", "server_port": 8388, "method": "aes-128-gcm", "password": "pass", "plugin": "obfs-local", "plugin_opts": "obfs=http;obfs-host=o.example"}
],
"endpoints": [
 {"type": "wireguard", "tag": "wg", "address": ["172.16.0.2/32"], "private_key": "PRIV", "mtu": 1280,
  "peers": [{"address": "w.example", "port": 51820, "public_key": "PUB", "pre_shared_key": "PSK", "reserved": [1, 2, 3]}]}
]}`

// TLS 是 TUIC、AnyTLS 的必选项，有没有 tls=true 生成的配置一样。
func comparableSettings(n config.Node) map[string]string {
	out := map[string]string{}
	for k, v := range n.Settings {
		if k == "tls" && (n.Protocol == "tuic" || n.Protocol == "anytls") {
			continue
		}
		out[k] = v
	}
	return out
}

func TestSubscriptionFormatsMatchShareLinks(t *testing.T) {
	want := map[string]config.Node{}
	for _, link := range formatSampleLinks {
		n, err := parseSubscriptionLine(link)
		if err != nil {
			t.Fatalf("%s: %v", link, err)
		}
		want[n.Name] = n
	}
	clash, err := parseClashSubscription(formatSampleClash)
	if err != nil {
		t.Fatal(err)
	}
	singbox, err := parseSingBoxSubscription(formatSampleSingBox)
	if err != nil {
		t.Fatal(err)
	}
	for format, nodes := range map[string][]config.Node{"clash": clash, "sing-box": singbox} {
		assertNodesMatch(t, format, nodes, want)
	}
}

// assertNodesMatch 要求 nodes 与 want（按名称）逐字段一致，数量也要相同。
func assertNodesMatch(t *testing.T, format string, nodes []config.Node, want map[string]config.Node) {
	t.Helper()
	if len(nodes) != len(want) {
		t.Errorf("%s: parsed %d nodes, want %d", format, len(nodes), len(want))
	}
	for _, n := range nodes {
		w, ok := want[n.Name]
		if !ok {
			t.Errorf("%s: unexpected node %s", format, n.Name)
			continue
		}
		if n.Protocol != w.Protocol || n.Server != w.Server || n.Port != w.Port || n.ID != w.ID {
			t.Errorf("%s %s: %s %s:%d id=%s, want %s %s:%d id=%s", format, n.Name, n.Protocol, n.Server, n.Port, n.ID, w.Protocol, w.Server, w.Port, w.ID)
		}
		got, exp := comparableSettings(n), comparableSettings(w)
		for k := range mergeKeys(got, exp) {
			if got[k] != exp[k] {
				t.Errorf("%s %s %s: %q, link gives %q", format, n.Name, k, got[k], exp[k])
			}
		}
	}
}

func TestSubscriptionFormatsKeepMultiplexAndNetwork(t *testing.T) {
	singbox, err := parseSingBoxSubscription(`{"outbounds":[
		{"type":"vmess","tag":"a","server":"v.example","server_port":443,"uuid":"u","network":"tcp","multiplex":{"enabled":true}},
		{"type":"shadowsocks","tag":"b","server":"s.example","server_port":8388,"method":"aes-128-gcm","password":"p","udp_over_tcp":true}]}`)
	if err != nil || len(singbox) != 2 {
		t.Fatalf("nodes=%+v err=%v", singbox, err)
	}
	if s := singbox[0].Settings; s["network"] != "tcp" || s["multiplex"] != "true" {
		t.Fatalf("vmess settings=%v", s)
	}
	if s := singbox[1].Settings; s["udp_over_tcp"] != "true" {
		t.Fatalf("ss settings=%v", s)
	}
	clash, err := parseClashSubscription("proxies:\n  - {name: a, type: vmess, server: v.example, port: 443, uuid: u, smux: {enabled: true}}\n")
	if err != nil || len(clash) != 1 || clash[0].Settings["multiplex"] != "true" {
		t.Fatalf("clash=%+v err=%v", clash, err)
	}
}
