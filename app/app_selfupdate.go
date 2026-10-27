package app

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const appLatestReleaseAPI = "https://api.github.com/repos/caichengle666/sbtun/releases/latest"
const maxAppArchiveSize = 512 << 20

// 重启后的新进程靠这两个环境变量知道：等旧进程释放单实例锁；旧进程退出前代理在运行，需要接着连上。
const (
	envUpdateRestart = "SBTUN_UPDATE_RESTART"
	envUpdateResume  = "SBTUN_UPDATE_RESUME"
)

// launchExecutable is resolved at start-up: after an update the running file is
// renamed (or its bundle replaced), and the restart must start the new one at the original path.
var launchExecutable = resolveLaunchExecutable()

// pendingVersion is the version installed by UpdateApp that has not been restarted into yet.
var pendingVersion string

func resolveLaunchExecutable() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		return resolved
	}
	return exe
}

// AppSelfUpdateResult reports what UpdateApp did.
type AppSelfUpdateResult struct {
	CurrentVersion string `json:"current_version"`
	LatestVersion  string `json:"latest_version"`
	Updated        bool   `json:"updated"`
	Message        string `json:"message"`
}

// UpdateApp downloads the latest sbtun release for this platform, verifies its
// SHA-256 and replaces the program files in place. sing-box, rules and user data
// are left alone. The new version runs after RestartApp (GUI) or the next start (CLI).
func (a *App) UpdateApp() (AppSelfUpdateResult, error) {
	result := AppSelfUpdateResult{CurrentVersion: Version()}
	if runtime.GOOS == "android" {
		return result, errors.New("安卓版请下载新的 APK 安装")
	}
	if launchExecutable == "" {
		return result, errors.New("无法确定 sbtun 程序所在位置")
	}
	assetName, err := appAssetName(runtime.GOOS, runtime.GOARCH)
	if err != nil {
		return result, err
	}
	ctx := a.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Minute)
	defer cancel()
	client := &http.Client{Timeout: 15 * time.Minute}
	release, err := latestGitHubRelease(ctx, client, appLatestReleaseAPI, "sbtun")
	if err != nil {
		return result, err
	}
	latest := strings.TrimPrefix(strings.TrimSpace(release.TagName), "v")
	result.LatestVersion = latest
	order, err := compareSingBoxVersions(result.CurrentVersion, latest)
	if err != nil {
		return result, err
	}
	if order >= 0 {
		result.Message = "已是最新版本"
		return result, nil
	}
	result.Message = "已更新到 v" + latest
	if pendingVersion == latest {
		// 已经装好、只是还没重启，不再重复下载。
		result.Updated = true
		return result, nil
	}
	asset, err := findReleaseAsset(release, assetName)
	if err != nil {
		return result, err
	}
	installDir := filepath.Dir(launchExecutable)
	if runtime.GOOS == "darwin" {
		installDir, err = darwinBundlePath(launchExecutable)
		if err != nil {
			return result, err
		}
		installDir = filepath.Dir(installDir)
	}
	archive, err := downloadAppArchive(ctx, client, asset, installDir)
	if err != nil {
		return result, err
	}
	defer os.Remove(archive)
	if runtime.GOOS == "darwin" {
		err = installDarwinBundle(archive, launchExecutable)
	} else {
		err = installPortable(archive, filepath.Dir(launchExecutable), runtime.GOOS)
	}
	if err != nil {
		return result, err
	}
	pendingVersion = latest
	result.Updated = true
	return result, nil
}

// RestartApp starts the freshly installed GUI and quits this one. The new
// process waits for the single-instance lock and reconnects if the proxy was on.
func (a *App) RestartApp() error {
	if runtime.GOOS == "android" || launchExecutable == "" {
		return errors.New("当前平台不支持自动重启")
	}
	command := exec.Command(launchExecutable, os.Args[1:]...)
	command.Env = append(os.Environ(), envUpdateRestart+"=1")
	if a.runtime != nil && a.runtime.State != nil && a.runtime.State.IsRunning() {
		command.Env = append(command.Env, envUpdateResume+"=1")
	}
	detachProcess(command)
	if err := command.Start(); err != nil {
		return fmt.Errorf("启动新版本失败: %w", err)
	}
	_ = command.Process.Release()
	// 让前端先收到返回值再退出。
	go func() {
		time.Sleep(300 * time.Millisecond)
		a.QuitApp()
	}()
	return nil
}

