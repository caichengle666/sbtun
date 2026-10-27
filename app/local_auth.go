package app

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync/atomic"

	"github.com/caichengle666/sbtun/config"
	"github.com/caichengle666/sbtun/core/singbox"
)

const localAuthFile = "local-auth.json"

// clashAPISecret is the Bearer secret for the sing-box clash API. It is shared
// by the GUI and CLI through the runtime data directory.
var clashAPISecret atomic.Value

func setClashAPISecret(secret string) { clashAPISecret.Store(secret) }

func currentClashAPISecret() string {
	secret, _ := clashAPISecret.Load().(string)
	return secret
}

// authorizeClash adds the clash API secret to a request when one is set.
func authorizeClash(req *http.Request) *http.Request {
	if secret := currentClashAPISecret(); secret != "" {
		req.Header.Set("Authorization", "Bearer "+secret)
	}
	return req
}

func randomToken(bytes int) (string, error) {
	buf := make([]byte, bytes)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func newLocalAuth() (singbox.LocalAuth, error) {
	secret, err := randomToken(24)
	if err != nil {
		return singbox.LocalAuth{}, err
	}
	password, err := randomToken(24)
	if err != nil {
		return singbox.LocalAuth{}, err
	}
	return singbox.LocalAuth{ClashSecret: secret, ProxyUsername: "sbtun", ProxyPassword: password}, nil
}

func validLocalAuth(auth singbox.LocalAuth) bool {
	return auth.ClashSecret != "" && auth.ProxyUsername != "" && auth.ProxyPassword != ""
}

// loadOrCreateLocalAuth returns the credentials that protect the local clash
// API and capture proxy ports. They are generated once per runtime directory
// so the GUI and CLI agree on them across processes and restarts.
func loadOrCreateLocalAuth(workDir string) (singbox.LocalAuth, error) {
	path := filepath.Join(workDir, localAuthFile)
	var auth singbox.LocalAuth
	err := config.WithFileLock(path, func() error {
		data, readErr := os.ReadFile(path)
		if readErr == nil {
			if json.Unmarshal(data, &auth) == nil && validLocalAuth(auth) {
				_ = config.ShareWithInvoker(path, 0o600) // 修复旧版本在 sudo 下留下的 root 文件
				return nil
			}
		} else if !errors.Is(readErr, os.ErrNotExist) {
			return fmt.Errorf("读取本地认证信息失败: %w", readErr)
		}
		created, genErr := newLocalAuth()
		if genErr != nil {
			return fmt.Errorf("生成本地认证信息失败: %w", genErr)
		}
		encoded, encErr := json.MarshalIndent(created, "", "  ")
		if encErr != nil {
			return encErr
		}
		if err := os.MkdirAll(workDir, 0o755); err != nil {
			return fmt.Errorf("创建运行目录失败: %w", err)
		}
		tmp, tmpErr := os.CreateTemp(workDir, ".local-auth-*.tmp")
		if tmpErr != nil {
			return tmpErr
		}
		name := tmp.Name()
		defer os.Remove(name)
		if _, err := tmp.Write(encoded); err != nil {
			_ = tmp.Close()
			return err
		}
		if err := tmp.Close(); err != nil {
			return err
		}
		// 只有本人可读，但在 sudo/pkexec 下交还调用者：普通用户的 CLI 要用同一套密钥访问运行中的实例。
		if err := config.ShareWithInvoker(name, 0o600); err != nil {
			return err
		}
		if err := config.ReplaceConfigFile(name, path); err != nil {
			return fmt.Errorf("保存本地认证信息失败: %w", err)
		}
		auth = created
		return nil
	})
	return auth, err
}
