package app

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/caichengle666/sbtun/config"
)

func TestParseVLessKeepsTransportAndTLSFields(t *testing.T) {
	node, err := parseVLess("vless://uuid@example.com:443?type=ws&host=cdn.example.com&path=%2Fws&sni=tls.example.com&security=tls&flow=xtls-rprx-vision&alpn=h2%2Chttp%2F1.1&insecure=1&pbk=public&sid=short&fp=chrome#test")
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"uuid": "uuid", "transport_type": "ws", "transport_host": "cdn.example.com", "transport_path": "/ws",
		"server_name": "tls.example.com", "tls": "true", "flow": "xtls-rprx-vision", "alpn": "h2,http/1.1",
		"insecure": "true", "reality_public_key": "public", "reality_short_id": "short", "utls_fingerprint": "chrome",
	}
	for key, value := range want {
		if node.Settings[key] != value {
			t.Fatalf("setting %s=%q want %q; settings=%v", key, node.Settings[key], value, node.Settings)
		}
	}
}

func TestParseVMessKeepsUUIDAndTransportFields(t *testing.T) {
	payload := `{"v":"2","ps":"test","add":"example.com","port":"443","id":"uuid","aid":"0","net":"ws","type":"none","host":"cdn.example.com","path":"/ws","tls":"tls","sni":"tls.example.com","alpn":"h2","fp":"chrome"}`
	link := "vmess://" + base64.StdEncoding.EncodeToString([]byte(payload))
	node, err := parseVMess(link)
	if err != nil {
		t.Fatal(err)
	}
	for key, value := range map[string]string{"uuid": "uuid", "security": "none", "transport_type": "ws", "transport_host": "cdn.example.com", "transport_path": "/ws", "server_name": "tls.example.com", "tls": "true", "alpn": "h2", "utls_fingerprint": "chrome"} {
		if node.Settings[key] != value {
			t.Fatalf("setting %s=%q want %q; settings=%v", key, node.Settings[key], value, node.Settings)
		}
	}
}

func TestParseTrojanKeepsTransportFields(t *testing.T) {
	node, err := parseTrojan("trojan://secret@example.com:443?type=grpc&serviceName=proxy&sni=tls.example.com&security=tls&alpn=h2&insecure=1#test")
	if err != nil {
		t.Fatal(err)
	}
	if node.Settings["transport_type"] != "grpc" || node.Settings["server_name"] != "tls.example.com" || node.Settings["tls"] != "true" {
		t.Fatalf("unexpected settings: %v", node.Settings)
	}
}

func TestParseHysteria2KeepsAdvancedFields(t *testing.T) {
	node, err := parseHysteria2("hysteria2://secret@example.com:443?sni=tls.example.com&alpn=h3&server_ports=2000-3000&hop_interval=30s&obfs=salamander&obfs-password=obfs#test")
	if err != nil {
		t.Fatal(err)
	}
	for key, value := range map[string]string{"sni": "tls.example.com", "alpn": "h3", "server_ports": "2000-3000", "hop_interval": "30s", "obfs_type": "salamander", "obfs_password": "obfs"} {
		if node.Settings[key] != value {
			t.Fatalf("setting %s=%q want %q; settings=%v", key, node.Settings[key], value, node.Settings)
		}
	}
}

func TestParseHTTPStyleKeepsTLSFields(t *testing.T) {
	node, err := parseHTTPStyle("https://user:pass@example.com:443?tls=1&sni=tls.example.com&insecure=1&path=%2Fproxy#test")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.EqualFold(node.Settings["tls"], "true") || node.Settings["server_name"] != "tls.example.com" || node.Settings["path"] != "/proxy" {
		t.Fatalf("unexpected settings: %v", node.Settings)
	}
}

func TestParseSubscriptionURLSplitsNodes(t *testing.T) {
	body := "socks://127.0.0.1:1080#one\nsocks://127.0.0.1:1081#two"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(base64.StdEncoding.EncodeToString([]byte(body))))
	}))
	defer server.Close()

	nodes, err := parseSubscription(server.URL + "/sub")
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 2 || nodes[0].Port != 1080 || nodes[1].Port != 1081 {
		t.Fatalf("解析节点异常: %+v", nodes)
	}
}

func TestImportSubscriptionSelectsFirstNode(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("socks://127.0.0.1:1080#one\nsocks://127.0.0.1:1081#two"))
	}))
	defer server.Close()

	a := New()
	a.manager = config.NewManager(filepath.Join(t.TempDir(), "config.json"))
	a.runtime = NewRuntimeCoordinator(filepath.Join(t.TempDir(), "runtime-data"), "sing-box")
	added, err := a.ImportSubscription(server.URL + "/sub")
	if err != nil {
		t.Fatal(err)
	}
	if added != 2 {
		t.Fatalf("added=%d want=2", added)
	}
	cfg, err := a.manager.Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.CurrentNodeID != cfg.Nodes[0].ID {
		t.Fatalf("current node=%q first=%q", cfg.CurrentNodeID, cfg.Nodes[0].ID)
	}
}

func TestParseSOCKS5Links(t *testing.T) {
	for _, link := range []string{"socks5://user:pass@example.com:1080#s5", "socks5h://example.com:1081#s5h"} {
		node, err := parseSubscriptionLine(link)
		if err != nil {
			t.Fatalf("parse %s: %v", link, err)
		}
		if node.Protocol != "socks" || node.Port == 0 {
			t.Fatalf("unexpected SOCKS5 node: %+v", node)
		}
	}
}
