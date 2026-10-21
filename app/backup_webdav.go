package app

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/caichengle666/sbtun/config"
	"github.com/caichengle666/sbtun/core/rules"
)

const (
	backupPrefix       = "sbtun-backup-"
	backupKeep         = 10
	backupMagic        = "SBTUNENC"
	backupKDFRounds    = 600000
	autoBackupInterval = time.Minute
)

// backupPayload 是备份文件内容：配置和规则集列表。配置里只有手动添加的节点，
// 订阅节点不备份（恢复时按订阅地址重新拉取），也不含 WebDAV 设置。
type backupPayload struct {
	App       string          `json:"app"`
	Version   int             `json:"version"`
	CreatedAt string          `json:"created_at"`
	Config    config.Config   `json:"config"`
	RuleSets  []rules.RuleSet `json:"rule_sets"`
	// CurrentNode 是备份时选中的节点 ID（可能是订阅节点），恢复后如果还在就继续选中它。
	CurrentNode string `json:"current_node,omitempty"`
}

// WebDAVBackupDTO 描述云端的一份备份。
type WebDAVBackupDTO struct {
	Name      string `json:"name"`
	Size      int64  `json:"size"`
	Modified  string `json:"modified"`
	Encrypted bool   `json:"encrypted"`
}

// ---------- App 方法（GUI 与 CLI 共用） ----------

// SaveWebDAVSettings 保存 WebDAV 设置；地址为空表示关闭云端备份。
func (a *App) SaveWebDAVSettings(s config.WebDAVSettings) error {
	s.URL = strings.TrimSpace(s.URL)
	s.Username = strings.TrimSpace(s.Username)
	if s.URL != "" {
		if _, err := newWebDAVClient(s); err != nil {
			return err
		}
	}
	a.operationMu.Lock()
	defer a.operationMu.Unlock()
	if a.manager == nil {
		return errors.New("配置管理器未初始化")
	}
	return a.manager.Update(func(_ config.Config, next *config.Config) error {
		if s.URL == "" {
			next.WebDAV = nil
		} else {
			next.WebDAV = &s
		}
		return nil
	})
}

// TestWebDAV 检查地址和账号能否访问，目录不存在时会尝试创建。
func (a *App) TestWebDAV(s config.WebDAVSettings) error {
	client, err := newWebDAVClient(s)
	if err != nil {
		return err
	}
	if err := client.ensureDir(); err != nil {
		return err
	}
	_, err = client.list()
	return err
}

// BackupToWebDAV 立即上传一份备份，返回文件名。
func (a *App) BackupToWebDAV() (string, error) {
	settings, err := a.webDAVSettings()
	if err != nil {
		return "", err
	}
	payload, hash, err := a.buildBackup()
	if err != nil {
		return "", err
	}
	name, err := uploadBackup(settings, payload)
	if err != nil {
		return "", err
	}
	a.saveBackupHash(hash)
	return name, nil
}

// ListWebDAVBackups 按时间从新到旧列出云端备份。
func (a *App) ListWebDAVBackups() ([]WebDAVBackupDTO, error) {
	settings, err := a.webDAVSettings()
	if err != nil {
		return nil, err
	}
	client, err := newWebDAVClient(settings)
	if err != nil {
		return nil, err
	}
	return client.list()
}