// AcquireSingleInstanceAfterUpdate is AcquireSingleInstance, except that a
// process started by RestartApp waits up to 30 seconds for the old one to exit.
func AcquireSingleInstanceAfterUpdate() (func(), error) {
	restarting := os.Getenv(envUpdateRestart) == "1"
	os.Unsetenv(envUpdateRestart)
	release, err := AcquireSingleInstance()
	for i := 0; err != nil && restarting && i < 60; i++ {
		time.Sleep(500 * time.Millisecond)
		release, err = AcquireSingleInstance()
	}
	return release, err
}

// resumeAfterUpdate reconnects when the previous process was running the proxy.
func (a *App) resumeAfterUpdate() {
	if os.Getenv(envUpdateResume) != "1" {
		return
	}
	os.Unsetenv(envUpdateResume)
	go func() { _ = a.Start() }()
}

func appAssetName(goos, goarch string) (string, error) {
	if goarch != "amd64" && goarch != "arm64" {
		return "", fmt.Errorf("暂不支持 %s 架构的自动更新", goarch)
	}
	switch goos {
	case "windows":
		return "sbtun-windows-" + goarch + ".zip", nil
	case "linux":
		return "sbtun-linux-" + goarch + ".tar.gz", nil
	case "darwin":
		return "sbtun-macos-" + goarch + ".zip", nil
	}
	return "", fmt.Errorf("暂不支持 %s 平台的自动更新", goos)
}

// downloadAppArchive streams the asset into dir (same volume as the install,
// so later renames stay atomic) and checks its SHA-256.
func downloadAppArchive(ctx context.Context, client *http.Client, asset githubAsset, dir string) (string, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, asset.BrowserDownloadURL, nil)
	if err != nil {
		return "", err
	}
	request.Header.Set("User-Agent", "sbtun-updater")
	response, err := client.Do(request)
	if err != nil {
		return "", fmt.Errorf("下载 %s 失败: %w", asset.Name, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("下载 %s 失败: HTTP %d", asset.Name, response.StatusCode)
	}
	file, err := os.CreateTemp(dir, ".sbtun-update-*")
	if err != nil {
		return "", fmt.Errorf("程序目录不可写，无法更新: %w", err)
	}
	hash := sha256.New()
	size, copyErr := io.Copy(io.MultiWriter(file, hash), io.LimitReader(response.Body, maxAppArchiveSize+1))
	closeErr := file.Close()
	fail := func(err error) (string, error) {
		os.Remove(file.Name())
		return "", err
	}
	switch {
	case copyErr != nil:
		return fail(fmt.Errorf("下载 %s 失败: %w", asset.Name, copyErr))
	case closeErr != nil:
		return fail(closeErr)
	case size > maxAppArchiveSize:
		return fail(errors.New("安装包超过大小限制"))
	case !strings.EqualFold(hex.EncodeToString(hash.Sum(nil)), strings.TrimPrefix(asset.Digest, "sha256:")):
		return fail(errors.New("安装包 SHA-256 校验失败，已取消更新"))
	}
	return file.Name(), nil
}

// portableFiles are the program files replaced on Windows/Linux; sing-box,
// wintun, rules and the bundled WebKit libraries stay as they are.
func portableFiles(goos string) (required, optional []string) {
	if goos == "windows" {
		return []string{"sbtun.exe", "sbtun-cli.exe"}, []string{"VERSION"}
	}
	return []string{"sbtun", "sbtun-cli"}, []string{"VERSION", "start-sbtun.sh"}
}

