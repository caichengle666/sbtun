package app

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/caichengle666/sbtun/config"
)

func vmessTestLink(fields string) string {
	return "vmess://" + base64.StdEncoding.EncodeToString([]byte(`{"v":"2",`+fields+`}`))
}

func nodesByLinks(t *testing.T, links ...string) map[string]config.Node {
	t.Helper()
	want := map[string]config.Node{}
	for _, link := range links {
		n, err := parseSubscriptionLine(link)
		if err != nil {
			t.Fatalf("%s: %v", link, err)
		}
		want[n.Name] = n
	}
	return want
}

const ssObfsLink = "ss://YWVzLTEyOC1nY206cGFzcw@s.example:8388/?plugin=obfs-local%3Bobfs%3Dhttp%3Bobfs-host%3Do.example#ss"

func TestSurgeConfigLinesMatchShareLinks(t *testing.T) {
	text := `[Proxy]
# 注释
vmess-ws = vmess, v.example, 443, username=uuid1, ws=true, ws-path=/v, ws-headers=Host:"h.example"|User-Agent:"x", tls=true, sni=s.example, skip-cert-verify=true
trojan = trojan, t.example, 443, password=pw, sni=s.example, ws=true, ws-path=/t, ws-headers=Host:"w.example"
hy2 = hysteria2, h.example, 443, password=pw, sni=s.example, download-bandwidth=200, port-hopping=2000-3000, port-hopping-interval=30
tuic = tuic-v5, t.example, 443, uuid=uuid, password=pw, sni=s, alpn=h3
anytls = anytls, a.example, 443, password=pw, sni=s, skip-cert-verify=true
socks = socks5, s.example, 1080, user, pass
ss = ss, s.example, 8388, encrypt-method=aes-128-gcm, password=pass, obfs=http, obfs-host=o.example
snell = snell, x.example, 443, psk=abc
s5t = socks5-tls, p.example, 443, user, pass
DIRECT = direct
`
	nodes, err := parseSubscriptionText(text)
	if err != nil {
		t.Fatal(err)
	}
	assertNodesMatch(t, "surge", nodes, nodesByLinks(t,
		vmessTestLink(`"ps":"vmess-ws","add":"v.example","port":"443","id":"uuid1","aid":"0","scy":"auto","net":"ws","host":"h.example","path":"/v","tls":"tls","sni":"s.example","allowInsecure":"1"`),
		"trojan://pw@t.example:443?sni=s.example&type=ws&host=w.example&path=%2Ft#trojan",
		"hysteria2://pw@h.example:443?sni=s.example&down_mbps=200&mport=2000-3000&hop_interval=30s#hy2",
		"tuic://uuid:pw@t.example:443?sni=s&alpn=h3#tuic",
		"anytls://pw@a.example:443?sni=s&insecure=1#anytls",
		"socks5://user:pass@s.example:1080#socks",
		ssObfsLink,
	))
}

func TestLoonConfigLinesMatchShareLinks(t *testing.T) {
	text := `ss = Shadowsocks,s.example,8388,aes-128-gcm,"pass",obfs-name=http,obfs-host=o.example
vmess-ws = VMess,v.example,443,auto,"uuid1",transport=ws,path=/v,host=h.example,over-tls=true,tls-name=s.example,skip-cert-verify=true
trojan = Trojan,t.example,443,"p,w",transport=ws,path=/t,host=w.example,tls-name=s.example
vless = VLESS,r.example,443,"uuid1",transport=tcp,over-tls=true,sni=www.a.com,flow=xtls-rprx-vision,public-key=PBK,short-id=ab
hy2 = Hysteria2,h.example,443,"pw",tls-name=s.example,download-bandwidth=200,port-hopping=2000-3000,port-hopping-interval=30
socks = socks5,s.example,1080,user,"pass"`
	nodes, err := parseSubscriptionText(text)
	if err != nil {
		t.Fatal(err)
	}
	assertNodesMatch(t, "loon", nodes, nodesByLinks(t,
		ssObfsLink,
		vmessTestLink(`"ps":"vmess-ws","add":"v.example","port":"443","id":"uuid1","aid":"0","scy":"auto","net":"ws","host":"h.example","path":"/v","tls":"tls","sni":"s.example","allowInsecure":"1"`),
		"trojan://p%2Cw@t.example:443?sni=s.example&type=ws&host=w.example&path=%2Ft#trojan",
		"vless://uuid1@r.example:443?security=reality&pbk=PBK&sid=ab&sni=www.a.com&flow=xtls-rprx-vision#vless",
		"hysteria2://pw@h.example:443?sni=s.example&down_mbps=200&mport=2000-3000&hop_interval=30s#hy2",
		"socks5://user:pass@s.example:1080#socks",
	))
}

