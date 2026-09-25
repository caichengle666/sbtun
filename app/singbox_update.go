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
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/caichengle666/sbtun/config"
)

const singBoxLatestReleaseURL = "https://api.github.com/repos/SagerNet/sing-box/releases/latest"
const maxSingBoxArchiveSize = 128 << 20
const maxSingBoxBinarySize = 128 << 20

var singBoxVersionPattern = regexp.MustCompile(`(?m)(\d+\.\d+\.\d+(?:[-+][0-9A-Za-z.-]+)?)`)
var singBoxReleaseAssetPattern = regexp.MustCompile(`(?s)<a\s+href="([^"]*/releases/download/[^"]+)"[^>]*>\s*<span class="text-bold">([^<]+)</span>`)
var singBoxReleaseDigestPattern = regexp.MustCompile(`(?s)<clipboard-copy[^>]*aria-label="Copy to clipboard digest for ([^"]+)"[^>]*value="(sha256:[0-9a-fA-F]{64})"`)

type SingBoxUpdateResult struct {
	CurrentVersion string `json:"current_version"`
	LatestVersion  string `json:"latest_version"`
	Updated        bool   `json:"updated"`
	Message        string `json:"message"`
}

type singBoxRelease struct {
	TagName string         `json:"tag_name"`
	Assets  []singBoxAsset `json:"assets"`
}

type singBoxAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Digest             string `json:"digest"`
}

