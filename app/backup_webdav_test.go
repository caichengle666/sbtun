package app

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/caichengle666/sbtun/config"
	"github.com/caichengle666/sbtun/core/rules"
	"golang.org/x/net/webdav"
)

func newWebDAVTestServer(t *testing.T) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	puts := &atomic.Int32{}
	dav := &webdav.Handler{FileSystem: webdav.NewMemFS(), LockSystem: webdav.NewMemLS()}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if user, pass, ok := r.BasicAuth(); !ok || user != "u" || pass != "p" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if r.Method == http.MethodPut {
			puts.Add(1)
		}
		dav.ServeHTTP(w, r)
	}))
	t.Cleanup(server.Close)
	return server, puts
}

func newBackupTestApp(t *testing.T) *App {
	a := newSubscriptionTestApp(t)
	a.workDir = t.TempDir()
	return a
}

func TestWebDAVBackupSkipsSubscriptionNodesAndRestores(t *testing.T) {
	subBody := atomic.Value{}
	subBody.Store("socks://127.0.0.1:2001#s1\nsocks://127.0.0.1:2002#s2")
	subServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(subBody.Load().(string)))
	}))
	defer subServer.Close()
	dav, _ := newWebDAVTestServer(t)

	for _, password := range []string{"", "secret"} {
		t.Run("encrypt="+password, func(t *testing.T) {
			a := newBackupTestApp(t)
			if _, err := a.ImportSubscription("socks://127.0.0.1:1080#manual"); err != nil {
				t.Fatal(err)
			}
			if _, err := a.ImportSubscription(subServer.URL + "/sub"); err != nil {
				t.Fatal(err)
			}
			settings := config.WebDAVSettings{URL: dav.URL + "/backup-" + password, Username: "u", Password: "p", EncryptPassword: password}
			if err := a.TestWebDAV(settings); err != nil {
				t.Fatal(err)
			}
			if err := a.SaveWebDAVSettings(settings); err != nil {
				t.Fatal(err)
			}
			cfg := a.GetConfig()
			if err := a.SelectNode(cfg.Nodes[2].ID); err != nil { // 选中订阅节点 s2
				t.Fatal(err)
			}
			name, err := a.BackupToWebDAV()
			if err != nil {
				t.Fatal(err)
			}
			if (password != "") != strings.HasSuffix(name, ".sbtun") {
				t.Fatalf("name=%s", name)
			}

			// 备份内容里没有订阅节点
			client, _ := newWebDAVClient(nil, settings)
			data, err := client.get(name)
			if err != nil {
				t.Fatal(err)
			}
			if password != "" && strings.Contains(string(data), "127.0.0.1") {
				t.Fatal("encrypted backup contains plaintext")
			}
			payload, err := decodeBackup(data, password)
			if err != nil {
				t.Fatal(err)
			}
			if len(payload.Config.Nodes) != 1 || payload.Config.Nodes[0].Name != "manual" || len(payload.Config.Subscriptions) != 1 || payload.Config.WebDAV != nil {
				t.Fatalf("payload=%+v", payload.Config)
			}
			if password != "" {
				if _, err := decodeBackup(data, "wrong"); err == nil {
					t.Fatal("wrong password accepted")
				}
			}

			// 在另一台“新电脑”上恢复：订阅节点按地址重新拉取，之前选中的订阅节点仍被选中
			b := newBackupTestApp(t)
			if err := b.SaveWebDAVSettings(settings); err != nil {
				t.Fatal(err)
			}
			list, err := b.ListWebDAVBackups()
			if err != nil || len(list) != 1 || list[0].Name != name {
				t.Fatalf("list=%+v err=%v", list, err)
			}
			if warning, err := b.RestoreFromWebDAV(name); err != nil || warning != "" {
				t.Fatalf("warning=%q err=%v", warning, err)
			}
			got := b.GetConfig()
			if len(got.Nodes) != 3 || got.CurrentNodeID != cfg.Nodes[2].ID || got.WebDAV == nil || got.WebDAV.URL != settings.URL {
				t.Fatalf("restored=%+v", got)
			}
			for _, n := range got.Nodes {
				if (n.Name == "manual") != (n.Subscription == "") {
					t.Fatalf("node %s subscription=%q", n.Name, n.Subscription)
				}
			}
			if err := b.DeleteWebDAVBackup("../config.json"); err == nil {
				t.Fatal("non-backup name accepted")
			}
			if err := b.DeleteWebDAVBackup(name); err != nil {
				t.Fatal(err)
			}
			if list, err := b.ListWebDAVBackups(); err != nil || len(list) != 0 {
				t.Fatalf("after delete list=%+v err=%v", list, err)
			}
		})
	}
}