func TestQuantumultXConfigLinesMatchShareLinks(t *testing.T) {
	text := `[server_local]
shadowsocks=s.example:8388, method=aes-128-gcm, password=pass, obfs=http, obfs-host=o.example, fast-open=false, udp-relay=false, tag=ss
vmess=v.example:443, method=chacha20-ietf-poly1305, password=uuid1, obfs=wss, obfs-host=h.example, obfs-uri=/v, tls-host=s.example, tls-verification=false, tag=vmess-ws
trojan=t.example:443, password=pw, over-tls=true, tls-host=s.example, obfs=wss, obfs-host=w.example, obfs-uri=/t, tag=trojan
vless=r.example:443, method=none, password=uuid1, obfs=over-tls, tls-host=www.a.com, vless-flow=xtls-rprx-vision, reality-base64-pubkey=PBK, reality-hex-shortid=ab, tag=vless
socks5=s.example:1080, username=user, password=pass, tag=socks
vmess=x.example:80, method=aes-128-gcm, password=u, obfs=http, tag=unsupported`
	nodes, err := parseSubscriptionText(text)
	if err != nil {
		t.Fatal(err)
	}
	assertNodesMatch(t, "quantumult-x", nodes, nodesByLinks(t,
		ssObfsLink,
		vmessTestLink(`"ps":"vmess-ws","add":"v.example","port":"443","id":"uuid1","aid":"0","scy":"chacha20-poly1305","net":"ws","host":"h.example","path":"/v","tls":"tls","sni":"s.example","allowInsecure":"1"`),
		"trojan://pw@t.example:443?sni=s.example&type=ws&host=w.example&path=%2Ft#trojan",
		"vless://uuid1@r.example:443?security=reality&pbk=PBK&sid=ab&sni=www.a.com&flow=xtls-rprx-vision#vless",
		"socks5://user:pass@s.example:1080#socks",
	))
}

func TestSIP008Subscription(t *testing.T) {
	nodes, err := parseSubscriptionText(`{"version":1,"servers":[
		{"id":"1","remarks":"ss","server":"s.example","server_port":8388,"password":"pass","method":"aes-128-gcm","plugin":"obfs-local","plugin_opts":"obfs=http;obfs-host=o.example"},
		{"id":"2","remarks":"skip","server":"x.example","server_port":443,"password":"p","method":"aes-128-gcm","plugin":"shadow-tls"}]}`)
	if err != nil {
		t.Fatal(err)
	}
	assertNodesMatch(t, "sip008", nodes, nodesByLinks(t, ssObfsLink))
}

func TestConfigLineImportsAsSingleNode(t *testing.T) {
	line := "trojan = trojan, t.example, 443, password=pw, sni=s.example"
	if err := ValidateNodeSource(line); err != nil {
		t.Fatal(err)
	}
	nodes, err := parseSubscription(line)
	if err != nil || len(nodes) != 1 || nodes[0].Protocol != "trojan" || nodes[0].Settings["server_name"] != "s.example" {
		t.Fatalf("nodes=%+v err=%v", nodes, err)
	}
	if err := ValidateNodeSource("hello = world"); err == nil {
		t.Fatal("random text accepted")
	}
}

func TestShadowrocketTrojanLink(t *testing.T) {
	assertNodesMatch(t, "shadowrocket-trojan", []config.Node{mustParseLine(t, "trojan://pw@t.example:443?peer=s.example&obfs=websocket&obfsParam=w.example&path=/t&allowInsecure=1#trojan")},
		nodesByLinks(t, "trojan://pw@t.example:443?sni=s.example&type=ws&host=w.example&path=%2Ft&allowInsecure=1#trojan"))
	assertNodesMatch(t, "shadowrocket-trojan-grpc", []config.Node{mustParseLine(t, "trojan://pw@t.example:443?peer=s.example&obfs=grpc&path=svc#g")},
		nodesByLinks(t, "trojan://pw@t.example:443?sni=s.example&type=grpc&serviceName=svc#g"))
}

func mustParseLine(t *testing.T, link string) config.Node {
	t.Helper()
	n, err := parseSubscriptionLine(link)
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func TestSubscriptionTriesUserAgents(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		ua := r.UserAgent()
		switch {
		case strings.Contains(ua, "clash"):
			http.Error(w, "forbidden", http.StatusForbidden)
		case strings.Contains(ua, "sing-box"):
			w.Write([]byte("   ")) // 空内容
		default:
			w.Write([]byte(base64.StdEncoding.EncodeToString([]byte("socks5://user:pass@s.example:1080#socks"))))
		}
	}))
	defer server.Close()
	nodes, err := parseSubscriptionBody(server.URL + "/sub")
	if err != nil || len(nodes) != 1 || calls.Load() != 3 {
		t.Fatalf("nodes=%v err=%v calls=%d", nodes, err, calls.Load())
	}

	// 网络不通时不再换 UA 重试
	server.Close()
	if _, err := parseSubscriptionBody(server.URL + "/sub"); err == nil || calls.Load() != 3 {
		t.Fatalf("err=%v calls=%d", err, calls.Load())
	}
}

func TestSubscriptionUpdateKeepsCurrentNodeByName(t *testing.T) {
	body := atomic.Value{}
	body.Store("socks5://u:p@a.example:1080#A\nsocks5://u:p@b.example:1080#B")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(body.Load().(string)))
	}))
	defer server.Close()
	a := newSubscriptionTestApp(t)
	if _, err := a.ImportSubscription(server.URL + "/sub"); err != nil {
		t.Fatal(err)
	}
	if err := a.SelectNode(a.GetConfig().Nodes[1].ID); err != nil {
		t.Fatal(err)
	}
	body.Store("socks5://u:p@a.example:1080#A\nsocks5://u:changed@b.example:1080#B")
	if _, err := a.UpdateSubscription(a.GetConfig().Subscriptions[0].ID); err != nil {
		t.Fatal(err)
	}
	cfg := a.GetConfig()
	for _, n := range cfg.Nodes {
		if n.ID == cfg.CurrentNodeID && n.Name != "B" {
			t.Fatalf("current node switched to %s", n.Name)
		}
	}
}