func (a *App) SingBoxVersion() (string, error) {
	if a.binary == "" {
		return "", errors.New("sing-box 路径未初始化")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return readSingBoxVersion(ctx, a.binary)
}

func (a *App) UpdateSingBox() (SingBoxUpdateResult, error) {
	a.operationMu.Lock()
	defer a.operationMu.Unlock()

	if a.binary == "" {
		return SingBoxUpdateResult{}, errors.New("sing-box 路径未初始化")
	}
	installed := fileExists(a.binary)
	if runtime.GOOS != "windows" && runtime.GOOS != "linux" {
		return SingBoxUpdateResult{}, fmt.Errorf("暂不支持在 %s 上更新 sing-box", runtime.GOOS)
	}
	if runtime.GOARCH != "amd64" && runtime.GOARCH != "arm64" {
		return SingBoxUpdateResult{}, fmt.Errorf("暂不支持 %s 架构的 sing-box 更新", runtime.GOARCH)
	}

	ctx := a.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	var err error
	current := ""
	if installed {
		current, err = readSingBoxVersion(ctx, a.binary)
		if err != nil {
			return SingBoxUpdateResult{}, fmt.Errorf("读取当前 sing-box 版本失败: %w", err)
		}
	}
	client := &http.Client{Timeout: 5 * time.Minute}
	release, err := latestSingBoxRelease(ctx, client, singBoxLatestReleaseURL)
	if err != nil {
		return SingBoxUpdateResult{}, err
	}
	latest := strings.TrimPrefix(strings.TrimSpace(release.TagName), "v")
	result := SingBoxUpdateResult{CurrentVersion: current, LatestVersion: latest}
	if installed {
		comparison, err := compareSingBoxVersions(current, latest)
		if err != nil {
			return SingBoxUpdateResult{}, err
		}
		if comparison == 0 {
			result.Message = "当前已是最新稳定版"
			return result, nil
		}
		if comparison > 0 {
			return result, fmt.Errorf("当前版本 %s 高于官方最新稳定版 %s，已跳过降级", current, latest)
		}
	}
	candidate, err := downloadSingBoxAsset(ctx, client, release, runtime.GOOS, runtime.GOARCH)
	if err != nil {
		return result, err
	}

	tempPath, err := writeSingBoxCandidate(filepath.Dir(a.binary), filepath.Base(a.binary), candidate)
	if err != nil {
		return result, fmt.Errorf("准备新内核失败: %w", err)
	}
	defer os.Remove(tempPath)
	if err := validateSingBoxCandidate(ctx, tempPath, filepath.Join(a.workDir, "runtime.json")); err != nil {
		return result, err
	}

	wasRunning := a.runtime != nil && a.runtime.State != nil && a.runtime.State.IsRunning()
	var cfg config.Config
	if wasRunning {
		if a.manager == nil {
			return result, errors.New("配置管理器未初始化，无法安全重启代理")
		}
		cfg, err = a.manager.Load()
		if err != nil {
			return result, fmt.Errorf("读取运行配置失败: %w", err)
		}
		a.stopSelectorSync()
		if err := a.runtime.Stop(); err != nil {
			a.startSelectorSync(cfg.CurrentNodeID)
			return result, fmt.Errorf("停止 sing-box 以应用更新失败: %w", err)
		}
	}

	backupPath := ""
	if installed {
		backupPath, err = backupSingBox(a.binary)
		if err != nil {
			if wasRunning {
				if startErr := a.startWithConfigLocked(cfg); startErr != nil {
					return result, fmt.Errorf("备份旧内核失败: %v；恢复代理也失败: %w", err, startErr)
				}
			}
			return result, fmt.Errorf("备份旧内核失败: %w", err)
		}
	}
	if err := os.Rename(tempPath, a.binary); err != nil {
		if backupPath != "" {
			os.Remove(backupPath)
		}
		if wasRunning {
			if startErr := a.startWithConfigLocked(cfg); startErr != nil {
				return result, fmt.Errorf("安装新内核失败: %v；恢复代理也失败: %w", err, startErr)
			}
		}
		return result, fmt.Errorf("安装新内核失败: %w", err)
	}

	if wasRunning {
		if err := a.startWithConfigLocked(cfg); err != nil {
			if backupPath != "" {
				rollbackErr := os.Rename(backupPath, a.binary)
				if rollbackErr == nil {
					rollbackErr = a.startWithConfigLocked(cfg)
				}
				if rollbackErr != nil {
					return result, fmt.Errorf("新内核启动失败: %v；恢复旧内核或代理失败: %w", err, rollbackErr)
				}
				return result, fmt.Errorf("新内核启动失败，已恢复旧内核和代理: %w", err)
			}
			return result, fmt.Errorf("新内核启动失败: %w", err)
		}
	}
	if backupPath != "" {
		_ = os.Remove(backupPath)
	}
	result.CurrentVersion = latest
	result.Updated = true
	result.Message = "sing-box 已更新并通过配置校验"
	return result, nil
}

func downloadSingBox(ctx context.Context, client *http.Client, releaseURL, goos, goarch string) ([]byte, string, error) {
	release, err := latestSingBoxRelease(ctx, client, releaseURL)
	if err != nil {
		return nil, "", err
	}
	version := strings.TrimPrefix(strings.TrimSpace(release.TagName), "v")
	binary, err := downloadSingBoxAsset(ctx, client, release, goos, goarch)
	return binary, version, err
}

func latestSingBoxRelease(ctx context.Context, client *http.Client, releaseURL string) (singBoxRelease, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, releaseURL, nil)
	if err != nil {
		return singBoxRelease{}, err
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("User-Agent", "sbtun-sing-box-updater")
	response, err := client.Do(request)
	if err != nil {
		return singBoxRelease{}, fmt.Errorf("查询 sing-box 官方版本失败: %w", err)
	}
	if response.StatusCode == http.StatusForbidden || response.StatusCode == http.StatusTooManyRequests {
		response.Body.Close()
		return latestSingBoxReleaseFromHTML(ctx, client, releaseURL)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return singBoxRelease{}, fmt.Errorf("查询 sing-box 官方版本失败: HTTP %d", response.StatusCode)
	}
	var release singBoxRelease
	if err := json.NewDecoder(io.LimitReader(response.Body, 4<<20)).Decode(&release); err != nil {
		return singBoxRelease{}, fmt.Errorf("解析 sing-box 官方版本信息失败: %w", err)
	}
	version := strings.TrimPrefix(strings.TrimSpace(release.TagName), "v")
	if !singBoxVersionPattern.MatchString(version) {
		return singBoxRelease{}, fmt.Errorf("官方返回了无效的 sing-box 版本号: %q", release.TagName)
	}
	return release, nil
}

func latestSingBoxReleaseFromHTML(ctx context.Context, client *http.Client, releaseURL string) (singBoxRelease, error) {
	releasesURL, err := singBoxReleasesPageURL(releaseURL)
	if err != nil {
		return singBoxRelease{}, err
	}
	page, err := getSingBoxHTML(ctx, client, releasesURL)
	if err != nil {
		return singBoxRelease{}, fmt.Errorf("查询 sing-box 官方版本失败，备用页面也不可用: %w", err)
	}
	defer page.Body.Close()
	finalURL := page.Request.URL
	tag, err := singBoxReleaseTag(finalURL.Path)
	if err != nil {
		return singBoxRelease{}, err
	}
	version := strings.TrimPrefix(tag, "v")
	if !singBoxVersionPattern.MatchString(version) {
		return singBoxRelease{}, fmt.Errorf("官方页面返回了无效的 sing-box 版本号: %q", tag)
	}
	assetsURL := *finalURL
	assetsURL.Path = strings.Replace(finalURL.Path, "/tag/"+tag, "/expanded_assets/"+tag, 1)
	assetsURL.RawQuery = ""
	assetsURL.Fragment = ""
	assetsPage, err := getSingBoxHTML(ctx, client, assetsURL.String())
	if err != nil {
		return singBoxRelease{}, fmt.Errorf("查询 sing-box 官方资源失败: %w", err)
	}
	defer assetsPage.Body.Close()
	assetsHTML, err := io.ReadAll(io.LimitReader(assetsPage.Body, 4<<20))
	if err != nil {
		return singBoxRelease{}, fmt.Errorf("读取 sing-box 官方资源失败: %w", err)
	}
	assets := parseSingBoxReleaseAssets(assetsPage.Request.URL, assetsHTML)
	if len(assets) == 0 {
		return singBoxRelease{}, errors.New("sing-box 官方发布页面没有可用的下载资源")
	}
	return singBoxRelease{TagName: tag, Assets: assets}, nil
}

func getSingBoxHTML(ctx context.Context, client *http.Client, pageURL string) (*http.Response, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, pageURL, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "text/html,application/xhtml+xml")
	request.Header.Set("User-Agent", "sbtun-sing-box-updater")
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

func singBoxReleasesPageURL(releaseURL string) (string, error) {
	parsed, err := url.Parse(releaseURL)
	if err != nil {
		return "", err
	}
	const apiPath = "/repos/SagerNet/sing-box/releases/latest"
	if parsed.Path == apiPath {
		if strings.EqualFold(parsed.Host, "api.github.com") {
			parsed.Host = "github.com"
		}
		parsed.Path = "/SagerNet/sing-box/releases/latest"
		parsed.RawQuery = ""
		parsed.Fragment = ""
		return parsed.String(), nil
	}
	if strings.HasSuffix(parsed.Path, "/releases/latest") {
		return parsed.String(), nil
	}
	return "", fmt.Errorf("无法从 %q 生成 sing-box 官方发布页面地址", releaseURL)
}

func singBoxReleaseTag(path string) (string, error) {
	const marker = "/releases/tag/"
	index := strings.Index(path, marker)
	if index < 0 {
		return "", fmt.Errorf("sing-box 官方发布页面地址无效: %q", path)
	}
	tag, err := url.PathUnescape(strings.Trim(path[index+len(marker):], "/"))
	if err != nil || tag == "" {
		return "", fmt.Errorf("sing-box 官方发布页面缺少版本号: %q", path)
	}
	return tag, nil
}

func parseSingBoxReleaseAssets(baseURL *url.URL, page []byte) []singBoxAsset {
	digests := make(map[string]string)
	for _, match := range singBoxReleaseDigestPattern.FindAllSubmatch(page, -1) {
		if len(match) == 3 {
			digests[string(match[1])] = string(match[2])
		}
	}
	assets := make([]singBoxAsset, 0, len(digests))
	for _, match := range singBoxReleaseAssetPattern.FindAllSubmatch(page, -1) {
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
		assets = append(assets, singBoxAsset{
			Name:               name,
			BrowserDownloadURL: baseURL.ResolveReference(downloadURL).String(),
			Digest:             digest,
		})
	}
	return assets
}

func downloadSingBoxAsset(ctx context.Context, client *http.Client, release singBoxRelease, goos, goarch string) ([]byte, error) {
	assetSuffix, binaryName, archiveType, err := singBoxAssetTarget(goos, goarch)
	if err != nil {
		return nil, err
	}
	version := strings.TrimPrefix(strings.TrimSpace(release.TagName), "v")
	if !singBoxVersionPattern.MatchString(version) {
		return nil, fmt.Errorf("官方返回了无效的 sing-box 版本号: %q", release.TagName)
	}
	var asset *singBoxAsset
	for i := range release.Assets {
		if strings.HasSuffix(release.Assets[i].Name, assetSuffix) {
			asset = &release.Assets[i]
			break
		}
	}
	if asset == nil || asset.BrowserDownloadURL == "" {
		return nil, fmt.Errorf("官方稳定版没有 %s/%s 对应的下载文件", goos, goarch)
	}
	if !strings.HasPrefix(asset.Digest, "sha256:") {
		return nil, errors.New("官方 sing-box 下载文件未提供 SHA-256 摘要，已取消更新")
	}

	archiveRequest, err := http.NewRequestWithContext(ctx, http.MethodGet, asset.BrowserDownloadURL, nil)
	if err != nil {
		return nil, err
	}
	archiveRequest.Header.Set("User-Agent", "sbtun-sing-box-updater")
	archiveResponse, err := client.Do(archiveRequest)
	if err != nil {
		return nil, fmt.Errorf("下载 sing-box %s 失败: %w", version, err)
	}
	defer archiveResponse.Body.Close()
	if archiveResponse.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("下载 sing-box 失败: HTTP %d", archiveResponse.StatusCode)
	}
	archiveData, err := io.ReadAll(io.LimitReader(archiveResponse.Body, maxSingBoxArchiveSize+1))
	if err != nil {
		return nil, fmt.Errorf("读取 sing-box 下载文件失败: %w", err)
	}
	if len(archiveData) > maxSingBoxArchiveSize {
		return nil, errors.New("sing-box 下载文件超过大小限制")
	}
	digest := sha256.Sum256(archiveData)
	if !strings.EqualFold(hex.EncodeToString(digest[:]), strings.TrimPrefix(asset.Digest, "sha256:")) {
		return nil, errors.New("sing-box SHA-256 校验失败，已取消更新")
	}
	var binary []byte
	if archiveType == "zip" {
		binary, err = singBoxBinaryFromZip(archiveData, binaryName)
	} else {
		binary, err = singBoxBinaryFromTarGz(archiveData, binaryName)
	}
	if err != nil {
		return nil, fmt.Errorf("解包 sing-box 失败: %w", err)
	}
	return binary, nil
}