func installPortable(archive, dir, goos string) error {
	required, optional := portableFiles(goos)
	wanted := map[string]bool{}
	for _, name := range append(append([]string{}, required...), optional...) {
		wanted[name] = true
	}
	staged := map[string]string{}
	defer func() {
		for _, path := range staged {
			os.Remove(path)
		}
	}()
	stage := func(name string, mode os.FileMode, content io.Reader) error {
		if !wanted[name] || staged[name] != "" {
			return nil
		}
		target := filepath.Join(dir, name+".new")
		if err := writeFileFrom(target, mode, content); err != nil {
			return fmt.Errorf("写入 %s 失败: %w", name, err)
		}
		staged[name] = target
		return nil
	}
	var err error
	if strings.HasSuffix(archive, ".zip") || goos == "windows" {
		err = walkZip(archive, stage)
	} else {
		err = walkTarGz(archive, stage)
	}
	if err != nil {
		return fmt.Errorf("解包安装包失败: %w", err)
	}
	for _, name := range required {
		if staged[name] == "" {
			return fmt.Errorf("安装包中缺少 %s", name)
		}
	}
	type swap struct{ name, old string }
	var done []swap
	rollback := func() {
		for i := len(done) - 1; i >= 0; i-- {
			target := filepath.Join(dir, done[i].name)
			os.Remove(target)
			if done[i].old != "" {
				os.Rename(done[i].old, target)
			}
		}
	}
	for _, name := range append(append([]string{}, required...), optional...) {
		source := staged[name]
		if source == "" {
			continue
		}
		target := filepath.Join(dir, name)
		old := ""
		if info, err := os.Stat(dir); err == nil {
			matchOwner(source, info) // 新增的文件跟随目录的所有者
		}
		if info, err := os.Stat(target); err == nil {
			matchOwner(source, info)
			// Windows 不能覆盖或删除正在运行的 exe，但可以改名；旧文件下次启动时清理。
			old = target + ".old-" + strconv.FormatInt(time.Now().UnixNano(), 36)
			if err := os.Rename(target, old); err != nil {
				rollback()
				return fmt.Errorf("替换 %s 失败: %w", name, err)
			}
		}
		if err := os.Rename(source, target); err != nil {
			if old != "" {
				os.Rename(old, target)
			}
			rollback()
			return fmt.Errorf("替换 %s 失败: %w", name, err)
		}
		delete(staged, name)
		done = append(done, swap{name, old})
	}
	for _, item := range done {
		if item.old != "" {
			os.Remove(item.old) // Windows 上运行中的文件删不掉，留给 cleanupUpdateLeftovers
		}
	}
	return nil
}

// installDarwinBundle unpacks the new sbtun.app beside the running one and swaps the two.
func installDarwinBundle(archive, exe string) error {
	bundle, err := darwinBundlePath(exe)
	if err != nil {
		return err
	}
	parent := filepath.Dir(bundle)
	tempDir, err := os.MkdirTemp(parent, ".sbtun-update-")
	if err != nil {
		return fmt.Errorf("程序目录不可写，请把 sbtun 移到“应用程序”文件夹后再更新: %w", err)
	}
	defer os.RemoveAll(tempDir)
	if err := extractZip(archive, tempDir); err != nil {
		return fmt.Errorf("解包安装包失败: %w", err)
	}
	fresh := filepath.Join(tempDir, "sbtun.app")
	if _, err := os.Stat(filepath.Join(fresh, "Contents", "MacOS", "sbtun")); err != nil {
		return errors.New("安装包中缺少 sbtun.app")
	}
	if info, err := os.Stat(bundle); err == nil {
		_ = filepath.Walk(fresh, func(path string, _ os.FileInfo, _ error) error {
			matchOwner(path, info)
			return nil
		})
	}
	old := bundle + ".old-" + strconv.FormatInt(time.Now().UnixNano(), 36)
	if err := os.Rename(bundle, old); err != nil {
		return fmt.Errorf("替换 sbtun.app 失败: %w", err)
	}
	if err := os.Rename(fresh, bundle); err != nil {
		os.Rename(old, bundle)
		return fmt.Errorf("替换 sbtun.app 失败: %w", err)
	}
	os.RemoveAll(old)
	return nil
}

func darwinBundlePath(exe string) (string, error) {
	bundle := filepath.Dir(filepath.Dir(filepath.Dir(exe)))
	if !strings.HasSuffix(bundle, ".app") || filepath.Base(filepath.Dir(exe)) != "MacOS" {
		return "", errors.New("只有从 sbtun.app 运行时才能自动更新")
	}
	return bundle, nil
}

// cleanupUpdateLeftovers removes renamed old files and half-finished downloads.
// On Windows the previous exe may still be exiting, so it retries for a while.
func cleanupUpdateLeftovers() {
	if launchExecutable == "" || runtime.GOOS == "android" {
		return
	}
	dirs := []string{filepath.Dir(launchExecutable)}
	if bundle, err := darwinBundlePath(launchExecutable); err == nil {
		dirs = []string{filepath.Dir(bundle)}
	}
	for attempt := 0; attempt < 20; attempt++ {
		left := 0
		for _, dir := range dirs {
			for _, pattern := range []string{"*.old-*", ".sbtun-update-*", "*.new"} {
				matches, _ := filepath.Glob(filepath.Join(dir, pattern))
				for _, path := range matches {
					if !isUpdateLeftover(filepath.Base(path)) {
						continue
					}
					if os.RemoveAll(path) != nil {
						left++
					}
				}
			}
		}
		if left == 0 {
			return
		}
		time.Sleep(time.Second)
	}
}

