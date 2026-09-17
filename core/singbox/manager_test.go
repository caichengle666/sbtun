package singbox

import (
	"os"
	"os/exec"
	"testing"
	"time"
)

func TestStopWaitsForProcessExit(t *testing.T) {
	cmd := exec.Command(os.Args[0], "-test.run=TestStopHelperProcess")
	cmd.Env = append(os.Environ(), "SBTUN_STOP_HELPER=1")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	m := NewManager()
	m.cmd = cmd
	m.done = make(chan struct{})
	go m.wait(cmd)

	if err := m.Stop(); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}
	if m.Running() {
		t.Fatal("Stop returned while process was still marked running")
	}
	select {
	case <-m.exited:
	default:
		t.Fatal("Stop returned before the exit event was recorded")
	}
}

func TestStopHelperProcess(t *testing.T) {
	if os.Getenv("SBTUN_STOP_HELPER") != "1" {
		return
	}
	time.Sleep(time.Minute)
}
