package cli

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/fatih/color"

	"github.com/zhangsan/scoop-gh-proxy/internal/scoop"
)

// ---------------------------------------------------------------------------
// set
// ---------------------------------------------------------------------------

func RunSet(dryRun bool) {
	cfg, err := scoop.GetScoopConfig()
	if err != nil {
		error_("%s", err)
		os.Exit(1)
	}

	rows, err := scoop.RunSetCommand(&cfg, dryRun)

	printSetSummary(rows)
}

func printSetSummary(result []scoop.OutdatedApp) {
	count := len(result)
	if count == 0 {
		info("Manifest to Set: 0")
		return
	}
	info("Manifest to Set: %d", count)
	printSetTable(result)
}

// ---------------------------------------------------------------------------
// restore
// ---------------------------------------------------------------------------

func RunRestore(dryRun bool) {
	cfg, err := scoop.GetScoopConfig()
	if err != nil {
		error_("%s", err)
		os.Exit(1)
	}

	items, err := scoop.FindRestoreCommandItems(cfg.RootPath)
	if err != nil {
		error_("%s", err)
		os.Exit(1)
	}

	if !dryRun {
		for i := range items {
			if err := scoop.RestoreManifest(&items[i]); err != nil {
				error_("还原 %s 失败: %v", items[i].Name, err)
				continue
			}

			success("已还原 %s", items[i].Name)
		}

		if cfg.GhScoopProxyBackup != "" && cfg.Proxy != cfg.GhScoopProxyBackup {
			if err := exec.Command("scoop", "config", "proxy", cfg.GhScoopProxyBackup).Run(); err != nil {
				warning("scoop config proxy %s 失败: %v", cfg.GhScoopProxyBackup, err)
			} else {
				success("已执行 scoop config proxy %s", cfg.GhScoopProxyBackup)
			}
		}
	}

	printRestoreSummary(items, !dryRun)
}

func printRestoreSummary(items []scoop.RestoreCommandItem, showStatus bool) {
	count := len(items)
	if count == 0 {
		info("Manifest to restore: 0")
		return
	}
	info("Manifest to restore: %d", count)

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

func RunStatus() {
	RunRestore(true)
	RunSet(true)
}

// ---------------------------------------------------------------------------
// 共享：manifest 定位
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// 共享：set 结果表
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
