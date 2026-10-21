package app

// 订阅/分享链接解析审计中各问题的回归测试。设置 SING_BOX_BINARY 时会额外用 sing-box check 校验生成的完整配置。

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/caichengle666/sbtun/config"
	"github.com/caichengle666/sbtun/core/singbox"
)

// buildRuntime 以 current 为当前节点、others 为备用节点生成完整运行配置，返回当前节点的出站。
func buildRuntime(t *testing.T, current config.Node, others ...config.Node) map[string]any {
	t.Helper()
	cfg := config.Default()
	cfg.RoutingMode = config.RoutingGlobal
	current.ID = "cur"
	cfg.Nodes = []config.Node{current}
	for i, n := range others {
		n.ID = "other" + string(rune('a'+i))
		cfg.Nodes = append(cfg.Nodes, n)
	}
	cfg.CurrentNodeID = "cur"
	data, err := singbox.BuildConfigWithAuth(cfg, t.TempDir(), singbox.LocalAuth{}, nil)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if binary := os.Getenv("SING_BOX_BINARY"); binary != "" {
		path := filepath.Join(t.TempDir(), "c.json")
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatal(err)
		}
		if out, err := exec.Command(binary, "check", "-c", path).CombinedOutput(); err != nil {
			t.Fatalf("sing-box check 失败: %s\n%s", out, data)
		}
	}
	var doc struct{ Outbounds []map[string]any }
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	for _, o := range doc.Outbounds {
		if o["tag"] == "node-cur" {
			return o
		}
	}
	t.Fatalf("当前节点出站缺失: %s", data)
	return nil
}

func parseOne(t *testing.T, link string) config.Node {
	t.Helper()
	nodes, err := parseSubscription(link)
	if err != nil || len(nodes) != 1 {
		t.Fatalf("parse %s: %v %d", link, err, len(nodes))
	}
	return nodes[0]
}

// 1. gRPC 传输不能写 host/path；备用节点也不能拖垮整份配置。
func TestGRPCTransportOnlyWritesServiceName(t *testing.T) {
	good := parseOne(t, "trojan://pw@good.example.com:443?sni=good.example.com#good")
	bad := parseOne(t, "vless://11111111-1111-1111-1111-111111111111@bad.example.com:443?type=grpc&security=tls&sni=bad.example.com&serviceName=svc&host=bad.example.com#bad")
	buildRuntime(t, good, bad)
	for link, want := range map[string]string{
		"vless://11111111-1111-1111-1111-111111111111@x.example.com:443?type=grpc&security=tls&serviceName=svc&host=x.example.com#a": "svc",
		"vless://11111111-1111-1111-1111-111111111111@x.example.com:443?type=grpc&security=tls&path=%2Fother#b":                      "other",
	} {
		transport, _ := buildRuntime(t, parseOne(t, link))["transport"].(map[string]any)
		if len(transport) != 2 || transport["service_name"] != want {
			t.Errorf("%s: transport=%v", link, transport)
		}
	}
}

// 2. SOCKS 不支持 TLS：链接导入时拒绝，存量节点生成时也不写 tls。
func TestSocksNeverGetsTLS(t *testing.T) {
	if _, err := parseSubscription("socks5://example.com:1080?tls=1#s"); err == nil {
		t.Error("socks+tls 应当拒绝导入")
	}
	if n := parseOne(t, "socks5://example.com:1080?sni=example.com#s"); n.Settings["server_name"] != "" {
		t.Errorf("未开 TLS 的 socks 不应保留 sni: %v", n.Settings)
	}
	stored := config.Node{Protocol: "socks", Server: "example.com", Port: 1080, Settings: map[string]string{"version": "5", "tls": "true", "server_name": "example.com"}}
	if out := buildRuntime(t, stored); out["tls"] != nil {
		t.Errorf("socks 出站不应有 tls: %v", out)
	}
}

