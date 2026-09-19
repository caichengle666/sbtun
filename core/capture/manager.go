package capture

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/elazarl/goproxy"
)

const (
	maxVisibleFlows = 500
	maxStoredFlows  = 2000
	maxBodyBytes    = 256 * 1024
)

type Flow struct {
	ID                uint64              `json:"id"`
	StartedAt         time.Time           `json:"started_at"`
	Method            string              `json:"method"`
	URL               string              `json:"url"`
	Host              string              `json:"host"`
	Protocol          string              `json:"protocol"`
	StatusCode        int                 `json:"status_code"`
	DurationMS        int64               `json:"duration_ms"`
	RequestBytes      int64               `json:"request_bytes"`
	ResponseBytes     int64               `json:"response_bytes"`
	RequestHeaders    map[string][]string `json:"request_headers"`
	ResponseHeaders   map[string][]string `json:"response_headers"`
	RequestBody       string              `json:"request_body,omitempty"`
	ResponseBody      string              `json:"response_body,omitempty"`
	RequestEncoding   string              `json:"request_encoding,omitempty"`
	ResponseEncoding  string              `json:"response_encoding,omitempty"`
	RequestTruncated  bool                `json:"request_truncated,omitempty"`
	ResponseTruncated bool                `json:"response_truncated,omitempty"`
}

type Status struct {
	Enabled              bool   `json:"enabled"`
	Running              bool   `json:"running"`
	Address              string `json:"address"`
	CertificatePath      string `json:"certificate_path"`
	CertificateInstalled bool   `json:"certificate_installed"`
	StoragePath          string `json:"storage_path"`
	FlowCount            int    `json:"flow_count"`
	Unsaved              bool   `json:"unsaved"`
	Message              string `json:"message"`
}

type pendingFlow struct {
	flow        Flow
	requestBody []byte
}

type Manager struct {
	mu        sync.RWMutex
	workDir   string
	address   string
	upstream  string
	server    *http.Server
	listener  net.Listener
	transport *http.Transport
	domains   []string
	flows     []Flow
	nextID    atomic.Uint64
	loaded    bool
	dirty     bool
}

func NewManager(workDir, address, upstream string) *Manager {
	return &Manager{workDir: workDir, address: address, upstream: upstream}
}

func (m *Manager) Start(ctx context.Context, domains []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.server != nil {
		return nil
	}
	domains = normalizeDomains(domains)
	if len(domains) == 0 {
		return errors.New("至少需要一个分析域名")
	}
	if err := m.loadLocked(); err != nil {
		return err
	}
	ca, err := m.ensureCA()
	if err != nil {
		return err
	}
	listener, err := net.Listen("tcp", m.address)
	if err != nil {
		return fmt.Errorf("启动分析代理失败: %w", err)
	}
	m.domains = domains
	proxy := goproxy.NewProxyHttpServer()
	proxy.AllowHTTP2 = true
	proxy.Verbose = false
	proxy.Tr = m.transport
	if proxy.Tr == nil {
		transport := &http.Transport{ForceAttemptHTTP2: true}
		if m.upstream != "" {
			upstreamURL, err := url.Parse("http://" + m.upstream)
			if err != nil {
				listener.Close()
				return fmt.Errorf("解析分析上游地址失败: %w", err)
			}
			transport.Proxy = http.ProxyURL(upstreamURL)
		}
		proxy.Tr = transport
	}
	if m.upstream != "" {
		proxy.ConnectDial = proxy.NewConnectDialToProxy("http://" + m.upstream)
	}
	mitm := &goproxy.ConnectAction{Action: goproxy.ConnectMitm, TLSConfig: goproxy.TLSConfigFromCA(&ca)}
	proxy.OnRequest().HandleConnectFunc(func(host string, _ *goproxy.ProxyCtx) (*goproxy.ConnectAction, string) {
		if matchesDomain(host, domains) {
			return mitm, host
		}
		return goproxy.OkConnect, host
	})
	proxy.OnRequest().DoFunc(func(req *http.Request, proxyCtx *goproxy.ProxyCtx) (*http.Request, *http.Response) {
		if !matchesDomain(req.Host, domains) {
			return req, nil
		}
		body, replacement, truncated, err := captureBody(req.Body, maxBodyBytes)
		req.Body = replacement
		if err != nil {
			return req, goproxy.NewResponse(req, goproxy.ContentTypeText, http.StatusBadGateway, err.Error())
		}
		proxyCtx.UserData = &pendingFlow{requestBody: body, flow: Flow{
			ID:               m.nextID.Add(1),
			StartedAt:        time.Now(),
			Method:           req.Method,
			URL:              req.URL.String(),
			Host:             hostname(req.Host),
			Protocol:         req.Proto,
			RequestBytes:     req.ContentLength,
			RequestHeaders:   clonedHeaders(req.Header),
			RequestTruncated: truncated,
		}}
		return req, nil
	})
	proxy.OnResponse().DoFunc(func(resp *http.Response, proxyCtx *goproxy.ProxyCtx) *http.Response {
		pending, ok := proxyCtx.UserData.(*pendingFlow)
		if !ok || pending == nil {
			return resp
		}
		flow := pending.flow
		flow.DurationMS = time.Since(flow.StartedAt).Milliseconds()
		if resp != nil {
			body, replacement, truncated, err := captureBody(resp.Body, maxBodyBytes)
			resp.Body = replacement
			if err == nil {
				flow.ResponseTruncated = truncated
				flow.ResponseBody, flow.ResponseEncoding = bodyForDisplay(body, resp.Header.Get("Content-Type"))
			}
			flow.StatusCode = resp.StatusCode
			flow.ResponseBytes = resp.ContentLength
			flow.ResponseHeaders = clonedHeaders(resp.Header)
		}
		flow.RequestBody, flow.RequestEncoding = bodyForDisplay(pending.requestBody, http.Header(flow.RequestHeaders).Get("Content-Type"))
		_ = m.appendFlow(flow)
		return resp
	})
	server := &http.Server{Handler: proxy, ReadHeaderTimeout: 10 * time.Second}
	m.listener = listener
	m.server = server
	go func() {
		_ = server.Serve(listener)
	}()
	return nil
}

