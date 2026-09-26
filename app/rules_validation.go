package app

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

func (a *App) validateRuleSet(path string) error {
	if a.binary == "" || !fileExists(a.binary) {
		return errors.New("未找到 sing-box 内核，无法验证规则集")
	}
	ctx := a.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	// match 会按 sing-box 实际加载规则集的方式解析 binary 格式，兼容
	// sing-box SRS 和由 AdGuard DNS 规则转换得到的 SRS。
	command := exec.CommandContext(ctx, a.binary, "rule-set", "match", "--format", "binary", path, "example.com")
	configureProcess(command)
	result, err := command.CombinedOutput()
	if err == nil {
		return nil
	}
	message := strings.TrimSpace(string(result))
	if len(message) > 240 {
		message = message[len(message)-240:]
	}
	if message == "" {
		return fmt.Errorf("sing-box 无法读取该规则集: %w", err)
	}
	return fmt.Errorf("sing-box 无法读取该规则集: %s", message)
}
