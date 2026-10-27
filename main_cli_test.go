//go:build cli

package main

import (
	"os"
	"runtime"
	"strings"
	"testing"
)

func TestParseCLIArgsWithoutCommandShowsHelp(t *testing.T) {
	command, args, ok := parseCLIArgs([]string{"sbtun-cli"})
	if ok || command != "" || args != nil {
		t.Fatalf("command=%q args=%v ok=%t", command, args, ok)
	}
}

func TestParseCLIArgsReturnsCommandArguments(t *testing.T) {
	command, args, ok := parseCLIArgs([]string{"sbtun-cli", "capture", "run", "example.com"})
	if !ok || command != "capture" || len(args) != 2 || args[0] != "run" || args[1] != "example.com" {
		t.Fatalf("command=%q args=%v ok=%t", command, args, ok)
	}
}

func TestValidateCLIArgsRejectsInvalidInputBeforeElevation(t *testing.T) {
	tests := []struct {
		command string
		args    []string
		want    string
	}{
		{command: "route", want: "sbtun route"},
		{command: "route", args: []string{"invalid"}, want: "无效路由模式"},
		{command: "auto-switch", args: []string{"maybe"}, want: "sbtun auto-switch"},
		{command: "test", args: []string{"0"}, want: "节点编号无效"},
		{command: "del", want: "sbtun del"},
		{command: "capture", args: []string{"cert"}, want: "capture cert"},
		{command: "kernel", want: "sbtun kernel"},
		{command: "kernel", args: []string{"invalid"}, want: "未知内核命令"},
	}
	for _, test := range tests {
		err := validateCLIArgs(test.command, test.args)
		if err == nil || !strings.Contains(err.Error(), test.want) {
			t.Fatalf("command=%s args=%v err=%v", test.command, test.args, err)
		}
	}
}

func TestValidateCLIArgsAcceptsSupportedCommands(t *testing.T) {
	for _, test := range []struct {
		command string
		args    []string
	}{
		{command: "route", args: []string{"smart"}},
		{command: "auto-switch", args: []string{"off"}},
		{command: "info", args: []string{"1"}},
		{command: "rules", args: []string{"update-all"}},
		{command: "rules", args: []string{"add", "AdGuard", "https://example.com/adguard.srs"}},
		{command: "rules", args: []string{"edit", "custom-1", "AdGuard 2", "https://example.com/adguard-2.srs"}},
		{command: "rules", args: []string{"delete", "custom-1"}},
		{command: "capture", args: []string{"enable", "*"}},
		{command: "kernel", args: []string{"version"}},
		{command: "kernel", args: []string{"update"}},
	} {
		if err := validateCLIArgs(test.command, test.args); err != nil {
			t.Fatalf("command=%s args=%v err=%v", test.command, test.args, err)
		}
	}
}

func TestIsTerminalInputRejectsRedirectedFile(t *testing.T) {
	input, err := os.CreateTemp(t.TempDir(), "stdin-*")
	if err != nil {
		t.Fatal(err)
	}
	defer input.Close()
	if isTerminalInput(input) {
		t.Fatal("regular file must not be treated as an interactive terminal")
	}
}

func TestValidateCLIArgsForSettingsAndRuleLists(t *testing.T) {
	for _, args := range [][]string{
		{"test"}, {"test", "1", "2"}, {"export"}, {"diag"},
		{"set", "ipv6", "on"}, {"set", "webrtc", "off"},
		{"custom", "add", "domain_suffix", "example.com", "direct"}, {"custom", "delete", "1"},
		{"filter", "add", "domain_keyword", "ads"}, {"filter", "list"},
		{"rules", "enable", "geosite-cn"}, {"rules", "restore"},
	} {
		if err := validateCLIArgs(args[0], args[1:]); err != nil {
			t.Fatalf("args=%v err=%v", args, err)
		}
	}
	for _, args := range [][]string{
		{"set", "ipv6"}, {"set", "dns", "on"}, {"test", "x"},
		{"custom", "add", "domain", "a.com"}, {"custom", "add", "regex", "a", "direct"},
		{"filter", "add", "domain", "a.com", "block"}, {"filter", "delete", "0"}, {"diag", "x"},
	} {
		if err := validateCLIArgs(args[0], args[1:]); err == nil {
			t.Fatalf("args=%v accepted", args)
		}
	}
	if !requiresElevation("set", []string{"ipv6", "on"}) || !requiresElevation("custom", []string{"add"}) || !requiresElevation("custom", []string{"edit"}) {
		t.Fatal("elevation rules wrong")
	}
	// 规则集的写命令要提权，和运行中的 root 实例操作同一份文件；只读命令不提权（macOS 一律提权）。
	if !requiresElevation("rules", []string{"add", "a", "https://x"}) || !requiresElevation("rules", []string{"update-all"}) {
		t.Fatal("rules writes must elevate")
	}
	if readOnly := requiresElevation("filter", nil) || requiresElevation("rules", []string{"list"}); readOnly != (runtime.GOOS == "darwin") {
		t.Fatal("read-only commands elevation wrong")
	}
	if !interactiveCommand("run", nil) || !interactiveCommand("edit", []string{"1"}) || interactiveCommand("edit", []string{"1", "n", "h", "443"}) || interactiveCommand("switch", []string{"1"}) {
		t.Fatal("interactive commands wrong")
	}
}

func TestShortMessageDropsSingBoxLog(t *testing.T) {
	got := shortMessage("URL 测试失败: 超时 +0800 2026-10-17 15:55:55 ERROR dial tcp")
	if got != "URL 测试失败: 超时" {
		t.Fatalf("got %q", got)
	}
}