func TestBackupFingerprintIgnoresSubscriptionNodes(t *testing.T) {
	subBody := atomic.Value{}
	subBody.Store("socks://127.0.0.1:2001#s1\nsocks://127.0.0.1:2002#s2")
	subServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(subBody.Load().(string)))
	}))
	defer subServer.Close()
	a := newBackupTestApp(t)
	if _, err := a.ImportSubscription(subServer.URL + "/sub"); err != nil {
		t.Fatal(err)
	}
	_, base, err := a.buildBackup()
	if err != nil {
		t.Fatal(err)
	}
	subBody.Store("socks://127.0.0.1:2001#s1\nsocks://127.0.0.1:2003#s3")
	if _, err := a.UpdateSubscription(a.GetConfig().Subscriptions[0].ID); err != nil {
		t.Fatal(err)
	}
	if err := a.SelectNode(a.GetConfig().Nodes[1].ID); err != nil {
		t.Fatal(err)
	}
	if _, hash, _ := a.buildBackup(); hash != base {
		t.Fatal("subscription node change or node switch changed backup fingerprint")
	}
	if _, err := a.ImportSubscription("socks://127.0.0.1:1080#manual"); err != nil {
		t.Fatal(err)
	}
	if _, hash, _ := a.buildBackup(); hash == base {
		t.Fatal("manual node change did not change backup fingerprint")
	}
}

func TestWebDAVBackupKeepsLatestTen(t *testing.T) {
	dav, puts := newWebDAVTestServer(t)
	settings := config.WebDAVSettings{URL: dav.URL + "/keep/", Username: "u", Password: "p"}
	client, _ := newWebDAVClient(nil, settings)
	if err := client.ensureDir(); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 12; i++ {
		if err := client.put(backupPrefix+"20260101-0000"+string(rune('a'+i))+".json", []byte("{}")); err != nil {
			t.Fatal(err)
		}
	}
	a := newBackupTestApp(t)
	payload, _, _ := a.buildBackup()
	name, err := a.uploadBackup(settings, payload)
	if err != nil {
		t.Fatal(err)
	}
	list, _ := client.list()
	if len(list) != backupKeep || list[0].Name != name || puts.Load() != 13 {
		t.Fatalf("list=%d first=%s puts=%d", len(list), list[0].Name, puts.Load())
	}
}

func TestWebDAVWrongPassword(t *testing.T) {
	dav, _ := newWebDAVTestServer(t)
	err := New().TestWebDAV(config.WebDAVSettings{URL: dav.URL, Username: "u", Password: "x"})
	if err == nil || !strings.Contains(err.Error(), "账号或密码") {
		t.Fatalf("err=%v", err)
	}
	if _, err := newWebDAVClient(nil, config.WebDAVSettings{URL: "ftp://x"}); err == nil {
		t.Fatal("ftp url accepted")
	}
}

