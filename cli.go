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

func runSet(dryRun bool) {
	cfg, err := getScoopConfig()
	if err != nil {
		error_("%s", err)
		os.Exit(1)
	}

	rows, err := RunSetCommand(&cfg, dryRun)

	// rows, err := parseScoopStatus(cfg.RootPath)
	// if err != nil {
	// 	error_("执行 scoop status -l 失败: %v", err)
	// 	os.Exit(1)
	// }

	// for _, r := range rows {
	// 	if r.Status != IsGitHub {
	// 		continue
	// 	}

	// 	isGitHub, err := patchManifest(&r, cfg.GhProxy, dryRun)
	// 	if err != nil {
	// 		return
	// 	}

	// 	if isGitHub {
	// 		if !dryRun {
	// 			if err := backupManifest(&r); err != nil {
	// 				warning("备份 %s 失败: %v", r.Name, err)
	// 			}
	// 		}
	// 	}
	// }

	// if !dryRun {
	// 	if cfg.Proxy != "" && cfg.Proxy != cfg.GhScoopProxyBackup { // 保存一下，因为后面会清空此值
	// 		_, err := exec.Command("scoop", "config", "gh_scoop_proxy_backup", cfg.Proxy).Output()
	// 		if err != nil {
	// 			warning("执行 scoop config gh_scoop_proxy_backup %s 失败: %w", cfg.Proxy, err)
	// 		}
	// 	}
	// 	if cfg.Proxy != "" {
	// 		if err := exec.Command("scoop", "config", "rm", "proxy").Run(); err != nil {
	// 			warning("scoop config rm proxy 失败: %v", err)
	// 		} else {
	// 			success("已执行 scoop config rm proxy")
	// 		}
	// 	}
	// }

	printSetSummary(rows)
}

func printSetSummary(result []SetCommandItem) {
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

func runRestore(dryRun bool) {
	cfg, err := getScoopConfig()
	if err != nil {
		error_("%s", err)
		os.Exit(1)
	}

	items, err := findRestoreCommandItems(cfg.RootPath)
	if err != nil {
		error_("%s", err)
		os.Exit(1)
	}

	if !dryRun {
		for i := range items {
			if err := restoreManifest(&items[i]); err != nil {
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

func printRestoreSummary(items []RestoreCommandItem, showStatus bool) {
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

func runStatus() {
	runRestore(true)
	runSet(true)
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
