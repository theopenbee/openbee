package main

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

const (
	githubAPILatest = "https://api.github.com/repos/theopenbee/openbee/releases/latest"
	githubRelBase   = "https://github.com/theopenbee/openbee/releases/download"

	upgradeBinaryName    = "openbee"
	upgradeBinaryNameWin = "openbee.exe"
	maxDownloadBytes     = 512 * 1024 * 1024 // 512 MB guard against runaway responses
)

type githubRelease struct {
	TagName string `json:"tag_name"`
}

var upgradeCheckOnly bool

var upgradeCmd = &cobra.Command{
	Use:   "upgrade",
	Short: "升级 openbee 到最新版本",
	Long:  "检测是否有新版本可用，如有则下载并替换当前二进制文件。",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runUpgrade(upgradeCheckOnly)
	},
}

func init() {
	upgradeCmd.Flags().BoolVar(&upgradeCheckOnly, "check", false, "仅检测是否有新版本，不执行升级")
	rootCmd.AddCommand(upgradeCmd)
}

func runUpgrade(checkOnly bool) error {
	current := version

	fmt.Printf("当前版本: %s\n", current)
	fmt.Println("正在检测最新版本...")

	latest, err := fetchLatestVersion()
	if err != nil {
		return fmt.Errorf("获取最新版本失败: %w", err)
	}

	fmt.Printf("最新版本: %s\n", latest)

	if !isNewer(latest, current) {
		fmt.Println("已是最新版本，无需升级。")
		return nil
	}

	fmt.Printf("发现新版本: %s\n", latest)

	if checkOnly {
		fmt.Printf("运行 'openbee upgrade' 执行升级。\n")
		return nil
	}

	return doUpgrade(latest)
}

func fetchLatestVersion() (string, error) {
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Get(githubAPILatest)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GitHub API 返回 %d", resp.StatusCode)
	}

	var rel githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return "", fmt.Errorf("解析响应失败: %w", err)
	}

	tag := strings.TrimSpace(rel.TagName)
	if tag == "" {
		return "", fmt.Errorf("获取到的版本号为空")
	}
	if !strings.HasPrefix(tag, "v") {
		tag = "v" + tag
	}
	return tag, nil
}

// isNewer returns true when latest is strictly newer than current.
// Falls back to string comparison for non-semver tags (e.g. "dev").
func isNewer(latest, current string) bool {
	lv := parseSemver(latest)
	cv := parseSemver(current)
	if lv == nil || cv == nil {
		return latest != current
	}
	for i := range min(len(lv), len(cv)) {
		if lv[i] > cv[i] {
			return true
		}
		if lv[i] < cv[i] {
			return false
		}
	}
	// All common parts equal: longer version (e.g. 1.0.1 vs 1.0) is newer.
	return len(lv) > len(cv)
}

func parseSemver(v string) []int {
	v = strings.TrimPrefix(v, "v")
	// Drop pre-release / build-metadata suffixes
	v = strings.SplitN(v, "-", 2)[0]
	v = strings.SplitN(v, "+", 2)[0]
	parts := strings.Split(v, ".")
	nums := make([]int, len(parts))
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			return nil
		}
		nums[i] = n
	}
	return nums
}

func doUpgrade(newVersion string) error {
	goos := runtime.GOOS

	versionNum := strings.TrimPrefix(newVersion, "v")
	archiveName := fmt.Sprintf("openbee-%s-%s-%s.tar.gz", versionNum, goos, runtime.GOARCH)
	archiveURL := fmt.Sprintf("%s/%s/%s", githubRelBase, newVersion, archiveName)
	checksumURL := fmt.Sprintf("%s/%s/checksums.txt", githubRelBase, newVersion)

	fmt.Printf("正在下载 %s...\n", archiveName)

	tmpDir, err := os.MkdirTemp("", "openbee-upgrade-*")
	if err != nil {
		return fmt.Errorf("创建临时目录失败: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	archivePath := filepath.Join(tmpDir, archiveName)
	if err := downloadFile(archiveURL, archivePath); err != nil {
		return fmt.Errorf("下载失败: %w", err)
	}

	// Verify checksum if available
	checksumPath := filepath.Join(tmpDir, "checksums.txt")
	if err := downloadFile(checksumURL, checksumPath); err != nil {
		fmt.Printf("警告: 无法下载 checksums.txt，跳过校验 (%v)\n", err)
	} else {
		fmt.Println("正在校验 SHA256...")
		if err := verifyChecksum(archivePath, archiveName, checksumPath); err != nil {
			return fmt.Errorf("校验失败: %w", err)
		}
		fmt.Println("SHA256 校验通过。")
	}

	// Extract binary from tarball
	binName := upgradeBinaryName
	if goos == "windows" {
		binName = upgradeBinaryNameWin
	}
	newBinPath := filepath.Join(tmpDir, binName)
	if err := extractBinary(archivePath, newBinPath); err != nil {
		return fmt.Errorf("解压失败: %w", err)
	}

	// Locate the current executable
	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("无法确定当前可执行文件路径: %w", err)
	}
	execPath, err = filepath.EvalSymlinks(execPath)
	if err != nil {
		return fmt.Errorf("解析符号链接失败: %w", err)
	}

	// Atomic replace: write to a temp file next to the target, then rename
	dir := filepath.Dir(execPath)
	tmpBin, err := os.CreateTemp(dir, ".openbee-new-*")
	if err != nil {
		// May lack write permission — try sudo-less approach with a clear message
		return fmt.Errorf("无法在 %s 创建临时文件 (可能需要 sudo): %w", dir, err)
	}
	tmpBinPath := tmpBin.Name()
	tmpBin.Close()
	defer os.Remove(tmpBinPath)

	if err := copyFile(newBinPath, tmpBinPath); err != nil {
		return fmt.Errorf("复制新二进制失败: %w", err)
	}
	if err := os.Chmod(tmpBinPath, 0755); err != nil {
		return fmt.Errorf("设置权限失败: %w", err)
	}
	if err := os.Rename(tmpBinPath, execPath); err != nil {
		return fmt.Errorf("替换二进制失败 (可能需要 sudo): %w", err)
	}

	fmt.Printf("升级成功！openbee 已更新到 %s。\n", newVersion)
	return nil
}

func downloadFile(url, dest string) error {
	client := &http.Client{Timeout: 5 * time.Minute}
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, url)
	}
	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, io.LimitReader(resp.Body, maxDownloadBytes))
	return err
}

func verifyChecksum(archivePath, archiveName, checksumPath string) error {
	data, err := os.ReadFile(checksumPath)
	if err != nil {
		return err
	}

	var expected string
	for line := range strings.SplitSeq(string(data), "\n") {
		parts := strings.Fields(line)
		if len(parts) == 2 && parts[1] == archiveName {
			expected = parts[0]
			break
		}
	}
	if expected == "" {
		return fmt.Errorf("checksums.txt 中未找到 %s 的校验值", archiveName)
	}

	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}
	actual := fmt.Sprintf("%x", h.Sum(nil))
	if actual != expected {
		return fmt.Errorf("SHA256 不匹配\n  期望: %s\n  实际: %s", expected, actual)
	}
	return nil
}

func extractBinary(archivePath, destPath string) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		name := filepath.Base(hdr.Name)
		if name == upgradeBinaryName || name == upgradeBinaryNameWin {
			out, err := os.Create(destPath)
			if err != nil {
				return err
			}
			if _, err := io.Copy(out, tr); err != nil {
				out.Close()
				return err
			}
			return out.Close()
		}
	}
	return fmt.Errorf("压缩包中未找到 openbee 二进制文件")
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}
