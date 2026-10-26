package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

var githubReleaseAssetPattern = regexp.MustCompile(`(?s)<a\s+href="([^"]*/releases/download/[^"]+)"[^>]*>\s*<span class="text-bold">([^<]+)</span>`)
var githubReleaseDigestPattern = regexp.MustCompile(`(?s)<clipboard-copy[^>]*aria-label="Copy to clipboard digest for ([^"]+)"[^>]*value="(sha256:[0-9a-fA-F]{64})"`)

type githubRelease struct {
	TagName string        `json:"tag_name"`
	Assets  []githubAsset `json:"assets"`
}

type githubAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Digest             string `json:"digest"`
}

// latestGitHubRelease reads the latest release from the GitHub API and falls
// back to the public release pages when the anonymous API quota is used up.
// label names the software in error messages (sing-box / sbtun).
func latestGitHubRelease(ctx context.Context, client *http.Client, releaseURL, label string) (githubRelease, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, releaseURL, nil)
	if err != nil {
		return githubRelease{}, err
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("User-Agent", "sbtun-updater")
	response, err := client.Do(request)
	if err != nil {
		return githubRelease{}, fmt.Errorf("查询 %s 最新版本失败: %w", label, err)
	}
	if response.StatusCode == http.StatusForbidden || response.StatusCode == http.StatusTooManyRequests {
		response.Body.Close()
		return latestGitHubReleaseFromHTML(ctx, client, releaseURL, label)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return githubRelease{}, fmt.Errorf("查询 %s 最新版本失败: HTTP %d", label, response.StatusCode)
	}
	var release githubRelease
	if err := json.NewDecoder(io.LimitReader(response.Body, 4<<20)).Decode(&release); err != nil {
		return githubRelease{}, fmt.Errorf("解析 %s 版本信息失败: %w", label, err)
	}
	if !singBoxVersionPattern.MatchString(strings.TrimPrefix(strings.TrimSpace(release.TagName), "v")) {
		return githubRelease{}, fmt.Errorf("返回了无效的 %s 版本号: %q", label, release.TagName)
	}
	return release, nil
}

func latestGitHubReleaseFromHTML(ctx context.Context, client *http.Client, releaseURL, label string) (githubRelease, error) {
	releasesURL, err := githubReleasesPageURL(releaseURL)
	if err != nil {
		return githubRelease{}, err
	}
	page, err := getGitHubHTML(ctx, client, releasesURL)
	if err != nil {
		return githubRelease{}, fmt.Errorf("查询 %s 最新版本失败，备用页面也不可用: %w", label, err)
	}
	defer page.Body.Close()
	finalURL := page.Request.URL
	tag, err := githubReleaseTag(finalURL.Path)
	if err != nil {
		return githubRelease{}, err
	}
	if !singBoxVersionPattern.MatchString(strings.TrimPrefix(tag, "v")) {
		return githubRelease{}, fmt.Errorf("发布页面返回了无效的 %s 版本号: %q", label, tag)
	}
	assetsURL := *finalURL
	assetsURL.Path = strings.Replace(finalURL.Path, "/tag/"+tag, "/expanded_assets/"+tag, 1)
	assetsURL.RawQuery = ""
	assetsURL.Fragment = ""
	assetsPage, err := getGitHubHTML(ctx, client, assetsURL.String())
	if err != nil {
		return githubRelease{}, fmt.Errorf("查询 %s 下载文件失败: %w", label, err)
	}
	defer assetsPage.Body.Close()
	assetsHTML, err := io.ReadAll(io.LimitReader(assetsPage.Body, 4<<20))
	if err != nil {
		return githubRelease{}, fmt.Errorf("读取 %s 下载文件列表失败: %w", label, err)
	}
	assets := parseGitHubReleaseAssets(assetsPage.Request.URL, assetsHTML)
	if len(assets) == 0 {
		return githubRelease{}, fmt.Errorf("%s 发布页面没有可用的下载文件", label)
	}
	return githubRelease{TagName: tag, Assets: assets}, nil
}

func getGitHubHTML(ctx context.Context, client *http.Client, pageURL string) (*http.Response, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, pageURL, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "text/html,application/xhtml+xml")
	request.Header.Set("User-Agent", "sbtun-updater")
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	if response.StatusCode != http.StatusOK {
		response.Body.Close()
		return nil, fmt.Errorf("HTTP %d", response.StatusCode)
	}
	return response, nil
}

// githubReleasesPageURL turns api.github.com/repos/<owner>/<repo>/releases/latest
// into github.com/<owner>/<repo>/releases/latest.
func githubReleasesPageURL(releaseURL string) (string, error) {
	parsed, err := url.Parse(releaseURL)
	if err != nil {
		return "", err
	}
	if !strings.HasSuffix(parsed.Path, "/releases/latest") {
		return "", fmt.Errorf("无法从 %q 生成发布页面地址", releaseURL)
	}
	if rest, ok := strings.CutPrefix(parsed.Path, "/repos/"); ok {
		if strings.EqualFold(parsed.Host, "api.github.com") {
			parsed.Host = "github.com"
		}
		parsed.Path = "/" + rest
	}
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String(), nil
}

func githubReleaseTag(path string) (string, error) {
	const marker = "/releases/tag/"
	index := strings.Index(path, marker)
	if index < 0 {
		return "", fmt.Errorf("发布页面地址无效: %q", path)
	}
	tag, err := url.PathUnescape(strings.Trim(path[index+len(marker):], "/"))
	if err != nil || tag == "" {
		return "", fmt.Errorf("发布页面缺少版本号: %q", path)
	}
	return tag, nil
}

func parseGitHubReleaseAssets(baseURL *url.URL, page []byte) []githubAsset {
	digests := make(map[string]string)
	for _, match := range githubReleaseDigestPattern.FindAllSubmatch(page, -1) {
		if len(match) == 3 {
			digests[string(match[1])] = string(match[2])
		}
	}
	assets := make([]githubAsset, 0, len(digests))
	for _, match := range githubReleaseAssetPattern.FindAllSubmatch(page, -1) {
		if len(match) != 3 {
			continue
		}
		name := string(match[2])
		digest, ok := digests[name]
		if !ok {
			continue
		}
		downloadURL, err := url.Parse(string(match[1]))
		if err != nil {
			continue
		}
		assets = append(assets, githubAsset{
			Name:               name,
			BrowserDownloadURL: baseURL.ResolveReference(downloadURL).String(),
			Digest:             digest,
		})
	}
	return assets
}

// findReleaseAsset returns the named asset; it must carry a SHA-256 digest.
func findReleaseAsset(release githubRelease, name string) (githubAsset, error) {
	for _, asset := range release.Assets {
		if asset.Name != name || asset.BrowserDownloadURL == "" {
			continue
		}
		if !strings.HasPrefix(asset.Digest, "sha256:") {
			return asset, fmt.Errorf("%s 未提供 SHA-256 摘要，已取消更新", name)
		}
		return asset, nil
	}
	return githubAsset{}, errors.New("最新版本没有 " + name)
}
