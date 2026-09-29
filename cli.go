package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/fatih/color"
)

// ---------------------------------------------------------------------------
// set
// ---------------------------------------------------------------------------

func runSet() {
	cfg, err := getScoopConfig()
	if err != nil {
		error_("%s", err)
		os.Exit(1)
	}

	rows, err := parseScoopStatus()
	if err != nil {
		error_("执行 scoop status -l 失败: %v", err)
		os.Exit(1)
	}

	results := make([]statusRow, 0, len(rows))
	for _, row := range rows {
		results = append(results, classifyAppForSet(row, cfg))
	}

	modified := 0
	for _, r := range results {
		if r.ProxyStatus != cfg.GhProxy {
			continue
		}
		if applySetForApp(r, cfg) {
			modified++
		}
	}

	if cfg.Proxy != "" {
		if err := exec.Command("scoop", "config", "rm", "proxy").Run(); err != nil {
			warning("scoop config rm proxy 失败: %v", err)
		} else {
			success("已执行 scoop config rm proxy")
		}
	}

	printSetSummary(modified, results)
}

// classifyAppForSet 只读分析，判断某个 app 的状态，不做任何修改。
func classifyAppForSet(row statusRow, cfg scoopConfig) statusRow {
	if row.ProxyStatus == "Skipped" {
		return row
	}

	manifestPath := findManifest(cfg.RootPath, row.Name)
	if manifestPath == "" {
		warning("未找到 %s 的 manifest，跳过", row.Name)
		row.ProxyStatus = "Skipped"
		return row
	}

	ghURL, err := manifestHasGitHubURL(manifestPath)
	if err != nil {
		warning("读取 %s manifest 失败: %v", row.Name, err)
		row.ProxyStatus = "Skipped"
		return row
	}
	if !ghURL {
		row.ProxyStatus = "Not github"
		return row
	}

	if _, err := os.Stat(backupPath(manifestPath)); err == nil {
		warning("%s 的备份文件已存在，跳过", row.Name)
		row.ProxyStatus = "Skipped"
		return row
	}

	row.ProxyStatus = cfg.GhProxy
	return row
}

// applySetForApp 执行备份 + 修改，返回是否成功。
func applySetForApp(row statusRow, cfg scoopConfig) bool {
	manifestPath := findManifest(cfg.RootPath, row.Name)
	if manifestPath == "" {
		return false
	}

	ok, err := backupAppJSON(manifestPath)
	if err != nil {
		warning("备份 %s 失败: %v", row.Name, err)
		return false
	}
	if !ok {
		return false
	}

	if _, err := patchGitHubURLs(manifestPath, cfg.GhProxy); err != nil {
		error_("修改 %s manifest 失败: %v，还原备份", row.Name, err)
		_ = os.Rename(backupPath(manifestPath), manifestPath)
		return false
	}

	success("已更新 %s", row.Name)
	return true
}

func printSetSummary(modified int, results []statusRow) {
	if modified == 0 {
		info("Manifest to Set: 0")
		return
	}
	info("Manifest to Set: %d", modified)
	printSetTable(results)
}

// ---------------------------------------------------------------------------
// reset
// ---------------------------------------------------------------------------

// resetItem 一条待还原的 app 记录
type resetItem struct {
	Name   string
	Bucket string
	Path   string // [app].json 的完整路径（backup 同目录）
}

func runReset() {
	cfg, err := getScoopConfig()
	if err != nil {
		error_("%s", err)
		os.Exit(1)
	}

	items := collectResetItems(cfg.RootPath)
	restored := 0
	for _, it := range items {
		if err := restoreAppJSON(it.Path); err != nil {
			error_("还原 %s 失败: %v", it.Name, err)
			continue
		}
		restored++
		success("已还原 %s", it.Name)
	}

	if cfg.GhScoopProxyBackup != "" {
		if err := exec.Command("scoop", "config", "proxy", cfg.GhScoopProxyBackup).Run(); err != nil {
			warning("scoop config proxy %s 失败: %v", cfg.GhScoopProxyBackup, err)
		} else {
			success("已执行 scoop config proxy %s", cfg.GhScoopProxyBackup)
		}
	}

	printResetSummary(restored, items)
}

// collectResetItems 只读扫描，收集 buckets 下所有 *-gh-backup.json
func collectResetItems(rootPath string) []resetItem {
	bucketsDir := filepath.Join(rootPath, "buckets")
	var items []resetItem

	filepath.Walk(bucketsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		if !strings.HasSuffix(info.Name(), "-gh-backup.json") {
			return nil
		}
		appName := stripBackupSuffix(info.Name())
		items = append(items, resetItem{
			Name:   appName,
			Bucket: deriveBucketFromPath(bucketsDir, path),
			Path:   stripBackupSuffix(path),
		})
		return nil
	})

	if len(items) == 0 {
		info("未找到任何 *-gh-backup.json，无需还原")
	}
	return items
}

