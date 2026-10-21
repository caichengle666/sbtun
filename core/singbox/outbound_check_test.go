package singbox

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/caichengle666/sbtun/config"
)

const testUUID = "11111111-1111-1111-1111-111111111111"

// 典型节点（含审计里出过问题的写法）放进同一份运行配置，必须整份通过 sing-box check。
func TestTypicalNodesBuildValidRuntimeConfig(t *testing.T) {
	nodes := []config.Node{
		{ID: "grpc-host", Protocol: "vless", Server: "g.example.com", Port: 443, Settings: map[string]string{"uuid": testUUID, "tls": "true", "server_name": "g.example.com", "transport_type": "grpc", "transport_service_name": "svc", "transport_host": "g.example.com", "transport_path": "/svc"}},
		{ID: "socks-tls", Protocol: "socks", Server: "s.example.com", Port: 1080, Settings: map[string]string{"version": "5", "tls": "true", "server_name": "s.example.com", "path": "/x"}},
		{ID: "ss2022", Protocol: "shadowsocks", Server: "ss.example.com", Port: 8443, Settings: map[string]string{"method": "2022-blake3-aes-128-gcm", "password": "YctPZ6U7xPPcU+gp3u+0tw"}},
		{ID: "vless-none", Protocol: "vless", Server: "v.example.com", Port: 80, Settings: map[string]string{"uuid": testUUID, "server_name": "v.example.com", "utls_fingerprint": "chrome", "transport_type": "ws", "transport_path": "/ws", "transport_host": "v.example.com"}},
		{ID: "h2", Protocol: "vmess", Server: "h.example.com", Port: 443, Settings: map[string]string{"uuid": testUUID, "security": "AUTO", "tls": "true", "transport_type": "http", "transport_host": "a.example.com,b.example.com", "transport_path": "/h2"}},
		{ID: "reality", Protocol: "vless", Server: "r.example.com", Port: 443, Settings: map[string]string{"uuid": testUUID, "tls": "true", "flow": "xtls-rprx-vision", "server_name": "www.microsoft.com", "utls_fingerprint": "Chrome", "reality_public_key": "jNXHt1yRo0vDuchQlIP6Z0ZvjT3KtzVI+T4E7RoLJS0=", "reality_short_id": "abcd"}},
		{ID: "hy2", Protocol: "hysteria2", Server: "hy.example.com", Port: 443, Settings: map[string]string{"password": "pw", "server_ports": "443,20000-30000", "hop_interval": "30"}},
	}
	cfg := testConfig(config.RoutingGlobal)
	cfg.Nodes = nodes
	cfg.CurrentNodeID = "grpc-host"
	data, err := BuildConfig(cfg, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	var runtime RuntimeConfig
	if err := json.Unmarshal(data, &runtime); err != nil {
		t.Fatal(err)
	}
	byTag := map[string]map[string]any{}
	for _, o := range runtime.Outbounds {
		byTag[o["tag"].(string)] = o
	}
	if len(byTag["node-grpc-host"]["transport"].(map[string]any)) != 2 {
		t.Errorf("grpc transport=%v", byTag["node-grpc-host"]["transport"])
	}
	if o := byTag["node-socks-tls"]; o["tls"] != nil || o["path"] != nil {
		t.Errorf("socks=%v", o)
	}
	if o := byTag["node-vless-none"]; o["tls"] != nil {
		t.Errorf("vless security=none=%v", o)
	}
	if o := byTag["node-ss2022"]; o["password"] != "YctPZ6U7xPPcU+gp3u+0tw==" {
		t.Errorf("ss2022=%v", o)
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
}

// 单节点校验：sing-box 必然拒绝的节点被跳过，不拖垮整份配置；当前节点不合法时明确报错。
func TestInvalidNodeIsSkipped(t *testing.T) {
	bad := []config.Node{
		{ID: "ss-key", Protocol: "shadowsocks", Server: "a.example.com", Port: 1, Settings: map[string]string{"method": "2022-blake3-aes-256-gcm", "password": "YctPZ6U7xPPcU%2Bgp3u%2B0tw%3D%3D"}},
		{ID: "ss-method", Protocol: "shadowsocks", Server: "a.example.com", Port: 1, Settings: map[string]string{"method": "chacha20", "password": "x"}},
		{ID: "reality", Protocol: "vless", Server: "a.example.com", Port: 1, Settings: map[string]string{"uuid": testUUID, "tls": "true", "reality_public_key": "PBK"}},
		{ID: "sid", Protocol: "vless", Server: "a.example.com", Port: 1, Settings: map[string]string{"uuid": testUUID, "tls": "true", "reality_public_key": "jNXHt1yRo0vDuchQlIP6Z0ZvjT3KtzVI-T4E7RoLJS0", "reality_short_id": "0123456789abcdef01"}},
		{ID: "flow", Protocol: "vless", Server: "a.example.com", Port: 1, Settings: map[string]string{"uuid": testUUID, "flow": "xtls-rprx-direct"}},
		{ID: "vmess-sec", Protocol: "vmess", Server: "a.example.com", Port: 1, Settings: map[string]string{"uuid": testUUID, "security": "aes-128-cfb"}},
		{ID: "tuic", Protocol: "tuic", Server: "a.example.com", Port: 1, Settings: map[string]string{"uuid": "not-a-uuid", "password": "x"}},
		{ID: "obfs", Protocol: "hysteria2", Server: "a.example.com", Port: 1, Settings: map[string]string{"password": "x", "obfs_type": "foo"}},
	}
	for _, n := range bad {
		if _, err := buildOutbound(n); err == nil {
			t.Errorf("%s 应当校验失败", n.ID)
		}
	}
	cfg := testConfig(config.RoutingGlobal)
	cfg.Nodes = append(cfg.Nodes, bad...)
	data, err := BuildConfig(cfg, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	var runtime RuntimeConfig
	if err := json.Unmarshal(data, &runtime); err != nil {
		t.Fatal(err)
	}
	for _, o := range runtime.Outbounds {
		if o["tag"] == "proxy" {
			if tags := o["outbounds"].([]any); len(tags) != 1 {
				t.Fatalf("selector=%v", tags)
			}
		}
	}
	cfg.CurrentNodeID = "ss-key"
	if _, err := BuildConfig(cfg, t.TempDir()); err == nil {
		t.Fatal("当前节点不合法时应报错")
	}
}
