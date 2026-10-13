package cli

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/fatih/color"

	"github.com/jqk/scoop-gh-proxy/internal/scoop"
)

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
	fmt.Println("  scoop-gh update [app...]       更新所有过期软件；指定 app 时只更新指定的 app")
	fmt.Println("  scoop-gh install <bucket/app>  安装指定的应用, bucket 可省略")
	fmt.Println()
	fmt.Println("  scoop-gh help                  显示帮助，详细信息参见 README.md")
	fmt.Println("  scoop-gh version               显示版本")
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

// printRestoreTable 输出带 Status 列的三列明细表（restore 与 update 的遗留清理共用）
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
	case scoop.Updated, scoop.Installed:
		success("%s: %s", app.Name, app.Status)
	case scoop.UpdateFailed, scoop.InstallFailed, scoop.RestoreFailed:
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

	fmt.Println()
	info("Update Summary: %d updated, %d failed", updated, failed)
	if len(results) > 0 {
		printSetTable(results)
	}
}

// ---------------------------------------------------------------------------
// 下载进度（渲染 scoop.Progress 事件，核心层不生成展示文本）
// ---------------------------------------------------------------------------

// progressPrinter 下载进度渲染器，兼作 scoop 流式输出的透传 writer（进度刷新
// 与流式透传同写一个目标，经 mu 串行防交错）：
//   - 终端：进度事件以 \r 回行首原地重绘一行（与 scoop aria2 进度同观感）；
//     后续 scoop 流式输出经 Write 前先补 \n 终结进度行，避免拼接到进度行尾部
//   - 非终端（重定向到文件、GUI 注入的 writer）：内容变化才输出一行（与
//     lineFilter 的重绘折叠规则一致）
type progressPrinter struct {
	w       io.Writer // 底层显示目标（os.Stdout / GUI 注入的 writer）
	console bool      // 底层显示目标是否为终端（决定进度行的刷新方式）

	mu      sync.Mutex
	open    bool   // 终端模式：进度行未终结（光标停在行尾等待重绘）
	lastLen int    // 当前屏上进度行的宽度（新行更短时补空格覆盖残留）
	last    string // 非终端模式：最近一次输出的进度行（内容变化才输出）
}

// newProgressPrinter 构造进度渲染器，w 为 scoop 流式输出与进度行的共同显示目标
func newProgressPrinter(w io.Writer) *progressPrinter {
	return &progressPrinter{w: w, console: scoop.IsConsole(w)}
}

// WriterIsConsole 实现 scoop 的 consoleAware 可选接口：runScoopStream 据此对
// 包装后的 writer 仍按终端模式（原样透传）处理
func (p *progressPrinter) WriterIsConsole() bool { return p.console }

// Write 实现 io.Writer，scoop 流式输出经此透传：进度行未终结时先补换行
func (p *progressPrinter) Write(buf []byte) (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.endLine()
	return p.w.Write(buf)
}

// printProgress 渲染一次下载进度（UpdateProxiedApp / InstallApp 的进度回调入口）
func (p *progressPrinter) printProgress(ev scoop.Progress) {
	line := renderDownloadLine(ev.File, ev.Downloaded, ev.Total)

	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.console {
		if line == p.last {
			return
		}
		p.last = line
		fmt.Fprintln(p.w, line)
		return
	}

	// 终端：\r 回行首重绘；新行更短时补空格覆盖上一行的残留字符
	if len(line) < p.lastLen {
		line += strings.Repeat(" ", p.lastLen-len(line))
	} else {
		p.lastLen = len(line)
	}
	fmt.Fprintf(p.w, "\r%s", line)
	p.open = true
}

// endLine 终结未完的进度行（补换行并复位屏上行宽），无未完行时为 no-op
func (p *progressPrinter) endLine() {
	if !p.open {
		return
	}
	_, _ = fmt.Fprint(p.w, "\n")
	p.open = false
	p.lastLen = 0
}

// finishLine scoop 流程结束时显式终结进度行（此后不再有流式输出触发 Write）
func (p *progressPrinter) finishLine() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.endLine()
}

// renderDownloadLine 渲染一行进度（样式仿 scoop 自带进度条）。
// humanSize 最大 8 字符（"999.9 MB"），右对齐定宽后 "/"、进度条与百分比对齐：
//
//	bun#1.4.3#88bb33f.zip ( 508.0 KB/ 42.6 MB ) [                              ]   1%
//	bun#1.4.3#88bb33f.zip (   6.1 MB/ 42.6 MB ) [====>                         ]  14%
func renderDownloadLine(name string, size, total int64) string {
	const barW = 30
	pct := 0
	if total > 0 {
		pct = int(size * 100 / total)
		if pct > 100 {
			pct = 100
		}
	}
	filled := pct * barW / 100
	bar := strings.Repeat("=", filled)
	if filled > 0 && filled < barW {
		bar += ">"
	}
	bar += strings.Repeat(" ", barW-len(bar))

	if total > 0 {
		return fmt.Sprintf("%s (%8s/%8s ) [%s] %3d%%", name, humanSize(size), humanSize(total), bar, pct)
	}
	return fmt.Sprintf("%s (%s) [%s]", name, humanSize(size), bar)
}

// humanSize scoop filesize 的 Go 版（core.ps1：1024 进制，一位小数）
func humanSize(n int64) string {
	const kb, mb, gb = 1 << 10, 1 << 20, 1 << 30
	switch {
	case n > gb:
		return fmt.Sprintf("%.1f GB", float64(n)/gb)
	case n > mb:
		return fmt.Sprintf("%.1f MB", float64(n)/mb)
	case n > kb:
		return fmt.Sprintf("%.1f KB", float64(n)/kb)
	default:
		return fmt.Sprintf("%d B", n)
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
