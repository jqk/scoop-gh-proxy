package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/fatih/color"

	"github.com/jqk/scoop-gh-proxy/internal/scoop"
)

// ---------------------------------------------------------------------------
// 用法
// ---------------------------------------------------------------------------

func printUsage() {
	color.New(color.FgWhite).Println(fmt.Sprintf("scoop-gh %s", Version))
	fmt.Println()
	fmt.Println("用法:")
	fmt.Println("  scoop-gh --set      将 bucket 中的 GitHub URL 替换为带 gh_proxy 的 URL")
	fmt.Println("  scoop-gh --restore  还原已备份的原始 [app].json")
	fmt.Println("  scoop-gh --status   预览将要还原/修改的明细（不实际修改）")
	fmt.Println("  scoop-gh --update   自动更新所有 outdated 的软件")
	fmt.Println()
	fmt.Println("选项:")
	fmt.Println("  -h, --help     显示帮助")
	fmt.Println("  -v, --version  显示版本")
}

// ---------------------------------------------------------------------------
// set / restore 汇总
// ---------------------------------------------------------------------------

func printSetSummary(result []scoop.OutdatedApp) {
	count := len(result)
	if count == 0 {
		info("Manifest to Set: 0")
		return
	}
	info("Manifest to Set: %d", count)
	printSetTable(result)
}

func printRestoreSummary(items []scoop.OutdatedApp, showStatus bool) {
	count := len(items)
	if count == 0 {
		info("Manifest to restore: 0")
		return
	}
	info("Manifest to restore: %d", count)

	if showStatus {
		printRestoreTable(items)
		return
	}

	// 无 Status 列的两列表格（restore dry-run 使用）
	nameW, bucketW := len("App Name"), len("Bucket Name")
	for _, it := range items {
		nameW = max(nameW, len(it.Name))
		bucketW = max(bucketW, len(it.Bucket))
	}

	rowFmt := fmt.Sprintf("%%-%ds  %%-%ds\n", nameW, bucketW)
	fmt.Printf(rowFmt, "App Name", "Bucket Name")
	fmt.Printf(rowFmt, dashRun(nameW), dashRun(bucketW))

	for _, it := range items {
		fmt.Printf(rowFmt, it.Name, it.Bucket)
	}
}

// printRestoreTable 输出带 Status 列的三列明细表（restore 与 --update 的遗留清理共用）
func printRestoreTable(items []scoop.OutdatedApp) {
	nameW, bucketW, statusW := len("App Name"), len("Bucket Name"), len("Status")
	for _, it := range items {
		nameW = max(nameW, len(it.Name))
		bucketW = max(bucketW, len(it.Bucket))
		statusW = max(statusW, len(string(it.Status)))
	}

	rowFmt := fmt.Sprintf("%%-%ds  %%-%ds  %%-%ds\n", nameW, bucketW, statusW)
	fmt.Printf(rowFmt, "App Name", "Bucket Name", "Status")
	fmt.Printf(rowFmt, dashRun(nameW), dashRun(bucketW), dashRun(statusW))

	for _, it := range items {
		c := pickColor(string(it.Status))
		c.Fprintf(os.Stdout, rowFmt, it.Name, it.Bucket, string(it.Status))
	}
}

// ---------------------------------------------------------------------------
// update 输出
// ---------------------------------------------------------------------------

// printAppOutcome 输出单个 app 的更新结果行（走 stdout，与 scoop 流式输出同流保序）
func printAppOutcome(app *scoop.OutdatedApp) {
	switch app.Status {
	case scoop.Updated:
		success("%s: %s", app.Name, app.Status)
	case scoop.UpdateFailed, scoop.RestoreFailed:
		fail("%s: %s", app.Name, app.Status)
	default:
		caution("%s: %s", app.Name, app.Status) // BackupExists 等未参与更新的状态
	}
}

// printUpdateSummary 输出更新汇总：计数 + 更新结果明细表（仅覆盖本次更新的 Proxied 组）
func printUpdateSummary(plan *scoop.UpdatePlan) {
	updated, failed := 0, 0
	results := make([]scoop.OutdatedApp, 0, len(plan.Proxied))
	for _, app := range plan.Proxied {
		switch app.Status {
		case scoop.Updated:
			updated++
		case scoop.UpdateFailed, scoop.RestoreFailed:
			failed++
		}
		results = append(results, *app)
	}
	skipped := len(plan.Skipped)

	fmt.Println()
	info("Update Summary: %d updated, %d failed, %d skipped", updated, failed, skipped)
	if len(results) > 0 {
		printSetTable(results)
	}
}

// ---------------------------------------------------------------------------
// 共享表格
// ---------------------------------------------------------------------------

func printSetTable(results []scoop.OutdatedApp) {
	const (
		hdrName   = "App Name"
		hdrVer    = "Installed Version"
		hdrLate   = "Latest Version"
		hdrBucket = "Bucket Name"
		hdrStatus = "Status"
	)

	nameW, verW, lateW, bucketW, statusW := len(hdrName), len(hdrVer), len(hdrLate), len(hdrBucket), len(hdrStatus)
	for _, r := range results {
		nameW = max(nameW, len(r.Name))
		verW = max(verW, len(r.Installed))
		lateW = max(lateW, len(r.Latest))
		bucketW = max(bucketW, len(r.Bucket))
		statusW = max(statusW, len(r.Status))
	}

	rowFmt := fmt.Sprintf("%%-%ds  %%-%ds  %%-%ds  %%-%ds  %%-%ds\n", nameW, verW, lateW, bucketW, statusW)
	fmt.Printf(rowFmt, hdrName, hdrVer, hdrLate, hdrBucket, hdrStatus)
	fmt.Printf(rowFmt, dashRun(nameW), dashRun(verW), dashRun(lateW), dashRun(bucketW), dashRun(statusW))

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
	switch status {
	case "Skipped":
		return color.New(color.FgYellow)
	case "Not github", "Proxy set":
		return color.New(color.FgWhite)
	case "Failed", "Update failed":
		return color.New(color.FgRed)
	default:
		return color.New(color.FgGreen)
	}
}

func dashRun(n int) string {
	return strings.Repeat("-", n)
}