func singBoxAssetTarget(goos, goarch string) (suffix, binaryName, archiveType string, err error) {
	switch goos {
	case "windows":
		return "windows-" + goarch + ".zip", "sing-box.exe", "zip", nil
	case "linux":
		return "linux-" + goarch + ".tar.gz", "sing-box", "tar.gz", nil
	default:
		return "", "", "", fmt.Errorf("暂不支持 %s 平台", goos)
	}
}

func singBoxBinaryFromZip(data []byte, binaryName string) ([]byte, error) {
	archive, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, err
	}
	for _, file := range archive.File {
		if filepath.Base(filepath.ToSlash(file.Name)) != binaryName {
			continue
		}
		if file.UncompressedSize64 > maxSingBoxBinarySize {
			return nil, errors.New("内核文件超过大小限制")
		}
		reader, err := file.Open()
		if err != nil {
			return nil, err
		}
		binary, readErr := io.ReadAll(io.LimitReader(reader, maxSingBoxBinarySize+1))
		closeErr := reader.Close()
		if readErr != nil {
			return nil, readErr
		}
		if closeErr != nil {
			return nil, closeErr
		}
		if len(binary) == 0 || len(binary) > maxSingBoxBinarySize {
			return nil, errors.New("内核文件大小无效")
		}
		return binary, nil
	}
	return nil, fmt.Errorf("压缩包中未找到 %s", binaryName)
}