// RestoreFromWebDAV 用指定备份整体覆盖本机的节点、订阅、规则和设置（WebDAV 设置保留本机的）。
// 订阅节点按地址重新拉取。返回的提示说明哪些订阅或规则文件没能下载。
func (a *App) RestoreFromWebDAV(name string) (string, error) {
	settings, err := a.webDAVSettings()
	if err != nil {
		return "", err
	}
	client, err := newWebDAVClient(settings)
	if err != nil {
		return "", err
	}
	data, err := client.get(name)
	if err != nil {
		return "", err
	}
	payload, err := decodeBackup(data, settings.EncryptPassword)
	if err != nil {
		return "", err
	}

	// 先在不持锁的情况下拉取订阅；拉不到的订阅沿用本机已有的节点。
	local, err := a.LoadConfig()
	if err != nil {
		return "", err
	}
	var warnings []string
	fetched := make(map[string][]config.Node, len(payload.Config.Subscriptions))
	usages := make(map[string]*config.SubscriptionUsage, len(payload.Config.Subscriptions))
	for _, sub := range payload.Config.Subscriptions {
		nodes, usage, err := a.parseSubscription(sub.URL)
		usages[sub.ID] = usage
		if err == nil && len(nodes) == 0 {
			err = errors.New("没有识别到节点")
		}
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("订阅“%s”拉取失败（%v），可稍后手动更新", sub.Name, err))
			for _, n := range local.Nodes {
				if n.Subscription == sub.ID {
					nodes = append(nodes, n)
				}
			}
		}
		fetched[sub.ID] = nodes
	}

	a.operationMu.Lock()
	defer a.operationMu.Unlock()
	if err := a.mutateConfigAndSyncLocked(func(before config.Config, next *config.Config) error {
		restored := payload.Config
		restored.WebDAV = before.WebDAV
		restored.Subscriptions = append([]config.Subscription(nil), payload.Config.Subscriptions...)
		restored.Nodes = append([]config.Node(nil), payload.Config.Nodes...)
		for i, sub := range restored.Subscriptions {
			restored.Nodes, _ = mergeSubscriptionNodes(restored.Nodes, sub.ID, fetched[sub.ID])
			restored.Subscriptions[i].UpdatedAt = time.Now().UTC().Format(time.RFC3339)
			restored.Subscriptions[i].Usage = usages[sub.ID]
		}
		if payload.CurrentNode != "" {
			restored.CurrentNodeID = payload.CurrentNode
		}
		ensureCurrentNode(&restored)
		*next = restored
		return nil
	}, false); err != nil {
		return "", err
	}
	if a.rulesManager != nil && len(payload.RuleSets) > 0 {
		missing, err := a.rulesManager.Replace(payload.RuleSets)
		if err != nil {
			return "", err
		}
		var failed []string
		for _, id := range missing {
			if err := a.rulesManager.UpdateWithValidation(id, a.validateRuleSet); err != nil {
				failed = append(failed, id)
			}
		}
		if len(failed) > 0 {
			warnings = append(warnings, "规则集下载失败，可稍后在规则页更新: "+strings.Join(failed, ", "))
		}
	}
	warning := strings.Join(warnings, "；")
	if err := a.reloadIfRunningLocked(); err != nil {
		return warning, err
	}
	// 刚恢复的内容和云端一致，不需要再自动上传。
	if _, hash, err := a.buildBackupLocked(); err == nil {
		a.saveBackupHash(hash)
	}
	return warning, nil
}

func (a *App) webDAVSettings() (config.WebDAVSettings, error) {
	cfg, err := a.LoadConfig()
	if err != nil {
		return config.WebDAVSettings{}, err
	}
	if cfg.WebDAV == nil || cfg.WebDAV.URL == "" {
		return config.WebDAVSettings{}, errors.New("还没有设置 WebDAV")
	}
	return *cfg.WebDAV, nil
}

// buildBackup 生成备份内容和内容指纹。订阅节点不进备份，指纹也忽略当前选中的节点和
// 订阅更新时间和流量：切换节点、订阅节点变了都不算变化，只有手动节点、订阅地址、规则、设置变了才算。
func (a *App) buildBackup() (backupPayload, string, error) {
	// 规则集列表由 operationMu 保护。
	a.operationMu.Lock()
	defer a.operationMu.Unlock()
	return a.buildBackupLocked()
}

