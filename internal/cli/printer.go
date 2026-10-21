package cli

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/fatih/color"

	"github.com/jqk/scoop-gh-proxy/internal/scoop"
)

// version 版本号，构建时由 build.bat / release.yml 通过 -ldflags -X 注入
// （规则：最近 tag 的 patch 号 + 其后提交次数）；直接 go build 时为 dev。
// -X 按包路径定位变量（internal/cli），与所在文件无关
var version = "dev"

// commitDate 版本对应代码（HEAD）的提交日期（YYYY-MM-DD），构建时注入；
// 为空表示非脚本构建
var commitDate = ""

// ---------------------------------------------------------------------------
// 程序信息头
// ---------------------------------------------------------------------------

// bannerText 程序信息头：版权行 + 名称版本行 + 空行，每条命令的输出以此开始。
// 日期为版本对应代码（HEAD）的提交日期，构建时注入；直接 go build 时省略日期，
// 版权年份取当前年
func bannerText() string {
	year := strconv.Itoa(time.Now().Year())
	if len(commitDate) >= 4 {
		year = commitDate[:4]
	}

	line2 := fmt.Sprintf("scoop-gh-proxy %s", version)
	if commitDate != "" {
		line2 += ", " + commitDate
	}

	return fmt.Sprintf("Copyright (c) 1999-%s Not a dream Co., Ltd.\n%s\n\n", year, line2)
}

// printBanner 输出程序信息头
func printBanner() {
	fmt.Print(bannerText())
}

// ---------------------------------------------------------------------------
// 用法
// ---------------------------------------------------------------------------

func printUsage() {
	fmt.Println("用法:")
	fmt.Println("  scoop-gh set                   向待更新应用 manifest 中的 github 下载链接插入代理地址")
	fmt.Println("  scoop-gh restore               还原 set 命令的修改")
	fmt.Println("  scoop-gh status                预览将要 set/restore 的明细")
	fmt.Println("  scoop-gh update [app...]       更新所有过期软件；不带参数时先更新 scoop 与 buckets")
	fmt.Println("  scoop-gh install <bucket/app>  安装指定的应用, bucket 可省略")
	fmt.Println("  scoop-gh list                  列出已安装应用, 按更新时间升序")
	fmt.Println()
	fmt.Println("  scoop-gh help                  显示帮助，详细信息参见 README.md")
	fmt.Println("  scoop-gh version               显示版本")
}

// ---------------------------------------------------------------------------
// set / restore 汇总
// ---------------------------------------------------------------------------

func printSetSummary(result []scoop.ManagedApp) {
	count := len(result)
	if count == 0 {
		info("Manifest to Set: 0")
		return
	}
	info("Manifest to Set: %d", count)
	printSetTable(result)
}

