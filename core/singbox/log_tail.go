package singbox

import (
	"os"
	"regexp"
	"strings"
)

var (
	ansiEscape = regexp.MustCompile(`\x1b\[[0-9;]*m`)
	// sing-box 日志行形如 "+0800 2026-10-23 15:00:00 FATAL[0000] start service: ..."。
	logLevel = regexp.MustCompile(`\b(FATAL|ERROR|WARN|INFO|DEBUG|TRACE)\b(\[\d+\])?\s*`)
)

// LastError 返回最近一次启动的 sing-box 日志里最后一条 FATAL / ERROR，去掉时间和级别前缀；
// 没有时返回空。用来在启动失败时把真正原因（如缺少管理员权限、wintun.dll）告诉用户。
func (m *Manager) LastError() string {
	m.mu.Lock()
	path := m.logPath
	m.mu.Unlock()
	if path == "" {
		return ""
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	lines := strings.Split(ansiEscape.ReplaceAllString(string(data), ""), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := lines[i]
		loc := logLevel.FindStringSubmatchIndex(line)
		if loc == nil {
			continue
		}
		if level := line[loc[2]:loc[3]]; level != "FATAL" && level != "ERROR" {
			continue
		}
		msg := strings.TrimSpace(line[loc[1]:])
		if len(msg) > 300 {
			msg = msg[:300] + "…"
		}
		return msg
	}
	return ""
}
