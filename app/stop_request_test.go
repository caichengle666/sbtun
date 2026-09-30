package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWaitForStopRequestConsumesRequest(t *testing.T) {
	a := New()
	a.workDir = t.TempDir()
	request := filepath.Join(a.workDir, stopRequestFile)
	if err := os.WriteFile(request, []byte("stop\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if !a.WaitForStopRequest(ctx) {
		t.Fatal("stop request was not detected")
	}
	if _, err := os.Stat(request); !os.IsNotExist(err) {
		t.Fatalf("stop request was not consumed: %v", err)
	}
}

func TestWaitForStopRequestStopsWithContext(t *testing.T) {
	a := New()
	a.workDir = t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if a.WaitForStopRequest(ctx) {
		t.Fatal("unexpected stop request")
	}
}
