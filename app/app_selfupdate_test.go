package app

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func writeTestZip(t *testing.T, path string, files map[string]string) {
	t.Helper()
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for name, content := range files {
		header := &zip.FileHeader{Name: name, Method: zip.Deflate}
		header.SetMode(0o755)
		entry, err := writer.CreateHeader(header)
		if err != nil {
			t.Fatal(err)
		}
		entry.Write([]byte(content))
	}
	writer.Close()
	if err := os.WriteFile(path, buffer.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeTestTarGz(t *testing.T, path string, files map[string]string) {
	t.Helper()
	var buffer bytes.Buffer
	compressed := gzip.NewWriter(&buffer)
	writer := tar.NewWriter(compressed)
	for name, content := range files {
		writer.WriteHeader(&tar.Header{Name: name, Mode: 0o755, Size: int64(len(content)), Typeflag: tar.TypeReg})
		writer.Write([]byte(content))
	}
	writer.Close()
	compressed.Close()
	if err := os.WriteFile(path, buffer.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readTestFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestInstallPortableWindowsZipReplacesOnlyProgramFiles(t *testing.T) {
	dir := t.TempDir()
	for name, content := range map[string]string{"sbtun.exe": "old gui", "sbtun-cli.exe": "old cli", "sing-box.exe": "my kernel", "VERSION": "0.1.0"} {
		os.WriteFile(filepath.Join(dir, name), []byte(content), 0o755)
	}
	archive := filepath.Join(dir, ".sbtun-update-1")
	writeTestZip(t, archive, map[string]string{"sbtun.exe": "new gui", "sbtun-cli.exe": "new cli", "sing-box.exe": "bundled kernel", "VERSION": "0.2.0", "rules/geoip-cn.srs": "x"})
	if err := installPortable(archive, dir, "windows"); err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]string{"sbtun.exe": "new gui", "sbtun-cli.exe": "new cli", "sing-box.exe": "my kernel", "VERSION": "0.2.0"} {
		if got := readTestFile(t, filepath.Join(dir, name)); got != want {
			t.Fatalf("%s = %q, want %q", name, got, want)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "rules")); !os.IsNotExist(err) {
		t.Fatal("rules must not be extracted")
	}
	leftovers, _ := filepath.Glob(filepath.Join(dir, "*.old-*"))
	staged, _ := filepath.Glob(filepath.Join(dir, "*.new*"))
	if len(leftovers)+len(staged) != 0 {
		t.Fatalf("leftovers: %v %v", leftovers, staged)
	}
}

func TestInstallPortableLinuxTarGzInSubdirectory(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "sbtun"), []byte("old"), 0o755)
	archive := filepath.Join(t.TempDir(), "pkg.tar.gz")
	writeTestTarGz(t, archive, map[string]string{
		"sbtun-linux-arm64/sbtun":            "new gui",
		"sbtun-linux-arm64/sbtun-cli":        "new cli",
		"sbtun-linux-arm64/start-sbtun.sh":   "#!/bin/sh",
		"sbtun-linux-arm64/libs/sbtun":       "deep file must be ignored",
		"sbtun-linux-arm64/webkit/sbtun-cli": "deep file must be ignored",
	})
	if err := installPortable(archive, dir, "linux"); err != nil {
		t.Fatal(err)
	}
	if got := readTestFile(t, filepath.Join(dir, "sbtun")); got != "new gui" {
		t.Fatalf("sbtun = %q", got)
	}
	if got := readTestFile(t, filepath.Join(dir, "sbtun-cli")); got != "new cli" {
		t.Fatalf("sbtun-cli = %q", got)
	}
	if info, _ := os.Stat(filepath.Join(dir, "sbtun-cli")); info.Mode().Perm()&0o100 == 0 {
		t.Fatal("binary must stay executable")
	}
}

func TestInstallPortableMissingBinaryKeepsOldFiles(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "sbtun"), []byte("old"), 0o755)
	archive := filepath.Join(t.TempDir(), "pkg.tar.gz")
	writeTestTarGz(t, archive, map[string]string{"pkg/sbtun": "new"})
	if err := installPortable(archive, dir, "linux"); err == nil {
		t.Fatal("expected missing sbtun-cli error")
	}
	if got := readTestFile(t, filepath.Join(dir, "sbtun")); got != "old" {
		t.Fatalf("sbtun changed to %q", got)
	}
	if staged, _ := filepath.Glob(filepath.Join(dir, "*.new*")); len(staged) != 0 {
		t.Fatalf("staged files left: %v", staged)
	}
}

