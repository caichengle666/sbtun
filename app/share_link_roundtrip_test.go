package app

import (
	"encoding/base64"
	"testing"

	"github.com/caichengle666/sbtun/config"
)

func TestShareLinksKeepAllFieldsAndRoundTrip(t *testing.T) {
	links := []string{
		"vless://uuid1@cf.example:443?encryption=none&security=tls&sni=t.example&host=t.example&type=ws&path=%2Fv&fp=chrome&alpn=h2&allowInsecure=1&flow=xtls-rprx-vision#ws",
		"vless://uuid1@r.example:443?security=reality&pbk=PBK&sid=ab&sni=www.a.com&type=grpc&serviceName=svc&fp=chrome#grpc",
		"vless://uuid1@h.example:443?security=tls&type=httpupgrade&host=h.example&path=%2Fu#hu",
		"vless://uuid1@h.example:443?security=tls&type=http&host=h.example&path=%2Fh#h2",
		"trojan://pw@t.example:443?sni=s.example&type=ws&host=w.example&path=%2Ft&allowInsecure=1&alpn=h2#trojan",
		"trojan://pw@t.example:443?sni=s.example&type=grpc&serviceName=g#trojang",
		"hysteria2://pw@h.example:443?sni=s.example&obfs=salamander&obfs-password=op&insecure=1&mport=2000-3000&alpn=h3#hy2",
		"hysteria://h1.example:443?protocol=udp&auth=pw&peer=s.example&upmbps=30&downmbps=200&obfs=xplus&obfsParam=op&mport=2000-3000&insecure=1&alpn=h3#hy1",
		"tuic://uuid:pw@t.example:443?congestion_control=bbr&udp_relay_mode=quic&sni=s&alpn=h3&allow_insecure=1#tuic",
		"anytls://pw@a.example:443?sni=s&fp=chrome&insecure=1&alpn=h2#anytls",
		"wireguard://PRIV@w.example:51820?publickey=PUB&address=172.16.0.2/32&reserved=1,2,3&mtu=1280&presharedkey=PSK&allowedips=0.0.0.0/0#wg",
		"socks://dXNlcjpwYXNz@s.example:1080#socks",
		"http://user:pass@p.example:8080#http",
		"ss://YWVzLTEyOC1nY206cGFzcw@s.example:8388?plugin=obfs-local%3Bobfs%3Dhttp#ss",
	}
	vm := `{"v":"2","ps":"vm","add":"v.example","port":"443","id":"uuid","aid":"0","scy":"auto","net":"ws","type":"none","host":"vh.example","path":"/vm","tls":"tls","sni":"vs.example","alpn":"h2","fp":"chrome"}`
	links = append(links, "vmess://"+base64.StdEncoding.EncodeToString([]byte(vm)))
	var out []config.Node
	for _, l := range links {
		n, err := parseSubscriptionLine(l)
		if err != nil {
			t.Errorf("%s: %v", l, err)
			continue
		}
		out = append(out, n)
	}
	want := map[string]map[string]string{
		"ws":      {"transport_host": "t.example", "transport_path": "/v", "insecure": "true"},
		"grpc":    {"transport_service_name": "svc"},
		"trojan":  {"tls": "true", "insecure": "true", "transport_host": "w.example"},
		"trojang": {"tls": "true", "transport_service_name": "g"},
		"socks":   {"username": "user", "password": "pass"},
		"ss":      {"plugin": "obfs-local", "plugin_opts": "obfs=http"},
		"vm":      {"security": "", "transport_path": "/vm"},
	}
	for _, n := range out {
		for k, v := range want[n.Name] {
			if n.Settings[k] != v {
				t.Errorf("%s %s=%q want %q", n.Name, k, n.Settings[k], v)
			}
		}
		link, err := nodeShareLink(n)
		if err != nil {
			t.Errorf("%s export: %v", n.Name, err)
			continue
		}
		back, err := parseSubscriptionLine(link)
		if err != nil {
			t.Errorf("%s reparse: %v (%s)", n.Name, err, link)
			continue
		}
		for k := range mergeKeys(n.Settings, back.Settings) {
			if n.Settings[k] != back.Settings[k] {
				t.Errorf("%s %s: %q -> %q (%s)", n.Name, k, n.Settings[k], back.Settings[k], link)
			}
		}
	}
	if len(out) != len(links) {
		t.Fatalf("parsed %d of %d links", len(out), len(links))
	}
}

func TestParseVMessNumericFields(t *testing.T) {
	payload := `{"v":2,"ps":"n","add":"v.example","port":443,"id":"uuid","aid":0,"net":"grpc","path":"svc","type":"none"}`
	node, err := parseVMess("vmess://" + base64.StdEncoding.EncodeToString([]byte(payload)))
	if err != nil {
		t.Fatal(err)
	}
	if node.Port != 443 || node.Settings["alter_id"] != "0" || node.Settings["transport_service_name"] != "svc" || node.Settings["security"] != "" {
		t.Fatalf("node=%+v", node)
	}
}

func mergeKeys(a, b map[string]string) map[string]bool {
	m := map[string]bool{}
	for k := range a {
		m[k] = true
	}
	for k := range b {
		m[k] = true
	}
	return m
}