// 3. security=none 的节点不能被开启 TLS。
func TestSecurityNoneKeepsTLSOff(t *testing.T) {
	vm := base64.StdEncoding.EncodeToString([]byte(`{"v":"2","ps":"vm","add":"example.com","port":"80","id":"11111111-1111-1111-1111-111111111111","net":"ws","path":"/ws","tls":"","sni":"example.com","fp":"chrome"}`))
	nodes := []config.Node{
		parseOne(t, "vless://11111111-1111-1111-1111-111111111111@example.com:80?type=ws&security=none&sni=example.com&fp=chrome#v"),
		parseOne(t, "trojan://pw@example.com:80?type=ws&security=none&sni=example.com#t"),
		parseOne(t, "vmess://"+vm),
	}
	qx, ok := parseConfigLine("trojan=example.com:80, password=pw, obfs=ws, obfs-host=example.com, obfs-uri=/ws, tag=t")
	if !ok {
		t.Fatal("QX 行解析失败")
	}
	nodes = append(nodes, qx)
	// 存量节点：tls 关闭但残留 sni/fp，生成端也要过滤。
	nodes = append(nodes, config.Node{Protocol: "vless", Server: "example.com", Port: 80, Settings: map[string]string{"uuid": "11111111-1111-1111-1111-111111111111", "server_name": "example.com", "utls_fingerprint": "chrome"}})
	for _, n := range nodes {
		if out := buildRuntime(t, n); out["tls"] != nil {
			t.Errorf("%s 未开 TLS 却生成了 tls: %v", n.Name, out["tls"])
		}
	}
}

// 4. SIP002 的 2022 方法用百分号编码 userinfo；导出也要按这个写法。
func TestShadowsocks2022PercentEncoded(t *testing.T) {
	n := parseOne(t, "ss://2022-blake3-aes-128-gcm:YctPZ6U7xPPcU%2Bgp3u%2B0tw%3D%3D@example.com:8443#ss")
	if n.Settings["password"] != "YctPZ6U7xPPcU+gp3u+0tw==" {
		t.Fatalf("password=%q", n.Settings["password"])
	}
	buildRuntime(t, n)
	link, err := nodeShareLink(n)
	if err != nil || !strings.HasPrefix(link, "ss://2022-blake3-aes-128-gcm:") {
		t.Fatalf("link=%s err=%v", link, err)
	}
	if back := parseOne(t, link); nodeFingerprint(back) != nodeFingerprint(n) {
		t.Errorf("往返不一致: %v / %v", n.Settings, back.Settings)
	}
}

// 5. 按 76 列换行的 base64 订阅。
func TestWrappedBase64Subscription(t *testing.T) {
	body := "c3M6Ly9ZV1Z6TFRJMU5pMW5ZMjA2Y0dGemN3QDEuMi4zLjQ6ODM4OCNhCnNzOi8vWVdWekxUSTFO\naTFuWTIwNmNHRnpjd0AxLjIuMy41OjgzODgjYgpzczovL1lXVnpMVEkxTmkxbll6QTZjR0Z6Y3dA\nMS4yLjMuNjo4Mzg4I2M="
	if nodes, _ := parseSubscriptionText(body); len(nodes) != 3 {
		t.Errorf("解析出 %d 个节点，期望 3", len(nodes))
	}
}

// 6. https:// 代理链接开启 TLS；导出的链接再导入不能被当成订阅地址。
func TestHTTPSProxyLink(t *testing.T) {
	n := parseOne(t, "https://u:p@p.example.com:443#p")
	if n.Settings["tls"] != "true" {
		t.Fatalf("settings=%v", n.Settings)
	}
	n.Settings["server_name"] = "p.example.com"
	link, _ := nodeShareLink(n)
	if isSubscriptionURL(link) || strings.Contains(link, "tls=1") {
		t.Errorf("导出链接 %s", link)
	}
	if back := parseOne(t, link); nodeFingerprint(back) != nodeFingerprint(n) {
		t.Errorf("往返不一致: %v / %v", n.Settings, back.Settings)
	}
	if !isSubscriptionURL("https://sub.example.com:8443/?token=abc") || !isSubscriptionURL("https://sub.example.com/api/v1/sub") {
		t.Error("真正的订阅地址被误判为代理节点")
	}
}