func singBoxBinaryFromTarGz(data []byte, binaryName string) ([]byte, error) {
	compressed, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer compressed.Close()
	archive := tar.NewReader(compressed)
	for {
		header, err := archive.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		if filepath.Base(filepath.ToSlash(header.Name)) != binaryName || !header.FileInfo().Mode().IsRegular() {
			continue
		}
		if header.Size <= 0 || header.Size > maxSingBoxBinarySize {
			return nil, errors.New("内核文件大小无效")
		}
		return io.ReadAll(io.LimitReader(archive, maxSingBoxBinarySize+1))
	}
	return nil, fmt.Errorf("压缩包中未找到 %s", binaryName)
}

func readSingBoxVersion(ctx context.Context, binary string) (string, error) {
	command := exec.CommandContext(ctx, binary, "version")
	configureProcess(command)
	output, err := command.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%w: %s", err, strings.TrimSpace(string(output)))
	}
	match := singBoxVersionPattern.FindStringSubmatch(string(output))
	if len(match) < 2 {
		return "", fmt.Errorf("无法识别版本输出: %s", strings.TrimSpace(string(output)))
	}
	return match[1], nil
}

func compareSingBoxVersions(current, latest string) (int, error) {
	currentParts, err := parseSingBoxVersion(current)
	if err != nil {
		return 0, err
	}
	latestParts, err := parseSingBoxVersion(latest)
	if err != nil {
		return 0, err
	}
	for i := range currentParts {
		if currentParts[i] < latestParts[i] {
			return -1, nil
		}
		if currentParts[i] > latestParts[i] {
			return 1, nil
		}
	}
	if strings.Contains(current, "-") && !strings.Contains(latest, "-") {
		return -1, nil
	}
	return 0, nil
}