func isUpdateLeftover(name string) bool {
	if strings.HasPrefix(name, ".sbtun-update-") {
		return true
	}
	base := name
	if index := strings.Index(name, ".old-"); index > 0 {
		base = name[:index]
	} else {
		base = strings.TrimSuffix(name, ".new")
	}
	switch base {
	case "sbtun", "sbtun-cli", "sbtun.exe", "sbtun-cli.exe", "VERSION", "start-sbtun.sh", "sbtun.app":
		return true
	}
	return false
}

func writeFileFrom(path string, mode os.FileMode, content io.Reader) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(file, io.LimitReader(content, maxAppArchiveSize))
	closeErr := file.Close()
	if copyErr == nil {
		copyErr = closeErr
	}
	if copyErr == nil {
		copyErr = os.Chmod(path, mode)
	}
	if copyErr != nil {
		os.Remove(path)
	}
	return copyErr
}

// walkZip / walkTarGz hand each regular file at the archive's top level (or
// one directory down, as in sbtun-linux-amd64/sbtun) to stage.
func walkZip(archive string, stage func(string, os.FileMode, io.Reader) error) error {
	reader, err := zip.OpenReader(archive)
	if err != nil {
		return err
	}
	defer reader.Close()
	for _, file := range reader.File {
		name, ok := shallowName(file.Name)
		if !ok || !file.Mode().IsRegular() {
			continue
		}
		content, err := file.Open()
		if err != nil {
			return err
		}
		err = stage(name, fileModeOr(file.Mode(), name), content)
		content.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

func walkTarGz(archive string, stage func(string, os.FileMode, io.Reader) error) error {
	file, err := os.Open(archive)
	if err != nil {
		return err
	}
	defer file.Close()
	compressed, err := gzip.NewReader(file)
	if err != nil {
		return err
	}
	defer compressed.Close()
	reader := tar.NewReader(compressed)
	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		name, ok := shallowName(header.Name)
		if !ok || header.Typeflag != tar.TypeReg {
			continue
		}
		if err := stage(name, fileModeOr(header.FileInfo().Mode(), name), reader); err != nil {
			return err
		}
	}
}

func shallowName(path string) (string, bool) {
	parts := strings.Split(strings.Trim(filepath.ToSlash(path), "/"), "/")
	if len(parts) == 0 || len(parts) > 2 {
		return "", false
	}
	return parts[len(parts)-1], true
}

func fileModeOr(mode os.FileMode, name string) os.FileMode {
	if perm := mode.Perm(); perm&0o400 != 0 {
		return perm
	}
	if name == "VERSION" {
		return 0o644
	}
	return 0o755
}

// extractZip unpacks a whole archive (the macOS .app bundle), keeping modes,
// symlinks and modification times.
func extractZip(archive, dest string) error {
	reader, err := zip.OpenReader(archive)
	if err != nil {
		return err
	}
	defer reader.Close()
	root := filepath.Clean(dest) + string(os.PathSeparator)
	for _, file := range reader.File {
		target := filepath.Join(dest, filepath.FromSlash(file.Name))
		if !strings.HasPrefix(target+string(os.PathSeparator), root) {
			return fmt.Errorf("安装包中有非法路径: %s", file.Name)
		}
		mode := file.Mode()
		if mode.IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		content, err := file.Open()
		if err != nil {
			return err
		}
		if mode&os.ModeSymlink != 0 {
			link, readErr := io.ReadAll(io.LimitReader(content, 4096))
			content.Close()
			if readErr != nil {
				return readErr
			}
			if err := os.Symlink(string(link), target); err != nil {
				return err
			}
			continue
		}
		err = writeFileFrom(target, fileModeOr(mode, file.Name), content)
		content.Close()
		if err != nil {
			return err
		}
		_ = os.Chtimes(target, file.Modified, file.Modified)
	}
	return nil
}
