package cli

import (
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/jqk/scoop-gh-proxy/internal/scoop"
)

// ---------------------------------------------------------------------------
// 下载进度（渲染 scoop.Progress 事件，核心层不生成展示文本）
// ---------------------------------------------------------------------------

// progressPrinter 下载进度渲染器，兼作 scoop 流式输出的透传 writer（进度刷新
// 与流式透传同写一个目标，经 mu 串行防交错）：
//   - 进度行始终 \r 回行首原地重绘一行（终端与管道型终端渲染一致，与 scoop
//     aria2 进度同观感），新行更短补空格覆盖残留；非终端（重定向到文件、GUI
//     注入的 writer）内容未变不重写（与 scoop 层 lineFilter 的重绘刷新规则一致，
//     两处同语义双实现是分层所需）
//   - 后续 scoop 流式输出经 Write 前先补 \n 终结进度行，避免拼接到进度行尾部
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

// printProgress 渲染一次下载进度（UpdateProxiedApp / UpdatePlainApp / InstallApp /
// InstallPlainApp 的进度回调入口）
func (p *progressPrinter) printProgress(ev scoop.Progress) {
	line := renderDownloadLine(ev.File, ev.Downloaded, ev.Total)

	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.console && line == p.last {
		return // 内容未变不重写
	}
	p.last = line

	// \r 回行首重绘；新行更短时补空格覆盖上一行的残留字符
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