func TestInstallDarwinBundleSwapsApp(t *testing.T) {
	parent := t.TempDir()
	exe := filepath.Join(parent, "sbtun.app", "Contents", "MacOS", "sbtun")
	os.MkdirAll(filepath.Dir(exe), 0o755)
	os.WriteFile(exe, []byte("old"), 0o755)
	archive := filepath.Join(t.TempDir(), "mac.zip")
	writeTestZip(t, archive, map[string]string{
		"sbtun.app/Contents/MacOS/sbtun":     "new",
		"sbtun.app/Contents/MacOS/sbtun-cli": "new cli",
		"sbtun.app/Contents/Info.plist":      "plist",
	})
	if err := installDarwinBundle(archive, exe); err != nil {
		t.Fatal(err)
	}
	if got := readTestFile(t, exe); got != "new" {
		t.Fatalf("sbtun = %q", got)
	}
	entries, _ := os.ReadDir(parent)
	if len(entries) != 1 {
		t.Fatalf("leftovers beside the bundle: %v", entries)
	}
}

func TestExtractZipRejectsTraversal(t *testing.T) {
	archive := filepath.Join(t.TempDir(), "bad.zip")
	writeTestZip(t, archive, map[string]string{"../evil": "x"})
	if err := extractZip(archive, t.TempDir()); err == nil {
		t.Fatal("expected traversal error")
	}
}

func TestDownloadAppArchiveChecksDigest(t *testing.T) {
	body := []byte("package")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.Write(body) }))
	defer server.Close()
	sum := sha256.Sum256(body)
	dir := t.TempDir()
	good := githubAsset{Name: "a.zip", BrowserDownloadURL: server.URL, Digest: "sha256:" + hex.EncodeToString(sum[:])}
	path, err := New().downloadAppArchive(context.Background(), server.Client(), good, dir)
	if err != nil || readTestFile(t, path) != "package" {
		t.Fatalf("download failed: %v", err)
	}
	os.Remove(path)
	bad := good
	bad.Digest = "sha256:" + hex.EncodeToString(make([]byte, 32))
	if _, err := New().downloadAppArchive(context.Background(), server.Client(), bad, dir); err == nil {
		t.Fatal("expected digest error")
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Fatalf("failed download left files: %v", entries)
	}
}

func TestUpdateLeftoverNames(t *testing.T) {
	for name, want := range map[string]bool{
		"sbtun.exe.old-abc":    true,
		"sbtun-cli.new":        true,
		"sbtun-cli.new-k3x9":   true,
		"notes.new-k3x9":       false,
		".sbtun-update-123":    true,
		"sbtun.app.old-x":      true,
		"config.json.old-x":    false,
		"sing-box.exe.old-abc": false,
		"notes.new":            false,
	} {
		if got := isUpdateLeftover(name); got != want {
			t.Errorf("isUpdateLeftover(%q) = %v, want %v", name, got, want)
		}
	}
}

func TestAppAssetName(t *testing.T) {
	for _, item := range []struct{ goos, goarch, want string }{
		{"windows", "amd64", "sbtun-windows-amd64.zip"},
		{"linux", "arm64", "sbtun-linux-arm64.tar.gz"},
		{"darwin", "arm64", "sbtun-macos-arm64.zip"},
	} {
		if got, err := appAssetName(item.goos, item.goarch); err != nil || got != item.want {
			t.Fatalf("%s/%s = %q, %v", item.goos, item.goarch, got, err)
		}
	}
	if _, err := appAssetName("linux", "386"); err == nil {
		t.Fatal("expected unsupported arch")
	}
}

func TestGitHubReleasesPageURL(t *testing.T) {
	got, err := githubReleasesPageURL("https://api.github.com/repos/caichengle666/sbtun/releases/latest")
	if err != nil || got != "https://github.com/caichengle666/sbtun/releases/latest" {
		t.Fatalf("got %q, %v", got, err)
	}
}
