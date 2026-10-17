package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCheckAppUpdate(t *testing.T) {
	tag := "v0.4.0"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/releases/latest" {
			http.NotFound(w, r)
			return
		}
		http.Redirect(w, r, "/releases/tag/"+tag, http.StatusFound)
	}))
	defer server.Close()
	latestURL := server.URL + "/releases/latest"

	result, err := checkAppUpdate(context.Background(), http.DefaultTransport, latestURL, "0.3.6")
	if err != nil {
		t.Fatal(err)
	}
	if !result.HasUpdate || result.LatestVersion != "0.4.0" || result.ReleaseURL != server.URL+"/releases/tag/v0.4.0" {
		t.Fatalf("unexpected result: %+v", result)
	}

	tag = "v0.3.6"
	result, err = checkAppUpdate(context.Background(), http.DefaultTransport, latestURL, "0.3.6")
	if err != nil || result.HasUpdate {
		t.Fatalf("same version reported update: %+v %v", result, err)
	}
}

func TestCheckAppUpdateWithoutRelease(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/releases", http.StatusFound)
	}))
	defer server.Close()
	if _, err := checkAppUpdate(context.Background(), http.DefaultTransport, server.URL+"/releases/latest", "0.3.6"); err == nil {
		t.Fatal("expected error when no release is published")
	}
}
