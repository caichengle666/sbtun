package app

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestLocalAuthIsSharedAcrossProcesses(t *testing.T) {
	dir := t.TempDir()
	first, err := loadOrCreateLocalAuth(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !validLocalAuth(first) {
		t.Fatalf("generated auth incomplete: %+v", first)
	}
	second, err := loadOrCreateLocalAuth(dir)
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatal("a second process must reuse the saved credentials")
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(filepath.Join(dir, localAuthFile))
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Fatalf("local auth file mode=%v want 0600", info.Mode().Perm())
		}
	}
}

func TestLocalAuthRegeneratesInvalidFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, localAuthFile), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	auth, err := loadOrCreateLocalAuth(dir)
	if err != nil || !validLocalAuth(auth) {
		t.Fatalf("auth=%+v err=%v", auth, err)
	}
}

func TestClashRequestsCarrySecret(t *testing.T) {
	var got string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Get("Authorization")
		_, _ = w.Write([]byte(`{"now":"node-a"}`))
	}))
	defer server.Close()
	previousURL := clashAPIBaseURL
	previousSecret := currentClashAPISecret()
	clashAPIBaseURL = server.URL
	setClashAPISecret("s3cret")
	defer func() {
		clashAPIBaseURL = previousURL
		setClashAPISecret(previousSecret)
	}()
	if _, err := currentSelector(); err != nil {
		t.Fatal(err)
	}
	if got != "Bearer s3cret" {
		t.Fatalf("Authorization=%q", got)
	}
}

func TestFetchURLRejectsOversizedSubscription(t *testing.T) {
	chunk := strings.Repeat("a", 1<<20)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// No Content-Length: the limit must hold while streaming too.
		for i := 0; i < 9; i++ {
			_, _ = w.Write([]byte(chunk))
			w.(http.Flusher).Flush()
		}
	}))
	defer server.Close()
	if _, err := fetchURL(server.URL); err == nil || !strings.Contains(err.Error(), "上限") {
		t.Fatalf("oversized subscription err=%v", err)
	}
}