// 7. h2 传输映射到 http；sing-box 不支持的传输报错而不是当成 TCP。
func TestLinkTransports(t *testing.T) {
	n := parseOne(t, "vless://11111111-1111-1111-1111-111111111111@example.com:443?type=h2&security=tls&sni=example.com&path=%2Fh2&host=a.example.com#h2")
	if n.Settings["transport_type"] != "http" {
		t.Fatalf("settings=%v", n.Settings)
	}
	if transport, _ := buildRuntime(t, n)["transport"].(map[string]any); transport["type"] != "http" || transport["path"] != "/h2" {
		t.Errorf("transport=%v", transport)
	}
	for _, typ := range []string{"xhttp", "kcp", "quic"} {
		if _, err := parseSubscription("vless://11111111-1111-1111-1111-111111111111@example.com:443?security=tls&type=" + typ + "#x"); err == nil {
			t.Errorf("type=%s 应当报不支持", typ)
		}
	}
	// type=tcp 就是不加传输层，不能写成 network=tcp（那会禁掉 UDP）。
	if n := parseOne(t, "vless://11111111-1111-1111-1111-111111111111@example.com:443?type=tcp&security=tls#t"); n.Settings["network"] != "" {
		t.Errorf("settings=%v", n.Settings)
	}
	vm := base64.StdEncoding.EncodeToString([]byte(`{"v":"2","ps":"vm","add":"example.com","port":"443","id":"u","net":"h2","path":"/h","tls":"tls"}`))
	vn := parseOne(t, "vmess://"+vm)
	link, _ := nodeShareLink(vn)
	if back := parseOne(t, link); nodeFingerprint(back) != nodeFingerprint(vn) {
		t.Errorf("vmess h2 往返不一致: %v / %v", vn.Settings, back.Settings)
	}
}

// 8. Hysteria2 URI：userpass 认证、多端口、省略端口。
func TestHysteria2URIScheme(t *testing.T) {
	if n := parseOne(t, "hysteria2://user:pass@example.com:443#u"); n.Settings["password"] != "user:pass" {
		t.Errorf("password=%q", n.Settings["password"])
	}
	n := parseOne(t, "hysteria2://pw@example.com:443,20000-30000/?sni=example.com&hop_interval=30#m")
	if n.Port != 443 || n.Settings["server_ports"] != "443,20000-30000" {
		t.Errorf("node=%+v", n)
	}
	out := buildRuntime(t, n)
	if ports, _ := out["server_ports"].([]any); len(ports) != 2 || ports[0] != "443:443" || out["hop_interval"] != "30s" {
		t.Errorf("out=%v", out)
	}
	if n := parseOne(t, "hysteria2://pw@example.com/?sni=example.com#d"); n.Port != 443 {
		t.Errorf("省略端口应默认 443: %+v", n)
	}
	if n := parseOne(t, "hysteria2://pw@[2001:db8::1]:8443#v6"); n.Server != "2001:db8::1" || n.Port != 8443 {
		t.Errorf("IPv6: %+v", n)
	}
}

// 9、13. trojan / vmess 导出再导入指纹不变。
func TestExportRoundTripKeepsFingerprint(t *testing.T) {
	vm := "vmess://" + base64.StdEncoding.EncodeToString([]byte(`{"v":"2","ps":"x","add":"example.com","port":"443","id":"11111111-1111-1111-1111-111111111111","aid":"0","net":"ws","path":"/ws"}`))
	for _, l := range []string{
		"trojan://pw@example.com:80?type=ws&security=none&path=%2Fws#t",
		"trojan://pw@example.com:443?sni=example.com&fp=chrome#f",
		vm,
	} {
		n := parseOne(t, l)
		link, _ := nodeShareLink(n)
		if back := parseOne(t, link); nodeFingerprint(back) != nodeFingerprint(n) {
			t.Errorf("往返不一致:\n %v\n %v", n.Settings, back.Settings)
		}
	}
}

