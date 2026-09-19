//go:build cli

package main

import (
	"os"
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
		{command: "test", want: "sbtun test"},
		{command: "del", want: "sbtun del"},
		{command: "capture", args: []string{"cert"}, want: "capture cert"},
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
		{command: "info", args: []string{"1"}},
		{command: "rules", args: []string{"update-all"}},
		{command: "capture", args: []string{"enable", "*"}},
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
