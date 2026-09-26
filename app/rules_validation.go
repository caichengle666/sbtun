package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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
	output, err := os.CreateTemp(filepath.Dir(path), ".rule-set-validation-*.json")
	if err != nil {
		return fmt.Errorf("创建规则集校验文件失败: %w", err)
	}
	outputPath := output.Name()
	if err := output.Close(); err != nil {
		_ = os.Remove(outputPath)
		return fmt.Errorf("关闭规则集校验文件失败: %w", err)
	}
	defer os.Remove(outputPath)
	command := exec.CommandContext(ctx, a.binary, "rule-set", "decompile", path, "-o", outputPath)
	command.Dir = filepath.Dir(a.binary)
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
