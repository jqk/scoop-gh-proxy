package main

import (
	"fmt"
	"os"
	"os/exec"
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

	rows, err := parseScoopStatus(cfg.RootPath)
	if err != nil {
		error_("执行 scoop status -l 失败: %v", err)
		os.Exit(1)
	}

	results := make([]SetCommandItem, 0, len(rows))
	for _, row := range rows {
		results = append(results, classifyAppForSet(row, cfg))
	}

	modified := 0
	for _, r := range results {
		if r.Status != IsGitHub {
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
func classifyAppForSet(row SetCommandItem, cfg ScoopConfig) SetCommandItem {
	if row.Status == SetSkipped || row.Status == NoManifes {
		return row
	}

	ghURL, err := manifestHasGitHubDownloadURL(row.Manifest)
	if err != nil {
		warning("读取 %s manifest 失败: %v", row.Name, err)
		row.Status = ManifesError
		return row
	}
	if !ghURL {
		row.Status = NotGitHub
		return row
	}

	if fileExists(row.ManifestBackup) {
		warning("%s 的备份文件已存在，跳过", row.Name)
		row.Status = BackupExists
		return row
	}

	row.Status = IsGitHub
	return row
}

// applySetForApp 执行备份 + 修改，返回是否成功。
func applySetForApp(row SetCommandItem, cfg ScoopConfig) bool {
	manifestPath := findManifest(cfg.RootPath, row.Name, row.Bucket)
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

func printSetSummary(modified int, results []SetCommandItem) {
	if modified == 0 {
		info("Manifest to Set: 0")
		return
	}
	info("Manifest to Set: %d", modified)
	printSetTable(results)
}

// ---------------------------------------------------------------------------
// restore
// ---------------------------------------------------------------------------

func runRestore() {
	cfg, err := getScoopConfig()
	if err != nil {
		error_("%s", err)
		os.Exit(1)
	}

	items := collectRestoreItems(cfg.RootPath)
	restored := 0
	for i := range items {
		if err := restoreAppJSON(items[i].Path); err != nil {
			error_("还原 %s 失败: %v", items[i].Name, err)
			items[i].Status = RestoreFailed
			continue
		}
		restored++
		items[i].Status = RestoreSuccess
		success("已还原 %s", items[i].Name)
	}

	if cfg.GhScoopProxyBackup != "" {
		if err := exec.Command("scoop", "config", "proxy", cfg.GhScoopProxyBackup).Run(); err != nil {
			warning("scoop config proxy %s 失败: %v", cfg.GhScoopProxyBackup, err)
		} else {
			success("已执行 scoop config proxy %s", cfg.GhScoopProxyBackup)
		}
	}

	printRestoreSummary(restored, items, true)
}

func printRestoreSummary(restored int, items []RestoreCommandItem, showStatus bool) {
	if restored == 0 {
		info("Manifest to restore: 0")
		return
	}
	info("Manifest to restore: %d", restored)

	nameW, bucketW, statusW := len("App Name"), len("Bucket Name"), len("Status")
	for _, it := range items {
		if len(it.Name) > nameW {
			nameW = len(it.Name)
		}
		if len(it.Bucket) > bucketW {
			bucketW = len(it.Bucket)
		}
		if showStatus && len(string(it.Status)) > statusW {
			statusW = len(string(it.Status))
		}
	}

	if showStatus {
		rowFmt := fmt.Sprintf("%%-%ds  %%-%ds  %%-%ds\n", nameW, bucketW, statusW)
		fmt.Printf(rowFmt, "App Name", "Bucket Name", "Status")
		fmt.Printf(rowFmt, dashRun(nameW), dashRun(bucketW), dashRun(statusW))
		fmt.Println()
		for _, it := range items {
			status := string(it.Status)
			if status == "" {
				status = "Skipped"
			}
			c := pickColor(status)
			c.Fprintf(os.Stdout, rowFmt, it.Name, it.Bucket, status)
		}
	} else {
		rowFmt := fmt.Sprintf("%%-%ds  %%-%ds\n", nameW, bucketW)
		fmt.Printf(rowFmt, "App Name", "Bucket Name")
		fmt.Printf(rowFmt, dashRun(nameW), dashRun(bucketW))
		fmt.Println()
		for _, it := range items {
			fmt.Printf(rowFmt, it.Name, it.Bucket)
		}
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

	// 第一部分：restore 明细（dry-run：不实际还原，Status 全为 Skipped）
	resetItems := collectRestoreItems(cfg.RootPath)
	for i := range resetItems {
		resetItems[i].Status = RestoreSkipped
	}
	printRestoreSummary(len(resetItems), resetItems, true)

	// 第二部分：set 明细（按还原后状态判断）
	rows, err := parseScoopStatus(cfg.RootPath)
	if err != nil {
		error_("执行 scoop status -l 失败: %v", err)
		os.Exit(1)
	}

	results := make([]SetCommandItem, 0, len(rows))
	for _, row := range rows {
		results = append(results, classifyAppForSet(row, cfg))
	}
	setCount := 0
	for _, r := range results {
		if r.Status == IsGitHub {
			setCount++
		}
	}
	fmt.Println()
	printSetSummary(setCount, results)
}

// ---------------------------------------------------------------------------
// 共享：manifest 定位
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// 共享：set 结果表
// ---------------------------------------------------------------------------

func printSetTable(results []SetCommandItem) {
	const (
		hdrName   = "App Name"
		hdrVer    = "Installed Version"
		hdrLate   = "Latest Version"
		hdrBucket = "Bucket Name"
		hdrStatus = "Status"
	)

	nameW, verW, lateW, bucketW, statusW := len(hdrName), len(hdrVer), len(hdrLate), len(hdrBucket), len(hdrStatus)
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
		if len(r.Bucket) > bucketW {
			bucketW = len(r.Bucket)
		}
		if len(r.Status) > statusW {
			statusW = len(r.Status)
		}
	}

	rowFmt := fmt.Sprintf("%%-%ds  %%-%ds  %%-%ds  %%-%ds  %%-%ds\n", nameW, verW, lateW, bucketW, statusW)
	fmt.Printf(rowFmt, hdrName, hdrVer, hdrLate, hdrBucket, hdrStatus)
	fmt.Printf(rowFmt, dashRun(nameW), dashRun(verW), dashRun(lateW), dashRun(bucketW), dashRun(statusW))
	fmt.Println()

	for _, r := range results {
		status := r.Status
		if status == "" {
			status = "Not github"
		}
		c := pickColor(string(status))
		c.Fprintf(os.Stdout, rowFmt, r.Name, r.Installed, r.Latest, r.Bucket, status)
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
