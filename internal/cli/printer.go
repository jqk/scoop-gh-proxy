package cli

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/fatih/color"

	"github.com/jqk/scoop-gh-proxy/internal/scoop"
)

// ---------------------------------------------------------------------------
// 用法
// ---------------------------------------------------------------------------

func printUsage() {
	color.New(color.FgWhite).Println(fmt.Sprintf("scoop-gh %s", version))
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
	rows := make([][]string, 0, len(items))
	for _, it := range items {
		rows = append(rows, []string{it.Name, it.Bucket})
	}
	printTable(os.Stdout, []string{"App Name", "Bucket Name"}, rows, nil)
}

// printRestoreTable 输出带 Status 列的三列明细表（restore 与 --update 的遗留清理共用）
func printRestoreTable(items []scoop.OutdatedApp) {
	rows := make([][]string, 0, len(items))
	for _, it := range items {
		rows = append(rows, []string{it.Name, it.Bucket, string(it.Status)})
	}
	printTable(os.Stdout, []string{"App Name", "Bucket Name", "Status"}, rows,
		func(row []string) *color.Color { return pickColor(row[2]) })
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

// printTable 输出等宽明细表：各列宽度取表头与数据的最大宽度，列间两个空格，
// 第二行为 "-" 分隔线。rowColor 为 nil 时行不着色；否则整行以其返回色输出
func printTable(w io.Writer, headers []string, rows [][]string, rowColor func(row []string) *color.Color) {
	widths := make([]int, len(headers))
	for i, h := range headers {
		widths[i] = len(h)
	}
	for _, row := range rows {
		for i, cell := range row {
			widths[i] = max(widths[i], len(cell))
		}
	}

	specs := make([]string, len(widths))
	dashes := make([]string, len(widths))
	for i, wd := range widths {
		specs[i] = fmt.Sprintf("%%-%ds", wd)
		dashes[i] = dashRun(wd)
	}
	rowFmt := strings.Join(specs, "  ") + "\n"

	printRow := func(cells []string) {
		args := make([]any, len(cells))
		for i, c := range cells {
			args[i] = c
		}
		c := plainColor // 无属性的 Color 输出原样文本
		if rowColor != nil {
			if rc := rowColor(cells); rc != nil {
				c = rc
			}
		}
		c.Fprintf(w, rowFmt, args...)
	}

	printRow(headers)
	printRow(dashes)
	for _, row := range rows {
		printRow(row)
	}
}

func printSetTable(results []scoop.OutdatedApp) {
	rows := make([][]string, 0, len(results))
	for _, r := range results {
		status := r.Status
		if status == "" {
			status = "Not github"
		}
		rows = append(rows, []string{r.Name, r.Installed, r.Latest, r.Bucket, string(status)})
	}
	printTable(os.Stdout,
		[]string{"App Name", "Installed Version", "Latest Version", "Bucket Name", "Status"}, rows,
		func(row []string) *color.Color { return pickColor(row[4]) })
}

// plainColor 无任何属性的 Color：wrap 时输出原样文本，用于不着色的行
var plainColor = color.New()

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
