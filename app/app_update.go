package app

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const appLatestReleaseURL = "https://github.com/caichengle666/sbtun/releases/latest"

// AppUpdateDTO describes whether a newer sbtun release is published.
type AppUpdateDTO struct {
	CurrentVersion string `json:"current_version"`
	LatestVersion  string `json:"latest_version"`
	HasUpdate      bool   `json:"has_update"`
	ReleaseURL     string `json:"release_url"`
}

// CheckAppUpdate asks GitHub for the latest sbtun release tag. 直连 15 秒不通就经已有节点再查。
func (a *App) CheckAppUpdate() (AppUpdateDTO, error) {
	ctx, cancel := context.WithTimeout(a.baseContext(), 45*time.Second)
	defer cancel()
	fetcher := &proxiedFetcher{app: a, timeout: 15 * time.Second}
	defer fetcher.close()
	return checkAppUpdate(ctx, fetcher, appLatestReleaseURL, Version())
}

// The releases/latest page redirects to .../releases/tag/vX.Y.Z, which avoids
// the GitHub API rate limit for unauthenticated clients.
func checkAppUpdate(ctx context.Context, transport http.RoundTripper, latestURL, current string) (AppUpdateDTO, error) {
	result := AppUpdateDTO{CurrentVersion: current}
	request, err := http.NewRequestWithContext(ctx, http.MethodHead, latestURL, nil)
	if err != nil {
		return result, err
	}
	request.Header.Set("User-Agent", "sbtun-update-check")
	client := &http.Client{
		Transport:     transport,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	response, err := client.Do(request)
	if err != nil {
		return result, fmt.Errorf("检查 sbtun 新版本失败: %w", err)
	}
	response.Body.Close()
	location := response.Header.Get("Location")
	if response.StatusCode < 300 || response.StatusCode >= 400 || location == "" {
		return result, fmt.Errorf("检查 sbtun 新版本失败: HTTP %d", response.StatusCode)
	}
	index := strings.LastIndex(location, "/tag/")
	if index < 0 {
		return result, errors.New("检查 sbtun 新版本失败: 尚未发布正式版本")
	}
	tag := location[index+len("/tag/"):]
	latest := strings.TrimPrefix(tag, "v")
	order, err := compareSingBoxVersions(current, latest)
	if err != nil {
		return result, fmt.Errorf("检查 sbtun 新版本失败: %w", err)
	}
	result.LatestVersion = latest
	result.HasUpdate = order < 0
	result.ReleaseURL = strings.TrimSuffix(latestURL, "/latest") + "/tag/" + tag
	return result, nil
}