func printRestoreSummary(items []scoop.ManagedApp, showStatus bool) {
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

// printRestoreTable 输出带 Status 列的三列明细表（restore 与 update 的遗留清理共用）
func printRestoreTable(items []scoop.ManagedApp) {
	rows := make([][]string, 0, len(items))
	for _, it := range items {
		rows = append(rows, []string{it.Name, it.Bucket, string(it.Status)})
	}
	printTable(os.Stdout, []string{"App Name", "Bucket Name", "Status"}, rows,
		func(row []string) *color.Color { return pickColor(scoop.ManagedAppStatus(row[2])) })
}

// ---------------------------------------------------------------------------
// update 输出
// ---------------------------------------------------------------------------

// printAppOutcome 输出单个 app 的更新结果行（走 stdout，与 scoop 流式输出同流保序）
func printAppOutcome(app *scoop.ManagedApp) {
	switch app.Status {
	case scoop.Updated, scoop.Installed:
		success("%s: %s", app.Name, app.Status)
	case scoop.UpdateFailed, scoop.InstallFailed, scoop.RestoreFailed:
		fail("%s: %s", app.Name, app.Status)
	default:
		caution("%s: %s", app.Name, app.Status) // BackupExists 等未参与更新的状态
	}
}

// printGroupSeparator 输出 update 两组操作之间的分隔横线（横线前一空行由本函数输出，
// 横线后一空行由下一组首个 app 块的空行承担）；两组任一为空时调用方不调用本函数
func printGroupSeparator() {
	fmt.Println()
	fmt.Println(dashRun(64))
}

// printUpdateSummary 输出更新汇总：计数 + 更新结果明细表（覆盖本次更新的
// Proxied 与 Not github 两组，顺序与执行顺序一致）
func printUpdateSummary(plan *scoop.UpdatePlan) {
	updated, failed := 0, 0
	results := make([]scoop.ManagedApp, 0, len(plan.Proxied)+len(plan.NotGithub))
	for _, group := range [][]*scoop.ManagedApp{plan.Proxied, plan.NotGithub} {
		for _, app := range group {
			switch app.Status {
			case scoop.Updated:
				updated++
			case scoop.UpdateFailed, scoop.RestoreFailed:
				failed++
			}
			results = append(results, *app)
		}
	}

	fmt.Println()
	info("Update Summary: %d updated, %d failed", updated, failed)
	if len(results) > 0 {
		printSetTable(results)
	}
}

// ---------------------------------------------------------------------------
// list 输出
// ---------------------------------------------------------------------------

// printListTable 输出已安装应用明细表（list 命令）：按更新时间升序，最久未更新的
// 在最前。Updated 为该应用最近一次安装/更新的时间（scoop list 的 Updated 列），
// 整行按 Updated 距今时长着色（见 listRowColor）
func printListTable(apps []scoop.ListApp) {
	info("Installed apps: %d", len(apps))
	if len(apps) == 0 {
		return
	}
	rows := make([][]string, 0, len(apps))
	for _, a := range apps {
		updated := ""
		if !a.Updated.IsZero() {
			updated = a.Updated.Format(time.DateTime)
		}
		rows = append(rows, []string{a.Name, a.Version, a.Source, updated, a.Info})
	}
	printTable(os.Stdout, []string{"App Name", "Version", "Bucket Name", "Updated", "Info"}, rows,
		listRowColor(time.Now()))
}

// listAgeBands list 明细行按 Updated 距今时长的四个分组（按新到旧排列），行着色
// 与表后统计共用：band 为统计行文本（Updated 仅首行带出，其余行共享其义），
// color 为整行颜色（nil = 正常色）
var listAgeBands = []struct {
	band  string
	color *color.Color
}{
	{"Updated within 1 month", color.New(color.FgGreen)},
	{"1 month to 1 year", nil},
	{"1 to 2 years", color.New(color.FgYellow)},
	{"over 2 years", color.New(color.FgRed)},
}

// listAgeIndex 返回 Updated 距今落入的分组下标（listAgeBands）。
// 边界归属较新的一组：恰好 1 个月 / 1 年 / 2 年归前一组
func listAgeIndex(updated, now time.Time) int {
	switch {
	case !updated.Before(now.AddDate(0, -1, 0)):
		return 0
	case !updated.Before(now.AddDate(-1, 0, 0)):
		return 1
	case !updated.Before(now.AddDate(-2, 0, 0)):
		return 2
	default:
		return 3
	}
}

// listRowColor 按 Updated 列（time.DateTime 文本）距今时长选择整行颜色
// （分组见 listAgeBands）；无法解析（含零值输出的空串）返回 nil 保持原色
func listRowColor(now time.Time) func(row []string) *color.Color {
	return func(row []string) *color.Color {
		updated, err := time.ParseInLocation(time.DateTime, row[3], time.Local)
		if err != nil {
			return nil
		}
		return listAgeBands[listAgeIndex(updated, now)].color
	}
}

// listAgeCounts 统计各分组的数量（无法解析 Updated 的零值不计入任何分组，
// 仅计入总数）
func listAgeCounts(apps []scoop.ListApp, now time.Time) []int {
	counts := make([]int, len(listAgeBands))
	for _, a := range apps {
		if !a.Updated.IsZero() {
			counts[listAgeIndex(a.Updated, now)]++
		}
	}
	return counts
}

// printListSummary 输出 list 明细表之后的时长分组统计：四个分组各一行，
// 标签右对齐共用同一冒号列、数量右对齐共用同一个位列，各行颜色与明细表
// 着色一致；总数即明细表上方的 "Installed apps: N"，不重复输出。
// Updated 距今越久说明该软件自那时起再无新版本可更（上游无代码升级），
// 并非用户未更新。N 为 0 时不输出（与明细表一致）
func printListSummary(apps []scoop.ListApp) {
	if len(apps) == 0 {
		return
	}
	counts := listAgeCounts(apps, time.Now())

	fmt.Println()
	labelWidth, countWidth := 0, 0
	for i, b := range listAgeBands {
		labelWidth = max(labelWidth, len(b.band))
		countWidth = max(countWidth, len(strconv.Itoa(counts[i])))
	}
	countWidth++ // 冒号后至少留一个空格
	for i, b := range listAgeBands {
		c := plainColor
		if b.color != nil {
			c = b.color
		}
		c.Fprintf(os.Stdout, "%*s:%*d\n", labelWidth, b.band, countWidth, counts[i])
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

func printSetTable(results []scoop.ManagedApp) {
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
		func(row []string) *color.Color { return pickColor(scoop.ManagedAppStatus(row[4])) })
}

// plainColor 无任何属性的 Color：wrap 时输出原样文本，用于不着色的行
var plainColor = color.New()

// pickColor 按状态值选择整行颜色（与 scoop 的状态常量编译期绑定）
func pickColor(status scoop.ManagedAppStatus) *color.Color {
	switch status {
	case scoop.Skipped:
		return color.New(color.FgYellow)
	case scoop.NotGitHub, scoop.ProxySet:
		return color.New(color.FgWhite)
	case scoop.RestoreFailed, scoop.UpdateFailed, scoop.InstallFailed:
		return color.New(color.FgRed)
	default:
		return color.New(color.FgGreen)
	}
}

func dashRun(n int) string {
	return strings.Repeat("-", n)
}
