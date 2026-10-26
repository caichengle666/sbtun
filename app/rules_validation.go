package app

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/caichengle666/sbtun/core/rules"
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
	// match 会按 sing-box 实际加载规则集的方式解析：.srs（含 AdGuard 转换的）按 binary，
	// 文本列表转换出的 .json 按 source。
	command := exec.CommandContext(ctx, a.binary, "rule-set", "match", "--format", rules.FileFormat(path), path, "example.com")
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