// deriveBucketFromPath 从 [app]-gh-backup.json 的路径推出 bucket 名
// 路径形如 buckets\<b>\bucket\<app>-gh-backup.json 或 buckets\<b>\<app>-gh-backup.json
func deriveBucketFromPath(bucketsDir, fullPath string) string {
	rel, err := filepath.Rel(bucketsDir, fullPath)
	if err != nil {
		return ""
	}
	parts := strings.Split(rel, string(filepath.Separator))
	if len(parts) == 0 {
		return ""
	}
	return parts[0]
}

func printResetSummary(restored int, items []resetItem) {
	if restored == 0 {
		info("Manifest to Reset: 0")
		return
	}
	info("Manifest to Reset: %d", restored)

	nameW, bucketW := len("Name"), len("Bucket")
	for _, it := range items {
		if len(it.Name) > nameW {
			nameW = len(it.Name)
		}
		if len(it.Bucket) > bucketW {
			bucketW = len(it.Bucket)
		}
	}
	rowFmt := fmt.Sprintf("%%-%ds  %%-%ds\n", nameW, bucketW)
	fmt.Printf(rowFmt, "Name", "Bucket")
	fmt.Printf(rowFmt, dashRun(nameW), dashRun(bucketW))
	fmt.Println()
	for _, it := range items {
		fmt.Printf(rowFmt, it.Name, it.Bucket)
	}
}

// ---------------------------------------------------------------------------
// status（dry-run：先 reset 明细，后 set 明细，不修改任何文件）
// ---------------------------------------------------------------------------

func runStatus() {
	cfg, err := getScoopConfig()
	if err != nil {
		error_("%s", err)
		os.Exit(1)
	}

	// 第一部分：reset 明细（待还原的 backup）
	resetItems := collectResetItems(cfg.RootPath)
	printResetSummary(len(resetItems), resetItems)

	// 第二部分：set 明细（按还原后状态判断）
	rows, err := parseScoopStatus()
	if err != nil {
		error_("执行 scoop status -l 失败: %v", err)
		os.Exit(1)
	}

	results := make([]statusRow, 0, len(rows))
	for _, row := range rows {
		results = append(results, classifyAppForSet(row, cfg))
	}
	setCount := 0
	for _, r := range results {
		if r.ProxyStatus == cfg.GhProxy {
			setCount++
		}
	}
	fmt.Println()
	printSetSummary(setCount, results)
}

// ---------------------------------------------------------------------------
// 共享：manifest 定位
// ---------------------------------------------------------------------------

// findManifest 读取 apps\<name>\current\install.json 获取 bucket，
// 在 buckets\<bucket>\bucket\<app>.json 或 buckets\<bucket>\<app>.json 中查找
func findManifest(rootPath, appName string) string {
	installPath := filepath.Join(rootPath, "apps", appName, "current", "install.json")
	data, err := os.ReadFile(installPath)
	if err != nil {
		return ""
	}
	var install struct {
		Bucket string `json:"bucket"`
	}
	if err := json.Unmarshal(data, &install); err != nil || install.Bucket == "" {
		return ""
	}
	bucketsBase := filepath.Join(rootPath, "buckets")
	for _, candidate := range []string{
		filepath.Join(bucketsBase, install.Bucket, "bucket", appName+".json"),
		filepath.Join(bucketsBase, install.Bucket, appName+".json"),
	} {
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return ""
}

// ---------------------------------------------------------------------------
// 共享：set 结果表
// ---------------------------------------------------------------------------

func printSetTable(results []statusRow) {
	const (
		hdrName  = "Name"
		hdrVer   = "Installed Version"
		hdrLate  = "Latest Version"
		hdrProxy = "Proxy Status"
	)

	nameW, verW, lateW, proxyW := len(hdrName), len(hdrVer), len(hdrLate), len(hdrProxy)
	for _, r := range results {
		if len(r.Name) > nameW {
			nameW = len(r.Name)
		}
		if len(r.Installed) > verW {
			verW = len(r.Installed)
		}
		if len(r.Latest) > lateW {
			lateW = len(r.Latest)
		}
		if len(r.ProxyStatus) > proxyW {
			proxyW = len(r.ProxyStatus)
		}
	}

	rowFmt := fmt.Sprintf("%%-%ds  %%-%ds  %%-%ds  %%-%ds\n", nameW, verW, lateW, proxyW)
	fmt.Printf(rowFmt, hdrName, hdrVer, hdrLate, hdrProxy)
	fmt.Printf(rowFmt, dashRun(nameW), dashRun(verW), dashRun(lateW), dashRun(proxyW))
	fmt.Println()

	for _, r := range results {
		status := r.ProxyStatus
		if status == "" {
			status = "Not github"
		}
		c := pickColor(status)
		c.Fprintf(os.Stdout, rowFmt, r.Name, r.Installed, r.Latest, status)
	}
}

func pickColor(status string) *color.Color {
	switch {
	case status == "Skipped":
		return color.New(color.FgYellow)
	case status == "Not github":
		return color.New(color.FgWhite)
	default:
		return color.New(color.FgGreen)
	}
}

func dashRun(n int) string {
	return strings.Repeat("-", n)
}