// 10. Clash 的 hysteria2 带宽单位要换算。
func TestClashHysteria2Bandwidth(t *testing.T) {
	nodes, _ := parseSubscriptionText("proxies:\n  - {name: a, type: hysteria2, server: example.com, port: 443, password: pw, up: \"1 Gbps\", down: 100}\n")
	if len(nodes) != 1 {
		t.Fatalf("nodes=%v", nodes)
	}
	if out := buildRuntime(t, nodes[0]); out["up_mbps"] != float64(1000) || out["down_mbps"] != float64(100) {
		t.Errorf("out=%v", out)
	}
}

// 11. 规则下载在锁外进行，并有大小上限。
func TestImportCustomRulesDownloadsOutsideLock(t *testing.T) {
	a := newSubscriptionTestApp(t)
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/big" {
			w.Write([]byte(strings.Repeat("#", maxSubscriptionBytes+1)))
			return
		}
		<-release
		w.Write([]byte("proxy,domain_suffix,example.com\n"))
	}))
	defer server.Close()
	done := make(chan error, 1)
	go func() {
		_, err := a.ImportCustomRules(server.URL + "/rules")
		done <- err
	}()
	time.Sleep(100 * time.Millisecond)
	locked := make(chan struct{})
	go func() {
		a.operationMu.Lock()
		a.operationMu.Unlock()
		close(locked)
	}()
	select {
	case <-locked:
	case <-time.After(2 * time.Second):
		t.Fatal("下载规则期间 operationMu 被占用")
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if _, err := a.ImportCustomRules(server.URL + "/big"); err == nil || !strings.Contains(err.Error(), "上限") {
		t.Fatalf("超大规则应报错: %v", err)
	}
}

// 12. vmess 没有名称时退回到 协议-地址。
func TestVMessDefaultName(t *testing.T) {
	vm := "vmess://" + base64.StdEncoding.EncodeToString([]byte(`{"ps":"","add":"example.com","port":"443","id":"u"}`))
	if n := parseOne(t, vm); n.Name != "vmess-example.com" {
		t.Errorf("name=%q", n.Name)
	}
}

// 13. 指纹归一化后订阅节点 ID 会变化一次，当前选中节点仍要保住。
func TestVMessNormalizationKeepsCurrentNode(t *testing.T) {
	before := config.Config{CurrentNodeID: "old", Nodes: []config.Node{
		{ID: "old", Name: "", Subscription: "s", Protocol: "vmess", Server: "a.example.com", Port: 443, Settings: map[string]string{"uuid": "u", "security": "auto"}},
		{ID: "b", Name: "", Subscription: "s", Protocol: "vmess", Server: "b.example.com", Port: 443, Settings: map[string]string{"uuid": "u", "security": "auto"}},
	}}
	fresh := []config.Node{
		{ID: "vmess-b", Name: "vmess-b.example.com", Protocol: "vmess", Server: "b.example.com", Port: 443, Settings: map[string]string{"uuid": "u"}},
		{ID: "vmess-a", Name: "vmess-a.example.com", Protocol: "vmess", Server: "a.example.com", Port: 443, Settings: map[string]string{"uuid": "u"}},
	}
	next := before
	next.Nodes, _ = mergeSubscriptionNodes(before.Nodes, "s", fresh)
	keepCurrentNodeByName(before, &next, "s")
	if next.CurrentNodeID != "vmess-a" {
		t.Fatalf("current=%q nodes=%v", next.CurrentNodeID, nodeIDs(next.Nodes))
	}
}

// 14. 配置读不出来时，订阅更新要报真实错误。
func TestSubscriptionUpdateReportsConfigError(t *testing.T) {
	a := newSubscriptionTestApp(t)
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte("{broken"), 0o600); err != nil {
		t.Fatal(err)
	}
	a.manager = config.NewManager(path)
	if _, err := a.UpdateSubscription("sub-x"); err == nil || strings.Contains(err.Error(), "订阅不存在") {
		t.Errorf("err=%v", err)
	}
	if results := a.UpdateAllSubscriptions(); len(results) != 1 || results[0].Error == "" {
		t.Errorf("results=%+v", results)
	}
}