func parseSingBoxVersion(version string) ([3]int, error) {
	var parts [3]int
	match := singBoxVersionPattern.FindStringSubmatch(version)
	if len(match) < 2 {
		return parts, fmt.Errorf("无效的 sing-box 版本号: %q", version)
	}
	core := strings.SplitN(match[1], "-", 2)[0]
	core = strings.SplitN(core, "+", 2)[0]
	for i, part := range strings.Split(core, ".") {
		value, err := strconv.Atoi(part)
		if err != nil {
			return parts, fmt.Errorf("无效的 sing-box 版本号: %q", version)
		}
		parts[i] = value
	}
	return parts, nil
}

func writeSingBoxCandidate(dir, binaryName string, data []byte) (string, error) {
	file, err := os.CreateTemp(dir, "."+binaryName+"-update-*")
	if err != nil {
		return "", err
	}
	path := file.Name()
	if _, err := file.Write(data); err != nil {
		file.Close()
		os.Remove(path)
		return "", err
	}
	mode := os.FileMode(0o755)
	if err := file.Chmod(mode); err != nil {
		file.Close()
		os.Remove(path)
		return "", err
	}
	if err := file.Sync(); err != nil {
		file.Close()
		os.Remove(path)
		return "", err
	}
	if err := file.Close(); err != nil {
		os.Remove(path)
		return "", err
	}
	return path, nil
}

func validateSingBoxCandidate(ctx context.Context, binary, configPath string) error {
	if _, err := readSingBoxVersion(ctx, binary); err != nil {
		return fmt.Errorf("新内核无法运行，已取消更新: %w", err)
	}
	if _, err := os.Stat(configPath); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return fmt.Errorf("读取运行配置失败: %w", err)
	}
	command := exec.CommandContext(ctx, binary, "check", "-c", configPath)
	configureProcess(command)
	if output, err := command.CombinedOutput(); err != nil {
		return fmt.Errorf("新内核不接受当前配置，已取消更新: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func backupSingBox(binary string) (string, error) {
	backup, err := os.CreateTemp(filepath.Dir(binary), ".sing-box-backup-*")
	if err != nil {
		return "", err
	}
	path := backup.Name()
	if err := backup.Close(); err != nil {
		os.Remove(path)
		return "", err
	}
	source, err := os.Open(binary)
	if err != nil {
		os.Remove(path)
		return "", err
	}
	defer source.Close()
	info, err := source.Stat()
	if err != nil {
		os.Remove(path)
		return "", err
	}
	destination, err := os.OpenFile(path, os.O_WRONLY|os.O_TRUNC, 0o700)
	if err != nil {
		os.Remove(path)
		return "", err
	}
	_, copyErr := io.Copy(destination, source)
	if copyErr == nil {
		copyErr = destination.Chmod(info.Mode().Perm())
	}
	closeErr := destination.Close()
	if copyErr != nil {
		os.Remove(path)
		return "", copyErr
	}
	if closeErr != nil {
		os.Remove(path)
		return "", closeErr
	}
	return path, nil
}
