//go:build !windows

package singbox

import (
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"testing"
	"time"
)

func startHelper(t *testing.T, mode string) (*Manager, *exec.Cmd) {
	t.Helper()
	marker := t.TempDir() + "/graceful"
	cmd := exec.Command(os.Args[0], "-test.run=TestGracefulStopHelperProcess")
	cmd.Env = append(os.Environ(), "SBTUN_GRACEFUL_HELPER="+mode, "SBTUN_GRACEFUL_MARKER="+marker)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	m := NewManager()
	m.cmd = cmd
	m.done = make(chan struct{})
	m.generation = 1
	m.current = 1
	go m.wait(cmd, 1)
	// Give the helper time to install its signal handler.
	time.Sleep(200 * time.Millisecond)
	return m, cmd
}

func TestStopSendsSIGTERMFirst(t *testing.T) {
	m, cmd := startHelper(t, "graceful")
	if err := m.Stop(); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}
	if !cmd.ProcessState.Success() {
		t.Fatalf("helper should exit cleanly after SIGTERM, state=%v", cmd.ProcessState)
	}
	event := <-m.Exited()
	if !event.Expected || event.Generation != 1 {
		t.Fatalf("unexpected exit event: %+v", event)
	}
	if m.Generation() != 0 {
		t.Fatalf("generation after exit=%d want 0", m.Generation())
	}
}

func TestStopKillsProcessIgnoringSIGTERM(t *testing.T) {
	m, cmd := startHelper(t, "ignore")
	started := time.Now()
	if err := m.Stop(); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}
	if elapsed := time.Since(started); elapsed < gracefulStopTimeout {
		t.Fatalf("Stop killed before the graceful timeout: %v", elapsed)
	}
	if cmd.ProcessState.Success() {
		t.Fatal("helper ignoring SIGTERM should have been killed")
	}
}

func TestGracefulStopHelperProcess(t *testing.T) {
	mode := os.Getenv("SBTUN_GRACEFUL_HELPER")
	if mode == "" {
		return
	}
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGTERM)
	if mode == "graceful" {
		<-signals
		os.Exit(0)
	}
	time.Sleep(time.Minute)
}