func (a *App) buildBackupLocked() (backupPayload, string, error) {
	cfg, err := a.LoadConfig()
	if err != nil {
		return backupPayload{}, "", err
	}
	payload := backupPayload{App: "sbtun", Version: 1, CurrentNode: cfg.CurrentNodeID}
	cfg.WebDAV = nil
	manual := make([]config.Node, 0, len(cfg.Nodes))
	for _, n := range cfg.Nodes {
		if n.Subscription == "" {
			manual = append(manual, n)
		}
	}
	cfg.Nodes = manual
	cfg.CurrentNodeID = ""
	cfg.Subscriptions = append([]config.Subscription(nil), cfg.Subscriptions...)
	for i := range cfg.Subscriptions {
		cfg.Subscriptions[i].UpdatedAt = ""
		cfg.Subscriptions[i].Usage = nil
	}
	payload.Config = cfg
	if a.rulesManager != nil {
		payload.RuleSets = a.rulesManager.Sets()
		for i := range payload.RuleSets {
			payload.RuleSets[i].Path = filepath.Base(payload.RuleSets[i].Path)
		}
	}
	fingerprint := payload
	fingerprint.CurrentNode = ""
	data, err := json.Marshal(fingerprint)
	if err != nil {
		return backupPayload{}, "", err
	}
	sum := sha256.Sum256(data)
	return payload, hex.EncodeToString(sum[:]), nil
}

func (a *App) backupStatePath() string {
	return filepath.Join(a.workDir, "webdav-backup.json")
}

func (a *App) lastBackupHash() string {
	var state struct {
		Hash string `json:"hash"`
	}
	data, err := os.ReadFile(a.backupStatePath())
	if err == nil {
		_ = json.Unmarshal(data, &state)
	}
	return state.Hash
}

func (a *App) saveBackupHash(hash string) {
	data, _ := json.Marshal(map[string]string{"hash": hash})
	_ = os.MkdirAll(a.workDir, 0o755)
	_ = os.WriteFile(a.backupStatePath(), data, 0o600)
}

// autoBackupLoop 每分钟检查一次；内容和上次上传的不同、并且一分钟内没有再变时才上传。
func (a *App) autoBackupLoop() {
	if a.ctx == nil {
		return
	}
	ticker := time.NewTicker(autoBackupInterval)
	defer ticker.Stop()
	pending := ""
	for {
		select {
		case <-a.ctx.Done():
			return
		case <-ticker.C:
			pending = a.autoBackupTick(pending)
		}
	}
}

// autoBackupTick 执行一次检查，返回等待确认的内容指纹。
func (a *App) autoBackupTick(pending string) string {
	settings, err := a.webDAVSettings()
	if err != nil || !settings.AutoBackup {
		return ""
	}
	payload, hash, err := a.buildBackup()
	if err != nil || hash == a.lastBackupHash() {
		return ""
	}
	if hash != pending {
		return hash // 等下一次确认改动已经结束，避免连续修改时传很多份
	}
	if _, err := uploadBackup(settings, payload); err != nil {
		return hash // 上传失败下次再试
	}
	a.saveBackupHash(hash)
	return ""
}

// ---------- 备份文件编码 ----------

func uploadBackup(settings config.WebDAVSettings, payload backupPayload) (string, error) {
	client, err := newWebDAVClient(settings)
	if err != nil {
		return "", err
	}
	now := time.Now()
	payload.CreatedAt = now.Format(time.RFC3339)
	data, err := encodeBackup(payload, settings.EncryptPassword)
	if err != nil {
		return "", err
	}
	name := backupFileName(now, settings.EncryptPassword != "")
	if err := client.ensureDir(); err != nil {
		return "", err
	}
	if err := client.put(name, data); err != nil {
		return "", err
	}
	if list, err := client.list(); err == nil && len(list) > backupKeep {
		for _, old := range list[backupKeep:] {
			_ = client.delete(old.Name)
		}
	}
	return name, nil
}

var hostCleaner = regexp.MustCompile(`[^A-Za-z0-9-]+`)

