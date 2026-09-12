package singbox

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"syscall"
)

// ValidateConfig 使用 sing-box 自身的 check 命令校验 runtime 配置。
// 不自行复制 sing-box 的 schema 校验逻辑，避免核心升级后产生第二套规则。
func ValidateConfig(ctx context.Context, binary, configPath string) error {
	if binary == "" {
		return fmt.Errorf("sing-box 程序路径不能为空")
	}
	if configPath == "" {
		return fmt.Errorf("sing-box 配置路径不能为空")
	}
	cmd := exec.CommandContext(ctx, binary, "check", "-c", configPath)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	cmd.Stdout = &stderr
	if err := cmd.Run(); err != nil {
		if msg := stderr.String(); msg != "" {
			return fmt.Errorf("sing-box 配置校验失败: %s", msg)
		}
		return fmt.Errorf("sing-box 配置校验失败: %w", err)
	}
	return nil
}