func TestAutoBackupOnlyUploadsSettledChanges(t *testing.T) {
	dav, puts := newWebDAVTestServer(t)
	a := newBackupTestApp(t)
	if _, err := a.ImportSubscription("socks://127.0.0.1:1080#manual"); err != nil {
		t.Fatal(err)
	}
	settings := config.WebDAVSettings{URL: dav.URL + "/auto/", Username: "u", Password: "p"}
	if err := a.SaveWebDAVSettings(settings); err != nil {
		t.Fatal(err)
	}
	if p := a.autoBackupTick(""); p != "" || puts.Load() != 0 {
		t.Fatal("auto backup ran while disabled")
	}
	settings.AutoBackup = true
	if err := a.SaveWebDAVSettings(settings); err != nil {
		t.Fatal(err)
	}
	p := a.autoBackupTick("") // 第一次只记下变化
	if p == "" || puts.Load() != 0 {
		t.Fatalf("pending=%q puts=%d", p, puts.Load())
	}
	if p = a.autoBackupTick(p); p != "" || puts.Load() != 1 { // 一分钟后没再变，上传
		t.Fatalf("pending=%q puts=%d", p, puts.Load())
	}
	for i := 0; i < 3; i++ { // 没有变化不再上传
		p = a.autoBackupTick(p)
	}
	if puts.Load() != 1 {
		t.Fatalf("puts=%d", puts.Load())
	}
}

// v0.3.19 ~ v0.3.3x 的备份里路由模式可能是已取消的 custom，必须先迁移再校验，否则旧备份恢复不了。
func TestDecodeBackupMigratesCustomRoutingMode(t *testing.T) {
	cfg := config.Default()
	cfg.RoutingMode = "custom"
	data, err := encodeBackup(backupPayload{App: "sbtun", Version: 1, Config: cfg}, "")
	if err != nil {
		t.Fatal(err)
	}
	payload, err := decodeBackup(data, "")
	if err != nil {
		t.Fatalf("旧备份无法恢复: %v", err)
	}
	if err := config.Validate(payload.Config); err != nil {
		t.Fatal(err)
	}
}

// 配置恢复失败时，已经换上的规则集要换回去，不留一半新一半旧。
func TestRestoreRollsBackRuleSetsWhenConfigFails(t *testing.T) {
	dav, _ := newWebDAVTestServer(t)
	dir := t.TempDir()
	blocker := filepath.Join(dir, "blocked")
	if err := os.WriteFile(blocker, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	a := New()
	a.manager = config.NewManager(filepath.Join(dir, "config.json"))
	a.runtime = NewRuntimeCoordinator(filepath.Join(blocker, "runtime-data"), "sing-box") // 运行配置写不进去
	a.rulesManager = rules.NewManager(filepath.Join(dir, "rules"))
	if err := a.rulesManager.SetEnabled("geosite-cn", false); err != nil {
		t.Fatal(err)
	}
	before, _ := a.rulesManager.Sets()
	settings := config.WebDAVSettings{URL: dav.URL + "/rollback", Username: "u", Password: "p"}
	if err := a.SaveWebDAVSettings(settings); err != nil {
		t.Fatal(err)
	}
	cfg := config.Default()
	cfg.Nodes = []config.Node{{ID: "n", Name: "n", Protocol: "socks", Server: "127.0.0.1", Port: 1080}}
	cfg.CurrentNodeID = "n"
	if err := os.WriteFile(filepath.Join(dir, "rules", "ads.srs"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	name, err := (*App)(nil).uploadBackup(settings, backupPayload{App: "sbtun", Version: 1, Config: cfg,
		RuleSets: []rules.RuleSet{{ID: "ads", Name: "ads", URL: "https://example.com/ads.srs", Path: "ads.srs", Enabled: true}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.RestoreFromWebDAV(name); err == nil {
		t.Fatal("运行配置写不进去时恢复应失败")
	}
	after, _ := a.rulesManager.Sets()
	if len(after) != len(before) || after[0].Enabled != before[0].Enabled {
		t.Fatalf("规则集没有回滚: %+v", after)
	}
}