// backupFileName 以时间开头，按文件名排序就是按时间排序；带上主机名方便区分多台电脑。
func backupFileName(t time.Time, encrypted bool) string {
	name := backupPrefix + t.Format("20060102-150405")
	if host, err := os.Hostname(); err == nil {
		if host = strings.Trim(hostCleaner.ReplaceAllString(host, "-"), "-"); host != "" {
			name += "-" + host
		}
	}
	if encrypted {
		return name + ".sbtun"
	}
	return name + ".json"
}

func isBackupFile(name string) bool {
	return strings.HasPrefix(name, backupPrefix) && (strings.HasSuffix(name, ".json") || strings.HasSuffix(name, ".sbtun"))
}

func encodeBackup(payload backupPayload, password string) ([]byte, error) {
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil || password == "" {
		return data, err
	}
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return nil, err
	}
	gcm, err := backupCipher(password, salt)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	out := append([]byte(backupMagic), salt...)
	out = append(out, nonce...)
	return gcm.Seal(out, nonce, data, []byte(backupMagic)), nil
}

func decodeBackup(data []byte, password string) (backupPayload, error) {
	if bytes.HasPrefix(data, []byte(backupMagic)) {
		if password == "" {
			return backupPayload{}, errors.New("这份备份已加密，请先在 WebDAV 设置里填写加密密码")
		}
		rest := data[len(backupMagic):]
		if len(rest) < 16+12 {
			return backupPayload{}, errors.New("备份文件已损坏")
		}
		gcm, err := backupCipher(password, rest[:16])
		if err != nil {
			return backupPayload{}, err
		}
		nonce, sealed := rest[16:16+gcm.NonceSize()], rest[16+gcm.NonceSize():]
		plain, err := gcm.Open(nil, nonce, sealed, []byte(backupMagic))
		if err != nil {
			return backupPayload{}, errors.New("加密密码不对，或备份文件已损坏")
		}
		data = plain
	}
	var payload backupPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return backupPayload{}, fmt.Errorf("备份文件格式错误: %w", err)
	}
	if payload.App != "sbtun" || payload.Version != 1 {
		return backupPayload{}, errors.New("不是 sbtun 备份文件，或版本不支持")
	}
	if err := config.Validate(payload.Config); err != nil {
		return backupPayload{}, fmt.Errorf("备份中的配置无效: %w", err)
	}
	return payload, nil
}

func backupCipher(password string, salt []byte) (cipher.AEAD, error) {
	key, err := pbkdf2.Key(sha256.New, password, salt, backupKDFRounds, 32)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

// ---------- 最小 WebDAV 客户端 ----------

type webdavClient struct {
	base     *url.URL
	username string
	password string
	http     *http.Client
}

func newWebDAVClient(s config.WebDAVSettings) (*webdavClient, error) {
	u, err := url.Parse(strings.TrimSpace(s.URL))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, errors.New("WebDAV 地址无效，应以 http:// 或 https:// 开头")
	}
	if !strings.HasSuffix(u.Path, "/") {
		u.Path += "/"
	}
	u.RawPath = ""
	return &webdavClient{base: u, username: s.Username, password: s.Password, http: &http.Client{Timeout: 30 * time.Second}}, nil
}

