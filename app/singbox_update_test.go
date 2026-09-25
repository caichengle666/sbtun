package app

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDownloadSingBoxUsesMatchingOfficialPlatformAsset(t *testing.T) {
	cases := []struct {
		name        string
		goos        string
		goarch      string
		assetName   string
		binaryName  string
		archiveData func(t *testing.T, content []byte) []byte
	}{
		{"windows-amd64", "windows", "amd64", "sing-box-1.2.3-windows-amd64.zip", "sing-box.exe", makeSingBoxZip},
		{"windows-arm64", "windows", "arm64", "sing-box-1.2.3-windows-arm64.zip", "sing-box.exe", makeSingBoxZip},
		{"linux-amd64", "linux", "amd64", "sing-box-1.2.3-linux-amd64.tar.gz", "sing-box", makeSingBoxTarGz},
		{"linux-arm64", "linux", "arm64", "sing-box-1.2.3-linux-arm64.tar.gz", "sing-box", makeSingBoxTarGz},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			binaryContent := []byte("sing-box test binary")
			archiveData := tc.archiveData(t, binaryContent)
			digest := sha256.Sum256(archiveData)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/latest":
					_ = json.NewEncoder(w).Encode(map[string]any{
						"tag_name": "v1.2.3",
						"assets": []map[string]string{{
							"name":                 tc.assetName,
							"browser_download_url": "http://" + r.Host + "/asset",
							"digest":               "sha256:" + hex.EncodeToString(digest[:]),
						}},
					})
				case "/asset":
					_, _ = w.Write(archiveData)
				default:
					http.NotFound(w, r)
				}
			}))
			defer server.Close()

			got, version, err := downloadSingBox(context.Background(), server.Client(), server.URL+"/latest", tc.goos, tc.goarch)
			if err != nil {
				t.Fatal(err)
			}
			if version != "1.2.3" || !bytes.Equal(got, binaryContent) {
				t.Fatalf("version=%q binary=%q", version, got)
			}
		})
	}
}

func TestDownloadSingBoxRejectsDigestMismatch(t *testing.T) {
	archiveData := makeSingBoxZip(t, []byte("binary"))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/latest" {
			_, _ = fmt.Fprintf(w, `{"tag_name":"v1.2.3","assets":[{"name":"sing-box-1.2.3-windows-amd64.zip","browser_download_url":"http://%s/asset","digest":"sha256:%064d"}]}`, r.Host, 0)
			return
		}
		_, _ = w.Write(archiveData)
	}))
	defer server.Close()
	if _, _, err := downloadSingBox(context.Background(), server.Client(), server.URL+"/latest", "windows", "amd64"); err == nil || !strings.Contains(err.Error(), "SHA-256") {
		t.Fatalf("expected checksum failure, got %v", err)
	}
}

func TestSingBoxAssetTargetRejectsUnsupportedPlatform(t *testing.T) {
	if _, _, _, err := singBoxAssetTarget("darwin", "arm64"); err == nil {
		t.Fatal("expected unsupported platform error")
	}
}

func TestCompareSingBoxVersions(t *testing.T) {
	cases := []struct {
		current string
		latest  string
		want    int
	}{
		{"1.10.0", "1.11.0", -1},
		{"1.11.0", "1.11.0", 0},
		{"1.12.0", "1.11.0", 1},
		{"1.11.0-alpha.1", "1.11.0", -1},
	}
	for _, tc := range cases {
		got, err := compareSingBoxVersions(tc.current, tc.latest)
		if err != nil {
			t.Fatalf("compare %s to %s: %v", tc.current, tc.latest, err)
		}
		if got != tc.want {
			t.Errorf("compare %s to %s=%d want=%d", tc.current, tc.latest, got, tc.want)
		}
	}
}

func makeSingBoxZip(t *testing.T, content []byte) []byte {
	t.Helper()
	var buffer bytes.Buffer
	archive := zip.NewWriter(&buffer)
	file, err := archive.Create("sing-box-release/sing-box.exe")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func makeSingBoxTarGz(t *testing.T, content []byte) []byte {
	t.Helper()
	var buffer bytes.Buffer
	compressed := gzip.NewWriter(&buffer)
	archive := tar.NewWriter(compressed)
	if err := archive.WriteHeader(&tar.Header{Name: "sing-box-release/sing-box", Mode: 0o755, Size: int64(len(content))}); err != nil {
		t.Fatal(err)
	}
	if _, err := archive.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	if err := compressed.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}
