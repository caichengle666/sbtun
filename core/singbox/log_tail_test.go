package singbox

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLastError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sing-box.log")
	log := "+0800 2026-10-23 15:00:00 \x1b[36mINFO\x1b[0m[0000] network: updated default interface\n" +
		"+0800 2026-10-23 15:00:00 \x1b[31mFATAL\x1b[0m[0000] start service: start inbound/tun[tun-in]: configure tun interface: Access is denied.\n" +
		"+0800 2026-10-23 15:00:01 \x1b[36mINFO\x1b[0m[0001] closing\n"
	if err := os.WriteFile(path, []byte(log), 0o644); err != nil {
		t.Fatal(err)
	}
	m := &Manager{logPath: path}
	want := "start service: start inbound/tun[tun-in]: configure tun interface: Access is denied."
	if got := m.LastError(); got != want {
		t.Fatalf("got %q", got)
	}
	if got := (&Manager{}).LastError(); got != "" {
		t.Fatalf("no log: %q", got)
	}
}