func (c *webdavClient) do(method, name string, body []byte, headers map[string]string) (*http.Response, error) {
	target := *c.base
	if name != "" {
		target.Path = path.Join(c.base.Path, name)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	req, err := http.NewRequestWithContext(ctx, method, target.String(), bytes.NewReader(body))
	if err != nil {
		cancel()
		return nil, err
	}
	if c.username != "" || c.password != "" {
		req.SetBasicAuth(c.username, c.password)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("连接 WebDAV 失败: %w", err)
	}
	resp.Body = cancelOnClose{resp.Body, cancel}
	return resp, nil
}

type cancelOnClose struct {
	io.ReadCloser
	cancel context.CancelFunc
}

func (c cancelOnClose) Close() error {
	err := c.ReadCloser.Close()
	c.cancel()
	return err
}

func webdavStatusError(action string, resp *http.Response) error {
	switch resp.StatusCode {
	case http.StatusUnauthorized, http.StatusForbidden:
		return fmt.Errorf("%s失败：账号或密码不对（坚果云要用“应用密码”）", action)
	case http.StatusNotFound:
		return fmt.Errorf("%s失败：文件或目录不存在", action)
	case http.StatusConflict:
		return fmt.Errorf("%s失败：上级目录不存在", action)
	case http.StatusInsufficientStorage:
		return fmt.Errorf("%s失败：网盘空间不足", action)
	}
	return fmt.Errorf("%s失败：服务器返回 %s", action, resp.Status)
}

// ensureDir 确认备份目录存在，不存在就创建。
func (c *webdavClient) ensureDir() error {
	resp, err := c.do("PROPFIND", "", nil, map[string]string{"Depth": "0"})
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode == http.StatusMultiStatus || resp.StatusCode == http.StatusOK {
		return nil
	}
	if resp.StatusCode != http.StatusNotFound {
		return webdavStatusError("访问备份目录", resp)
	}
	resp, err = c.do("MKCOL", "", nil, nil)
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode == http.StatusCreated || resp.StatusCode == http.StatusMethodNotAllowed {
		return nil
	}
	return webdavStatusError("创建备份目录", resp)
}

func (c *webdavClient) put(name string, data []byte) error {
	resp, err := c.do(http.MethodPut, name, data, map[string]string{"Content-Type": "application/octet-stream"})
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return webdavStatusError("上传备份", resp)
	}
	return nil
}

func (c *webdavClient) get(name string) ([]byte, error) {
	if !isBackupFile(name) || strings.ContainsAny(name, `/\`) {
		return nil, fmt.Errorf("备份文件名无效: %s", name)
	}
	resp, err := c.do(http.MethodGet, name, nil, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, webdavStatusError("下载备份", resp)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 32<<20))
}

func (c *webdavClient) delete(name string) error {
	resp, err := c.do(http.MethodDelete, name, nil, nil)
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode/100 != 2 && resp.StatusCode != http.StatusNotFound {
		return webdavStatusError("删除旧备份", resp)
	}
	return nil
}

const propfindBody = `<?xml version="1.0" encoding="utf-8"?><d:propfind xmlns:d="DAV:"><d:prop><d:getcontentlength/><d:getlastmodified/></d:prop></d:propfind>`

// list 返回目录里的 sbtun 备份，按文件名（即时间）从新到旧排序。
func (c *webdavClient) list() ([]WebDAVBackupDTO, error) {
	resp, err := c.do("PROPFIND", "", []byte(propfindBody), map[string]string{"Depth": "1", "Content-Type": "application/xml; charset=utf-8"})
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusMultiStatus {
		return nil, webdavStatusError("读取备份列表", resp)
	}
	var doc struct {
		Responses []struct {
			Href  string `xml:"href"`
			Props []struct {
				Length   int64  `xml:"prop>getcontentlength"`
				Modified string `xml:"prop>getlastmodified"`
			} `xml:"propstat"`
		} `xml:"response"`
	}
	if err := xml.NewDecoder(io.LimitReader(resp.Body, 8<<20)).Decode(&doc); err != nil {
		return nil, fmt.Errorf("备份列表格式错误: %w", err)
	}
	var out []WebDAVBackupDTO
	for _, r := range doc.Responses {
		href := r.Href
		if u, err := url.Parse(href); err == nil {
			href = u.Path
		}
		name := path.Base(strings.TrimSuffix(href, "/"))
		if !isBackupFile(name) {
			continue
		}
		item := WebDAVBackupDTO{Name: name, Encrypted: strings.HasSuffix(name, ".sbtun")}
		for _, p := range r.Props {
			if p.Length > 0 {
				item.Size = p.Length
			}
			if t, err := http.ParseTime(p.Modified); err == nil {
				item.Modified = t.Local().Format("2006-01-02 15:04:05")
			}
		}
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name > out[j].Name })
	return out, nil
}
