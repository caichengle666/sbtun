package app

import (
	"strings"
	"testing"
)

func TestTerminalQRCode(t *testing.T) {
	code, err := TerminalQRCode("vless://uuid@example.com:443?security=tls#test")
	if err != nil {
		t.Fatal(err)
	}
	if lines := strings.Count(code, "\n"); lines < 10 {
		t.Fatalf("二维码行数异常: %d", lines)
	}
}