func (m *Manager) Stop() error {
	m.mu.Lock()
	server := m.server
	m.server = nil
	m.listener = nil
	m.mu.Unlock()
	if server == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func (m *Manager) Status(enabled bool) Status {
	m.mu.RLock()
	_, markerErr := os.Stat(m.installMarkerPath())
	address := m.address
	if m.listener != nil {
		address = m.listener.Addr().String()
	}
	running := m.server != nil
	m.mu.RUnlock()
	return Status{
		Enabled: enabled, Running: running, Address: address,
		CertificatePath: m.certPath(), CertificateInstalled: markerErr == nil,
		StoragePath: m.storagePath(), FlowCount: m.flowCount(), Unsaved: m.isDirty(),
	}
}

func (m *Manager) InstallCertificate() error {
	m.mu.Lock()
	_, err := m.ensureCA()
	m.mu.Unlock()
	if err != nil {
		return err
	}
	if err := installCertificate(m.certPath()); err != nil {
		return err
	}
	return os.WriteFile(m.installMarkerPath(), []byte(time.Now().Format(time.RFC3339)), 0o600)
}

func (m *Manager) UninstallCertificate() error {
	if err := uninstallCertificate(m.certPath()); err != nil {
		return err
	}
	if err := os.Remove(m.installMarkerPath()); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func (m *Manager) ensureCA() (tls.Certificate, error) {
	if err := os.MkdirAll(m.workDir, 0o700); err != nil {
		return tls.Certificate{}, fmt.Errorf("创建分析证书目录失败: %w", err)
	}
	if cert, err := loadCA(m.certPath(), m.keyPath()); err == nil {
		return cert, nil
	}
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return tls.Certificate{}, err
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return tls.Certificate{}, err
	}
	now := time.Now()
	template := x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: "sbtun Local Capture CA", Organization: []string{"sbtun"}},
		NotBefore:    now.Add(-time.Hour), NotAfter: now.AddDate(10, 0, 0),
		IsCA: true, BasicConstraintsValid: true,
		KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageCRLSign | x509.KeyUsageDigitalSignature,
	}
	der, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		return tls.Certificate{}, err
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	if err := os.WriteFile(m.certPath(), certPEM, 0o600); err != nil {
		return tls.Certificate{}, err
	}
	if err := os.WriteFile(m.keyPath(), keyPEM, 0o600); err != nil {
		return tls.Certificate{}, err
	}
	return loadCA(m.certPath(), m.keyPath())
}

func loadCA(certPath, keyPath string) (tls.Certificate, error) {
	cert, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		return tls.Certificate{}, err
	}
	if len(cert.Certificate) == 0 {
		return tls.Certificate{}, errors.New("分析 CA 证书为空")
	}
	cert.Leaf, err = x509.ParseCertificate(cert.Certificate[0])
	return cert, err
}

func (m *Manager) certPath() string          { return filepath.Join(m.workDir, "sbtun-capture-ca.crt") }
func (m *Manager) keyPath() string           { return filepath.Join(m.workDir, "sbtun-capture-ca.key") }
func (m *Manager) installMarkerPath() string { return filepath.Join(m.workDir, ".installed") }
func (m *Manager) storagePath() string       { return filepath.Join(m.workDir, "capture.json") }

func captureBody(body io.ReadCloser, limit int64) ([]byte, io.ReadCloser, bool, error) {
	if body == nil {
		return nil, nil, false, nil
	}
	read, err := io.ReadAll(io.LimitReader(body, limit+1))
	replacement := &combinedReadCloser{Reader: io.MultiReader(bytes.NewReader(read), body), Closer: body}
	if err != nil {
		return append([]byte(nil), read...), replacement, false, err
	}
	truncated := int64(len(read)) > limit
	captured := read
	if truncated {
		captured = read[:limit]
	}
	return append([]byte(nil), captured...), replacement, truncated, nil
}

type combinedReadCloser struct {
	io.Reader
	io.Closer
}

func normalizeDomains(domains []string) []string {
	seen := map[string]struct{}{}
	result := make([]string, 0, len(domains))
	for _, domain := range domains {
		domain = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(domain)), ".")
		if domain == "" {
			continue
		}
		if _, exists := seen[domain]; exists {
			continue
		}
		seen[domain] = struct{}{}
		result = append(result, domain)
	}
	sort.Strings(result)
	return result
}

func matchesDomain(host string, domains []string) bool {
	host = hostname(host)
	for _, domain := range domains {
		if strings.Contains(domain, ".") && (host == domain || strings.HasSuffix(host, "."+domain)) {
			return true
		}
		if !strings.Contains(domain, ".") && strings.Contains(host, domain) {
			return true
		}
	}
	return false
}

func hostname(hostport string) string {
	hostport = strings.TrimSpace(strings.ToLower(hostport))
	if host, _, err := net.SplitHostPort(hostport); err == nil {
		return strings.Trim(host, "[]")
	}
	return strings.Trim(hostport, "[]")
}

func clonedHeaders(headers http.Header) map[string][]string {
	result := make(map[string][]string, len(headers))
	for name, values := range headers {
		result[name] = append([]string(nil), values...)
	}
	return result
}
