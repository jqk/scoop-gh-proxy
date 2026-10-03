package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/fatih/color"

	"github.com/jqk/scoop-gh-proxy/internal/scoop"
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

	apps, err := scoop.SetProxyForOutdatedApps(&cfg, dryRun)
	printSetSummary(apps)
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

	items, err := scoop.RestoreOutdatedAppManifests(&cfg, dryRun)
	if err != nil {
		error_("%s", err)
	}
	printRestoreSummary(items, !dryRun)
}

func printRestoreSummary(items []scoop.OutdatedApp, showStatus bool) {
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

		for _, it := range items {
			c := pickColor(string(it.Status))
			c.Fprintf(os.Stdout, rowFmt, it.Name, it.Bucket, string(it.Status))
		}
	} else {
		rowFmt := fmt.Sprintf("%%-%ds  %%-%ds\n", nameW, bucketW)
		fmt.Printf(rowFmt, "App Name", "Bucket Name")
		fmt.Printf(rowFmt, dashRun(nameW), dashRun(bucketW))

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
	fmt.Println() // 分隔 restore 明细与 set 明细
	RunSet(true)
}

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
	switch  status{
	case "Skipped":
		return color.New(color.FgYellow)
	case "Not github", "Proxy set":
		return color.New(color.FgWhite)
	case "Failed":
		return color.New(color.FgRed)
	default:
		return color.New(color.FgGreen)
	}
}

func dashRun(n int) string {
	return strings.Repeat("-", n)
}
